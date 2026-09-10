(() => {
  "use strict";

  const CHAIN_ID_DEC = 11155111;
  const CHAIN_ID_HEX = "0xaa36a7";
  const KK = "90";
  const NNN = "017";

  const REGISTRAR_ADDRESS = "0xD14C1b90635521A1C586ec9B871D797458cB1Db8";
  const REGISTRY_ADDRESS = "0xf7a0cAd5DDE44cA3E8B61C0d08A9019c3944Cb55";

  const REGISTRAR_ABI = [
    "function owner() view returns (address)",
    "function registerUser(uint64 osniasId,address localAddress) returns (bytes32 registrationRequestId)",
    "event RegistrationFinalized(bytes32 indexed registrationRequestId,uint64 indexed osniasId,address indexed localAddress,uint8 kk,uint16 nnn)"
  ];

  const REGISTRY_ABI = [
    "function getById(uint64 osniasId) view returns ((address localAddress,uint64 osniasId,uint8 status,uint24 reserved))",
    "function getByLocalAddress(address localAddress) view returns (uint64 osniasId,uint8 status)",
    "function isActivePair(uint64 osniasId,address localAddress) view returns (bool)"
  ];

  const STATUS = [
    "UNUSED",
    "PENDING",
    "ACTIVE",
    "SUSPENDED",
    "CLOSED",
    "REVOKED",
    "ARCHIVED"
  ];

  let provider = null;
  let signer = null;
  let registrar = null;
  let registry = null;
  let currentOsniasId = null;

  const $ = (id) => document.getElementById(id);

  const log = (message) => {
    const stamp = new Date().toLocaleTimeString();
    $("log").textContent += `\n[${stamp}] ${message}`;
    $("log").scrollTop = $("log").scrollHeight;
  };

  function onlyDigits(value) {
    return value.replace(/\D/g, "");
  }

  function calculateOsniasId(user9) {
    if (!/^\d{9}$/.test(user9)) {
      throw new Error("AAAAAAAAA must contain exactly 9 decimal digits.");
    }

    const payload = `${KK}${NNN}${user9}`;
    const base00 = BigInt(payload + "00");
    const remainder = base00 % 97n;
    const xx = 98n - remainder;
    const checksum = xx.toString().padStart(2, "0");
    const full = payload + checksum;
    const numeric = BigInt(full);

    if (numeric % 97n !== 1n) {
      throw new Error("Internal MOD-97 validation failed.");
    }

    return {
      display: `${KK}-${NNN}-${user9}-${checksum}`,
      machine: full,
      bigint: numeric,
      checksum
    };
  }

  function refreshIdPreview() {
    const input = $("participantNumber");
    input.value = onlyDigits(input.value).slice(0, 9);

    if (input.value.length !== 9) {
      currentOsniasId = null;
      $("osniasIdDisplay").textContent = "90-017-_________-__";
      $("osniasIdMachine").textContent = "—";
      $("checksumStatus").className = "status neutral";
      $("checksumStatus").textContent = "Waiting for 9 digits";
      updateButtons();
      return;
    }

    try {
      currentOsniasId = calculateOsniasId(input.value);
      $("osniasIdDisplay").textContent = currentOsniasId.display;
      $("osniasIdMachine").textContent = currentOsniasId.machine;
      $("checksumStatus").className = "status good";
      $("checksumStatus").textContent = `MOD-97 valid · XX=${currentOsniasId.checksum}`;
    } catch (err) {
      currentOsniasId = null;
      $("checksumStatus").className = "status bad";
      $("checksumStatus").textContent = err.message;
    }

    updateButtons();
  }

  function validLocalAddress() {
    try {
      return ethers.isAddress($("localAddress").value.trim());
    } catch {
      return false;
    }
  }

  function updateButtons() {
    const connected = !!signer;
    const ready = connected && !!currentOsniasId && validLocalAddress();

    $("disconnectBtn").disabled = !connected;
    $("registerBtn").disabled = !ready;
    $("lookupBtn").disabled = !currentOsniasId || !validLocalAddress();
  }

  async function ensureSepolia() {
    const network = await provider.getNetwork();

    if (Number(network.chainId) === CHAIN_ID_DEC) {
      return true;
    }

    try {
      await window.ethereum.request({
        method: "wallet_switchEthereumChain",
        params: [{ chainId: CHAIN_ID_HEX }]
      });

      provider = new ethers.BrowserProvider(window.ethereum);
      signer = await provider.getSigner();
      return true;
    } catch (err) {
      throw new Error("Please switch wallet to Ethereum Sepolia.");
    }
  }

  async function connectWallet() {
    if (!window.ethereum) {
      throw new Error("No injected EVM wallet detected.");
    }

    await window.ethereum.request({ method: "eth_requestAccounts" });

    provider = new ethers.BrowserProvider(window.ethereum);
    signer = await provider.getSigner();

    await ensureSepolia();

    registrar = new ethers.Contract(
      REGISTRAR_ADDRESS,
      REGISTRAR_ABI,
      signer
    );

    registry = new ethers.Contract(
      REGISTRY_ADDRESS,
      REGISTRY_ABI,
      provider
    );

    const address = await signer.getAddress();
    const owner = await registrar.owner();

    $("walletAddress").textContent = address;
    $("nodeOwner").textContent = owner;

    $("networkDot").classList.add("good");
    $("networkState").textContent = "Ethereum Sepolia · connected";

    if (address.toLowerCase() === owner.toLowerCase()) {
      log(`Node owner connected: ${address}`);
    } else {
      log(`Warning: connected wallet is not Node Registrar owner. Owner=${owner}`);
    }

    updateButtons();
  }

  function disconnectLocal() {
    provider = null;
    signer = null;
    registrar = null;
    registry = null;

    $("walletAddress").textContent = "—";
    $("nodeOwner").textContent = "—";
    $("networkDot").classList.remove("good");
    $("networkState").textContent = "Wallet not connected";

    updateButtons();
    log("Local session disconnected.");
  }

  async function registerUser() {
    if (!currentOsniasId || !validLocalAddress()) {
      throw new Error("Generate a valid Osnias-ID and enter a valid local address.");
    }

    if (!signer || !registrar) {
      throw new Error("Connect the Node owner wallet.");
    }

    await ensureSepolia();

    const localAddress = $("localAddress").value.trim();
    const signerAddress = await signer.getAddress();
    const owner = await registrar.owner();

    if (signerAddress.toLowerCase() !== owner.toLowerCase()) {
      throw new Error("Connected wallet is not the Node Registrar owner.");
    }

    log(`Submitting ${currentOsniasId.display} -> ${localAddress}`);

    $("registerBtn").disabled = true;

    const tx = await registrar.registerUser(
      currentOsniasId.bigint,
      localAddress
    );

    $("txBox").classList.remove("hidden");
    $("txHash").textContent = tx.hash;

    log(`Transaction sent: ${tx.hash}`);

    const receipt = await tx.wait();
    log(`Transaction confirmed in block ${receipt.blockNumber}`);

    let requestId = "—";

    for (const entry of receipt.logs) {
      try {
        const parsed = registrar.interface.parseLog(entry);
        if (parsed && parsed.name === "RegistrationFinalized") {
          requestId = parsed.args.registrationRequestId;
          break;
        }
      } catch {}
    }

    $("requestId").textContent = requestId;
    log(`Registration FINALIZED. requestId=${requestId}`);

    await readRegistry();
    updateButtons();
  }

  async function readRegistry() {
    if (!currentOsniasId || !validLocalAddress()) {
      throw new Error("A valid Osnias-ID and local address are required.");
    }

    if (!provider) {
      provider = new ethers.JsonRpcProvider("https://ethereum-sepolia-rpc.publicnode.com");
    }

    if (!registry) {
      registry = new ethers.Contract(
        REGISTRY_ADDRESS,
        REGISTRY_ABI,
        provider
      );
    }

    const localAddress = $("localAddress").value.trim();
    const participant = await registry.getById(currentOsniasId.bigint);
    const activePair = await registry.isActivePair(
      currentOsniasId.bigint,
      localAddress
    );

    const statusNumber = Number(participant.status);

    $("readId").textContent =
      participant.osniasId === 0n
        ? "Not registered"
        : currentOsniasId.display;

    $("readAddress").textContent = participant.localAddress;
    $("readStatus").textContent =
      `${statusNumber} · ${STATUS[statusNumber] ?? "UNKNOWN"}`;

    $("readActivePair").textContent =
      activePair ? "YES" : "NO";

    log(
      `Registry read: ID=${participant.osniasId.toString()} ` +
      `address=${participant.localAddress} ` +
      `status=${STATUS[statusNumber] ?? statusNumber} ` +
      `activePair=${activePair}`
    );
  }

  async function refreshAll() {
    if (signer) {
      await connectWallet();
    }

    if (currentOsniasId && validLocalAddress()) {
      await readRegistry();
    }
  }

  function handleError(err) {
    console.error(err);
    const message =
      err?.shortMessage ||
      err?.reason ||
      err?.message ||
      String(err);

    log(`ERROR: ${message}`);
    alert(message);
    updateButtons();
  }

  $("participantNumber").addEventListener("input", refreshIdPreview);
  $("localAddress").addEventListener("input", updateButtons);

  $("connectBtn").addEventListener("click", () =>
    connectWallet().catch(handleError)
  );

  $("disconnectBtn").addEventListener("click", disconnectLocal);

  $("refreshBtn").addEventListener("click", () =>
    refreshAll().catch(handleError)
  );

  $("registerBtn").addEventListener("click", () =>
    registerUser().catch(handleError)
  );

  $("lookupBtn").addEventListener("click", () =>
    readRegistry().catch(handleError)
  );

  $("clearLogBtn").addEventListener("click", () => {
    $("log").textContent = "Ready.";
  });

  if (window.ethereum) {
    window.ethereum.on?.("accountsChanged", () => {
      disconnectLocal();
      log("Wallet account changed.");
    });

    window.ethereum.on?.("chainChanged", () => {
      disconnectLocal();
      log("Wallet network changed.");
    });
  }

  refreshIdPreview();
})();
