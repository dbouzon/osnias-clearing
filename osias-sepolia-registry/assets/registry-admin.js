(() => {
  "use strict";

  const CHAIN_ID = 11155111;
  const CHAIN_HEX = "0xaa36a7";
  const REGISTRY = "0xf7a0cAd5DDE44cA3E8B61C0d08A9019c3944Cb55";

  const ABI = [
    "function KK() view returns (uint8)",
    "function NETWORK_NAME() view returns (string)",
    "function owner() view returns (address)",
    "function getById(uint64 osniasId) view returns ((address localAddress,uint64 osniasId,uint8 status,uint24 reserved))",
    "function getByLocalAddress(address localAddress) view returns (uint64 osniasId,uint8 status)",
    "function setParticipantStatus(uint64 osniasId,uint8 newStatus)",
    "event ParticipantRegistered(uint64 indexed osniasId,uint16 indexed nnn,address indexed localAddress,uint8 status,address registrar)",
    "event ParticipantStatusChanged(uint64 indexed osniasId,address indexed localAddress,uint8 oldStatus,uint8 newStatus)"
  ];

  const STATUS = ["UNUSED","PENDING","ACTIVE","SUSPENDED","CLOSED","REVOKED","ARCHIVED"];

  let provider;
  let signer;
  let contract;
  let ownerAddress = null;
  let registered = [];

  const $ = id => document.getElementById(id);

  function log(msg) {
    $("log").textContent += `\n[${new Date().toLocaleTimeString()}] ${msg}`;
    $("log").scrollTop = $("log").scrollHeight;
  }

  function formatId(id) {
    const s = BigInt(id).toString().padStart(16, "0");
    return `${s.slice(0,2)}-${s.slice(2,5)}-${s.slice(5,14)}-${s.slice(14,16)}`;
  }

  function statusHtml(n) {
    const name = STATUS[n] || `UNKNOWN(${n})`;
    let cls = "s-terminal";
    if (n === 1) cls = "s-pending";
    if (n === 2) cls = "s-active";
    if (n === 3) cls = "s-suspended";
    return `<span class="status ${cls}">${n} · ${name}</span>`;
  }

  async function readonly() {
    if (!provider) provider = new ethers.JsonRpcProvider("https://ethereum-sepolia-rpc.publicnode.com");
    if (!contract) contract = new ethers.Contract(REGISTRY, ABI, provider);
    return contract;
  }

  async function loadHeader() {
    const c = await readonly();
    const [kk, networkName, owner] = await Promise.all([c.KK(), c.NETWORK_NAME(), c.owner()]);
    ownerAddress = owner;
    $("kkValue").textContent = kk.toString();
    $("networkName").textContent = networkName;
    $("registryOwner").textContent = owner;
  }

  async function connect() {
    if (!window.ethereum) throw new Error("No injected EVM wallet detected.");

    await window.ethereum.request({ method:"eth_requestAccounts" });
    provider = new ethers.BrowserProvider(window.ethereum);

    let network = await provider.getNetwork();
    if (Number(network.chainId) !== CHAIN_ID) {
      await window.ethereum.request({
        method:"wallet_switchEthereumChain",
        params:[{chainId:CHAIN_HEX}]
      });
      provider = new ethers.BrowserProvider(window.ethereum);
    }

    signer = await provider.getSigner();
    contract = new ethers.Contract(REGISTRY, ABI, signer);

    const wallet = await signer.getAddress();
    $("walletAddress").textContent = wallet;
    $("networkDot").classList.add("good");
    $("networkState").textContent = "Sepolia · connected";

    await loadHeader();

    if (wallet.toLowerCase() === ownerAddress.toLowerCase()) {
      log("Registry owner wallet connected.");
    } else {
      log(`WARNING: connected wallet is not Registry owner. Owner=${ownerAddress}`);
    }
  }

  async function scanEvents() {
    const c = await readonly();
    const from = Number($("fromBlock").value);
    const latest = await provider.getBlockNumber();

    if (!Number.isInteger(from) || from < 0 || from > latest) {
      throw new Error("Invalid start block.");
    }

    log(`Scanning ParticipantRegistered events from block ${from} to ${latest}...`);

    const filter = c.filters.ParticipantRegistered();
    const chunk = 5000;
    const events = [];

    for (let start = from; start <= latest; start += chunk) {
      const end = Math.min(start + chunk - 1, latest);
      const logs = await c.queryFilter(filter, start, end);
      events.push(...logs);
    }

    const dedup = new Map();
    for (const ev of events) {
      dedup.set(ev.args.osniasId.toString(), {
        id: ev.args.osniasId,
        nnn: Number(ev.args.nnn),
        address: ev.args.localAddress
      });
    }

    registered = [...dedup.values()];
    await renderParticipants();
    log(`${registered.length} participant(s) found.`);
  }

  async function renderParticipants() {
    const c = await readonly();
    const body = $("participantsBody");

    if (!registered.length) {
      body.innerHTML = `<tr><td colspan="5" class="empty">No registered participant found.</td></tr>`;
      return;
    }

    const rows = [];

    for (const item of registered) {
      const p = await c.getById(item.id);
      const st = Number(p.status);
      const terminal = st >= 4;

      rows.push(`
        <tr>
          <td><strong>${formatId(item.id)}</strong><br><code>${item.id.toString()}</code></td>
          <td><code>${p.localAddress}</code></td>
          <td>${statusHtml(st)}</td>
          <td>${String(item.nnn).padStart(3,"0")}</td>
          <td>
            <div class="row-actions">
              <button class="btn" data-id="${item.id}" data-status="2" ${terminal || st===2 ? "disabled":""}>Active</button>
              <button class="btn" data-id="${item.id}" data-status="3" ${terminal || st===3 || st===1 ? "disabled":""}>Suspend</button>
              <button class="btn" data-id="${item.id}" data-status="4" ${terminal ? "disabled":""}>Close</button>
              <button class="btn" data-id="${item.id}" data-status="5" ${terminal ? "disabled":""}>Revoke</button>
              <button class="btn" data-id="${item.id}" data-status="6" ${terminal ? "disabled":""}>Archive</button>
            </div>
          </td>
        </tr>
      `);
    }

    body.innerHTML = rows.join("");

    body.querySelectorAll("button[data-id]").forEach(btn => {
      btn.addEventListener("click", () => {
        changeStatus(BigInt(btn.dataset.id), Number(btn.dataset.status))
          .catch(handleError);
      });
    });
  }

  async function changeStatus(id, newStatus) {
    if (!signer) throw new Error("Connect the Registry owner wallet first.");

    const wallet = await signer.getAddress();
    if (!ownerAddress) await loadHeader();

    if (wallet.toLowerCase() !== ownerAddress.toLowerCase()) {
      throw new Error("Connected wallet is not the Registry owner.");
    }

    const targetName = STATUS[newStatus];
    const confirmed = confirm(
      `Change ${formatId(id)} to ${targetName}?\n\n` +
      (newStatus >= 4 ? "WARNING: this status is irreversible." : "")
    );

    if (!confirmed) return;

    log(`Changing ${formatId(id)} -> ${targetName}...`);

    const tx = await contract.setParticipantStatus(id, newStatus);
    log(`Transaction sent: ${tx.hash}`);

    const receipt = await tx.wait();
    log(`Confirmed in block ${receipt.blockNumber}.`);

    await renderParticipants();
  }

  async function lookupById() {
    const raw = $("lookupId").value.replace(/\D/g,"");
    if (!raw) throw new Error("Enter an Osnias-ID.");

    const id = BigInt(raw);
    const c = await readonly();
    const p = await c.getById(id);

    $("lookupResult").textContent =
      `Osnias-ID : ${formatId(id)}\n` +
      `Machine   : ${p.osniasId.toString()}\n` +
      `Address   : ${p.localAddress}\n` +
      `Status    : ${Number(p.status)} · ${STATUS[Number(p.status)] || "UNKNOWN"}`;
  }

  async function lookupByAddress() {
    const address = $("lookupAddress").value.trim();
    if (!ethers.isAddress(address)) throw new Error("Invalid EVM address.");

    const c = await readonly();
    const [id, status] = await c.getByLocalAddress(address);

    $("lookupResult").textContent =
      id === 0n
        ? "Address not registered."
        : `Osnias-ID : ${formatId(id)}\nMachine   : ${id.toString()}\nAddress   : ${address}\nStatus    : ${Number(status)} · ${STATUS[Number(status)] || "UNKNOWN"}`;
  }

  async function refresh() {
    await loadHeader();
    if (registered.length) await renderParticipants();
  }

  function handleError(err) {
    console.error(err);
    const msg = err?.shortMessage || err?.reason || err?.message || String(err);
    log(`ERROR: ${msg}`);
    alert(msg);
  }

  $("connectBtn").addEventListener("click", () => connect().catch(handleError));
  $("refreshBtn").addEventListener("click", () => refresh().catch(handleError));
  $("scanBtn").addEventListener("click", () => scanEvents().catch(handleError));
  $("lookupIdBtn").addEventListener("click", () => lookupById().catch(handleError));
  $("lookupAddressBtn").addEventListener("click", () => lookupByAddress().catch(handleError));
  $("clearLogBtn").addEventListener("click", () => $("log").textContent = "Ready.");

  loadHeader().catch(handleError);
})();
