/*
 * Osnias Clearing — EVM Escrow Client
 * Ethereum Sepolia technical testnet only.
 */
(() => {
  "use strict";

  const C = window.OSNIAS_EVM_CONFIG;
  if (!C) throw new Error("OSNIAS_EVM_CONFIG is missing.");

  const ESCROW_ABI = [
    "function owner() view returns (address)",
    "function osniasController() view returns (address)",
    "function usdcAddress() pure returns (address)",
    "function totalEscrow() view returns (uint256)",
    "function accountingSolvent() view returns (bool)",
    "function unallocatedUSDC() view returns (uint256)",
    "function createClientEscrow(bytes32 osniasId,bytes32 escrowId,address clientWallet)",
    "function fundClientEscrow(bytes32 escrowId,uint256 amount)",
    "function getClientEscrow(bytes32 escrowId) view returns (tuple(bytes32 osniasId,bytes32 escrowId,address clientWallet,uint256 balance,uint256 mintCommitted,uint256 mintReserved,uint256 burnReserved,uint64 createdAt,bool active))",
    "function availableForMint(bytes32 osniasId,bytes32 escrowId) view returns (uint256)",
    "function availableForBurn(bytes32 osniasId,bytes32 escrowId) view returns (uint256)"
  ];

  const USDC_ABI = [
    "function approve(address spender,uint256 amount) returns (bool)",
    "function allowance(address owner,address spender) view returns (uint256)",
    "function balanceOf(address account) view returns (uint256)"
  ];

  let provider, signer, escrow, usdc, connectedAddress;
  const $ = id => document.getElementById(id);

  function setStatus(id, msg, type="") {
    const el = $(id);
    if (!el) return;
    el.textContent = msg;
    el.className = "status" + (type ? " " + type : "");
  }

  function toBytes32(v) {
    v = (v || "").trim();
    if (!v) throw new Error("Missing identifier.");
    if (/^0x[0-9a-fA-F]{64}$/.test(v)) return v;
    return ethers.keccak256(ethers.toUtf8Bytes(v));
  }

  function fmt(v) {
    return ethers.formatUnits(v, C.usdcDecimals) + " USDC";
  }

  async function ensureWallet() {
    if (!window.ethereum) throw new Error("No EVM browser wallet detected.");

    if (!provider) provider = new ethers.BrowserProvider(window.ethereum);
    if (!signer) signer = await provider.getSigner();
    if (!connectedAddress) connectedAddress = await signer.getAddress();

    const network = await provider.getNetwork();
    if (network.chainId !== C.chainId) {
      throw new Error("Wrong network. Connect to Ethereum Sepolia.");
    }

    escrow = new ethers.Contract(C.escrowAddress, ESCROW_ABI, signer);
    usdc = new ethers.Contract(C.usdcAddress, USDC_ABI, signer);

    if ($("walletAddress")) $("walletAddress").textContent = connectedAddress;
    if ($("networkLabel")) $("networkLabel").textContent = C.chainName;
    if ($("networkDot")) $("networkDot").className = "dot ok";
  }

  async function connect() {
    try {
      await window.ethereum.request({ method: "eth_requestAccounts" });
      provider = new ethers.BrowserProvider(window.ethereum);
      signer = await provider.getSigner();
      connectedAddress = await signer.getAddress();
      await ensureWallet();
      await refresh();
    } catch (e) {
      if ($("networkLabel")) $("networkLabel").textContent = "Connection failed";
      if ($("networkDot")) $("networkDot").className = "dot bad";
      alert(e.message || e);
    }
  }

  async function refresh() {
    try {
      await ensureWallet();

      const [owner, controller, usdcAddr, total, solvent, unallocated, walletBalance] =
        await Promise.all([
          escrow.owner(),
          escrow.osniasController(),
          escrow.usdcAddress(),
          escrow.totalEscrow(),
          escrow.accountingSolvent(),
          escrow.unallocatedUSDC(),
          usdc.balanceOf(connectedAddress)
        ]);

      if ($("ownerValue")) $("ownerValue").textContent = owner;
      if ($("controllerValue")) $("controllerValue").textContent = controller;
      if ($("usdcValue")) $("usdcValue").textContent = usdcAddr;
      if ($("walletUsdc")) $("walletUsdc").textContent = fmt(walletBalance);
      if ($("totalEscrow")) $("totalEscrow").textContent = fmt(total);
      if ($("unallocatedValue")) $("unallocatedValue").textContent = fmt(unallocated);

      if ($("solvent")) {
        $("solvent").textContent = solvent ? "YES" : "NO";
        $("solvent").style.color = solvent ? "var(--ok)" : "var(--bad)";
      }
    } catch (e) {
      console.error(e);
    }
  }

  async function lookupClient() {
    try {
      await ensureWallet();
      const id = toBytes32($("lookupEscrowId").value);
      const a = await escrow.getClientEscrow(id);

      if (a.escrowId === ethers.ZeroHash) {
        setStatus("lookupStatus", "No client escrow found.", "bad");
        return;
      }

      setStatus("lookupStatus", [
        "Osnias ID: " + a.osniasId,
        "Escrow ID: " + a.escrowId,
        "Client wallet: " + a.clientWallet,
        "Balance: " + fmt(a.balance),
        "MINT committed: " + fmt(a.mintCommitted),
        "MINT reserved: " + fmt(a.mintReserved),
        "BURN reserved: " + fmt(a.burnReserved),
        "Active: " + a.active,
        "Created: " + new Date(Number(a.createdAt) * 1000).toISOString()
      ].join("\n"), "ok");

      const mint = await escrow.availableForMint(a.osniasId, a.escrowId);
      if ($("topMintAvailable")) $("topMintAvailable").textContent = fmt(mint);
    } catch (e) {
      setStatus("lookupStatus", e.shortMessage || e.message || String(e), "bad");
    }
  }

  async function createClient() {
    try {
      await ensureWallet();

      const oid = toBytes32($("createOsniasId").value);
      const eid = toBytes32($("createEscrowId").value);
      const cw = ethers.getAddress($("createWallet").value.trim());

      setStatus("createStatus", "Waiting for wallet confirmation...");
      const tx = await escrow.createClientEscrow(oid, eid, cw);
      setStatus("createStatus", "Transaction sent:\n" + tx.hash);
      await tx.wait();

      setStatus("createStatus", "Client escrow created.\n" + tx.hash, "ok");

      $("lookupEscrowId").value = $("createEscrowId").value;
      $("fundEscrowId").value = $("createEscrowId").value;
      $("availOsniasId").value = $("createOsniasId").value;
      $("availEscrowId").value = $("createEscrowId").value;
    } catch (e) {
      setStatus("createStatus", e.shortMessage || e.message || String(e), "bad");
    }
  }

  async function approveUsdc() {
    try {
      await ensureWallet();
      const amount = ethers.parseUnits($("fundAmount").value.trim(), C.usdcDecimals);
      if (amount <= 0n) throw new Error("Amount must be > 0.");

      setStatus("fundStatus", "Waiting for USDC approval...");
      const tx = await usdc.approve(C.escrowAddress, amount);
      await tx.wait();

      setStatus("fundStatus", "USDC approval confirmed.\n" + tx.hash, "ok");
    } catch (e) {
      setStatus("fundStatus", e.shortMessage || e.message || String(e), "bad");
    }
  }

  async function fundEscrow() {
    try {
      await ensureWallet();

      const eid = toBytes32($("fundEscrowId").value);
      const amount = ethers.parseUnits($("fundAmount").value.trim(), C.usdcDecimals);
      const allowance = await usdc.allowance(connectedAddress, C.escrowAddress);

      if (allowance < amount) throw new Error("USDC allowance too low. Approve first.");

      setStatus("fundStatus", "Waiting for funding confirmation...");
      const tx = await escrow.fundClientEscrow(eid, amount);
      await tx.wait();

      setStatus("fundStatus", "Escrow funded.\n" + tx.hash, "ok");
      await refresh();
    } catch (e) {
      setStatus("fundStatus", e.shortMessage || e.message || String(e), "bad");
    }
  }

  async function readAvailability() {
    try {
      await ensureWallet();

      const oid = toBytes32($("availOsniasId").value);
      const eid = toBytes32($("availEscrowId").value);

      const [m, b] = await Promise.all([
        escrow.availableForMint(oid, eid),
        escrow.availableForBurn(oid, eid)
      ]);

      if ($("mintAvailable")) $("mintAvailable").textContent = fmt(m);
      if ($("burnAvailable")) $("burnAvailable").textContent = fmt(b);
      if ($("topMintAvailable")) $("topMintAvailable").textContent = fmt(m);
    } catch (e) {
      if ($("mintAvailable")) $("mintAvailable").textContent = "Error";
      if ($("burnAvailable")) $("burnAvailable").textContent = "Error";
      alert(e.shortMessage || e.message || String(e));
    }
  }

  function init() {
    if ($("connectBtn")) $("connectBtn").onclick = connect;
    if ($("refreshBtn")) $("refreshBtn").onclick = refresh;
    if ($("lookupBtn")) $("lookupBtn").onclick = lookupClient;
    if ($("createBtn")) $("createBtn").onclick = createClient;
    if ($("approveBtn")) $("approveBtn").onclick = approveUsdc;
    if ($("fundBtn")) $("fundBtn").onclick = fundEscrow;
    if ($("availabilityBtn")) $("availabilityBtn").onclick = readAvailability;

    if (window.ethereum) {
      window.ethereum.on?.("accountsChanged", () => location.reload());
      window.ethereum.on?.("chainChanged", () => location.reload());
    }
  }

  window.OsniasEvmEscrow = { connect, refresh, lookupClient, createClient, approveUsdc, fundEscrow, readAvailability };
  document.addEventListener("DOMContentLoaded", init);
})();
