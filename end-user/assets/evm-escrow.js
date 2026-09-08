/*
 * Osnias Clearing — EVM Escrow Client
 * File: /end-user/assets/evm-escrow.js
 * Version: 1.2.2
 * Date: 2026-09-08
 *
 * Uses /end-user/assets/wallet-connect.js
 * Ethereum Sepolia technical testnet only.
 *
 * Wallet metrics:
 * - USDC in wallet      = Circle Sepolia USDC balance of connected wallet
 * - USDC under escrow   = balance of the connected wallet's ClientEscrowAccount
 * - USDC able to MINT   = max(wallet USDC - wallet escrow USDC, 0)
 */

(() => {
  "use strict";

  const C = window.OSNIAS_EVM_CONFIG;
  if (!C) throw new Error("OSNIAS_EVM_CONFIG is missing.");

  const ESCROW_ABI = [
    "function owner() view returns (address)",
    "function osniasController() view returns (address)",
    "function usdcAddress() pure returns (address)",
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

  let signer = null;
  let escrow = null;
  let usdc = null;
  let connectedAddress = null;

  const $ = id => document.getElementById(id);
  const fmt = v => ethers.formatUnits(v, C.usdcDecimals) + " USDC";

  const FALLBACK_RPC = "https://ethereum-sepolia-rpc.publicnode.com";
  let fallbackProvider = null;

  function getFallbackProvider() {
    if (!fallbackProvider) {
      fallbackProvider = new ethers.JsonRpcProvider(FALLBACK_RPC, {
        name: "sepolia",
        chainId: 11155111
      });
    }
    return fallbackProvider;
  }

  async function readWalletUsdcBalance(address) {
    /*
     * Primary path: read through the connected wallet provider.
     * Fallback path: public Sepolia JSON-RPC, read-only.
     */
    try {
      return {
        balance: await usdc.balanceOf(address),
        source: "wallet-provider",
        error: null
      };
    } catch (primaryError) {
      console.warn("Primary USDC balanceOf failed:", primaryError);

      try {
        const readOnlyUsdc = new ethers.Contract(
          C.usdcAddress,
          USDC_ABI,
          getFallbackProvider()
        );

        return {
          balance: await readOnlyUsdc.balanceOf(address),
          source: "fallback-rpc",
          error: primaryError
        };
      } catch (fallbackError) {
        console.error("Fallback USDC balanceOf failed:", fallbackError);

        const error = new Error(
          "USDC balance read failed on wallet provider and fallback Sepolia RPC."
        );

        error.primaryError = primaryError;
        error.fallbackError = fallbackError;
        throw error;
      }
    }
  }

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

  function escrowStorageKey(address) {
    return "osnias:evm:clientEscrowId:" + String(address || "").toLowerCase();
  }

  function rememberEscrowId(address, escrowId) {
    if (!address || !escrowId) return;
    localStorage.setItem(escrowStorageKey(address), escrowId);
  }

  function rememberedEscrowId(address) {
    if (!address) return null;
    return localStorage.getItem(escrowStorageKey(address));
  }

  async function bindContracts() {
    if (!window.OsniasWallet) throw new Error("wallet-connect.js is not ready.");

    signer = await window.OsniasWallet.getSigner();
    connectedAddress = await signer.getAddress();

    escrow = new ethers.Contract(C.escrowAddress, ESCROW_ABI, signer);
    usdc = new ethers.Contract(C.usdcAddress, USDC_ABI, signer);

    if ($("walletAddress")) $("walletAddress").textContent = connectedAddress;
    if ($("networkLabel")) $("networkLabel").textContent = "Ethereum Sepolia";
    if ($("networkDot")) $("networkDot").className = "dot ok";
  }

  async function connectedWalletEscrow() {
    const knownId = rememberedEscrowId(connectedAddress);
    if (!knownId) return null;

    const account = await escrow.getClientEscrow(knownId);
    if (account.escrowId === ethers.ZeroHash) return null;

    if (String(account.clientWallet).toLowerCase() !== String(connectedAddress).toLowerCase()) {
      localStorage.removeItem(escrowStorageKey(connectedAddress));
      return null;
    }

    return account;
  }

  async function refreshWalletMetrics() {
    await bindContracts();

    /*
     * Wallet USDC is independent from Osnias escrow accounting.
     * Read and display it first so an escrow-side lookup failure can never
     * prevent the user's Circle USDC wallet balance from being shown.
     */
    let walletRead;

    try {
      walletRead = await readWalletUsdcBalance(connectedAddress);
    } catch (error) {
      const primary =
        error?.primaryError?.shortMessage ||
        error?.primaryError?.message ||
        "unknown primary error";

      const fallback =
        error?.fallbackError?.shortMessage ||
        error?.fallbackError?.message ||
        "unknown fallback error";

      if ($("walletUsdc")) {
        $("walletUsdc").textContent = "Error";
      }

      if ($("walletEscrowHint")) {
        $("walletEscrowHint").textContent =
          "USDC read error — see browser console.";
      }

      console.error("USDC WALLET READ FAILURE", {
        wallet: connectedAddress,
        usdc: C.usdcAddress,
        primary,
        fallback
      });

      throw error;
    }

    const walletBalance = walletRead.balance;

    if ($("walletUsdc")) {
      $("walletUsdc").textContent = fmt(walletBalance);
      $("walletUsdc").title =
        walletRead.source === "fallback-rpc"
          ? "Read via fallback Sepolia RPC"
          : "Read via connected wallet provider";
    }

    if (walletRead.source === "fallback-rpc") {
      console.info(
        "USDC wallet balance read via fallback Sepolia RPC.",
        {
          wallet: connectedAddress,
          usdc: C.usdcAddress
        }
      );
    }

    /*
     * The contract does not expose a reverse wallet -> escrowId index.
     * We therefore use the escrowId previously selected/created by this wallet.
     */
    let account = null;

    try {
      account = await connectedWalletEscrow();
    } catch (error) {
      console.warn("Unable to read connected wallet escrow:", error);
    }

    if (!account) {
      if ($("walletEscrowUsdc")) $("walletEscrowUsdc").textContent = "0 USDC";

      /*
       * No escrow account is currently associated in this browser.
       * The immediate theoretical capacity therefore equals the wallet balance.
       * Once an escrowId is selected/created, the actual escrow balance is
       * subtracted automatically.
       */
      if ($("topMintAvailable")) $("topMintAvailable").textContent = fmt(walletBalance);

      if ($("walletEscrowHint")) {
        $("walletEscrowHint").textContent =
          "No client escrow selected for this wallet.";
      }

      return;
    }

    const escrowBalance = account.balance;
    const ableToMint =
      walletBalance > escrowBalance
        ? walletBalance - escrowBalance
        : 0n;

    if ($("walletEscrowUsdc")) {
      $("walletEscrowUsdc").textContent = fmt(escrowBalance);
    }

    if ($("topMintAvailable")) {
      $("topMintAvailable").textContent = fmt(ableToMint);
    }

    if ($("walletEscrowHint")) {
      $("walletEscrowHint").textContent =
        "Client escrow: " + account.escrowId;
    }
  }

  async function connect() {
    try {
      await window.OsniasWallet.connect();
      await bindContracts();
      await refresh();
    } catch (e) {
      if ($("networkLabel")) $("networkLabel").textContent = "Connection failed";
      if ($("networkDot")) $("networkDot").className = "dot bad";
      alert(e.message || e);
    }
  }

  async function disconnect() {
    try {
      await window.OsniasWallet.disconnect();
    } finally {
      signer = escrow = usdc = null;
      connectedAddress = null;
      if ($("walletAddress")) $("walletAddress").textContent = "—";
      if ($("walletUsdc")) $("walletUsdc").textContent = "—";
      if ($("walletEscrowUsdc")) $("walletEscrowUsdc").textContent = "—";
      if ($("topMintAvailable")) $("topMintAvailable").textContent = "—";
      if ($("networkLabel")) $("networkLabel").textContent = "Disconnected";
      if ($("networkDot")) $("networkDot").className = "dot";
      updateConnectButton(false);
    }
  }

  function updateConnectButton(connected) {
    const btn = $("connectBtn");
    if (!btn) return;
    btn.textContent = connected ? "Disconnect Wallet" : "Connect Wallet";
    btn.onclick = connected ? disconnect : connect;
  }

  async function refresh() {
    /*
     * First priority: user-facing wallet metrics.
     * These must remain available even if an ancillary configuration read fails.
     */
    try {
      await refreshWalletMetrics();
      updateConnectButton(true);
    } catch (error) {
      console.error("Wallet metric refresh failed:", error);

      if ($("walletUsdc")) $("walletUsdc").textContent = "Error";
      if ($("networkLabel")) $("networkLabel").textContent = "USDC read error";
      if ($("networkDot")) $("networkDot").className = "dot bad";

      if ($("walletEscrowHint") && !$("walletEscrowHint").textContent.includes("USDC read error")) {
        $("walletEscrowHint").textContent =
          "USDC read failed. Open browser console for diagnostic details.";
      }
    }

    /*
     * Secondary diagnostics: contract configuration/accounting.
     * A failure here must not blank the wallet metrics above.
     */
    try {
      await bindContracts();

      const [owner, controller, usdcAddr, solvent, unallocated] =
        await Promise.all([
          escrow.owner(),
          escrow.osniasController(),
          escrow.usdcAddress(),
          escrow.accountingSolvent(),
          escrow.unallocatedUSDC()
        ]);

      if ($("ownerValue")) $("ownerValue").textContent = owner;
      if ($("controllerValue")) $("controllerValue").textContent = controller;
      if ($("usdcValue")) $("usdcValue").textContent = usdcAddr;

      if ($("unallocatedValue")) {
        $("unallocatedValue").textContent = fmt(unallocated);
      }

      if ($("solvent")) {
        $("solvent").textContent = solvent ? "YES" : "NO";
        $("solvent").style.color = solvent ? "var(--ok)" : "var(--bad)";
      }
    } catch (error) {
      console.error("Escrow diagnostic refresh failed:", error);
    }
  }

  async function lookupClient() {
    try {
      await bindContracts();
      const id = toBytes32($("lookupEscrowId").value);
      const a = await escrow.getClientEscrow(id);

      if (a.escrowId === ethers.ZeroHash) {
        setStatus("lookupStatus", "No client escrow found.", "bad");
        return;
      }

      const isConnectedWallet =
        String(a.clientWallet).toLowerCase() === String(connectedAddress).toLowerCase();

      if (isConnectedWallet) {
        rememberEscrowId(connectedAddress, a.escrowId);
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
        "Connected wallet: " + (isConnectedWallet ? "YES" : "NO"),
        "Created: " + new Date(Number(a.createdAt) * 1000).toISOString()
      ].join("\n"), "ok");

      if (isConnectedWallet) {
        await refreshWalletMetrics();
      }
    } catch (e) {
      setStatus("lookupStatus", e.shortMessage || e.message || String(e), "bad");
    }
  }

  async function createClient() {
    try {
      await bindContracts();
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

      if (String(cw).toLowerCase() === String(connectedAddress).toLowerCase()) {
        rememberEscrowId(connectedAddress, eid);
        await refreshWalletMetrics();
      }
    } catch (e) {
      setStatus("createStatus", e.shortMessage || e.message || String(e), "bad");
    }
  }

  async function approveUsdc() {
    try {
      await bindContracts();
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
      await bindContracts();
      const eid = toBytes32($("fundEscrowId").value);
      const account = await escrow.getClientEscrow(eid);

      if (account.escrowId === ethers.ZeroHash) {
        throw new Error("Unknown client escrow.");
      }

      if (String(account.clientWallet).toLowerCase() !== String(connectedAddress).toLowerCase()) {
        throw new Error("This escrow does not belong to the connected wallet.");
      }

      const amount = ethers.parseUnits($("fundAmount").value.trim(), C.usdcDecimals);
      const allowance = await usdc.allowance(connectedAddress, C.escrowAddress);

      if (allowance < amount) throw new Error("USDC allowance too low. Approve first.");

      setStatus("fundStatus", "Waiting for funding confirmation...");
      const tx = await escrow.fundClientEscrow(eid, amount);
      await tx.wait();

      rememberEscrowId(connectedAddress, eid);
      setStatus("fundStatus", "Escrow funded.\n" + tx.hash, "ok");
      await refresh();
    } catch (e) {
      setStatus("fundStatus", e.shortMessage || e.message || String(e), "bad");
    }
  }

  async function readAvailability() {
    try {
      await bindContracts();
      const oid = toBytes32($("availOsniasId").value);
      const eid = toBytes32($("availEscrowId").value);

      const [m, b] = await Promise.all([
        escrow.availableForMint(oid, eid),
        escrow.availableForBurn(oid, eid)
      ]);

      $("mintAvailable").textContent = fmt(m);
      $("burnAvailable").textContent = fmt(b);
    } catch (e) {
      $("mintAvailable").textContent = "Error";
      $("burnAvailable").textContent = "Error";
      alert(e.shortMessage || e.message || String(e));
    }
  }

  function applyWalletState(detail) {
    const connected = Boolean(detail?.connected);
    updateConnectButton(connected);

    if (connected) {
      connectedAddress = detail.account || null;
      if ($("walletAddress")) $("walletAddress").textContent = connectedAddress || "—";
      if ($("networkLabel")) $("networkLabel").textContent = "Ethereum Sepolia";
      if ($("networkDot")) $("networkDot").className = "dot ok";
      refresh();
    } else {
      if ($("walletAddress")) $("walletAddress").textContent = "—";
      if ($("walletUsdc")) $("walletUsdc").textContent = "—";
      if ($("walletEscrowUsdc")) $("walletEscrowUsdc").textContent = "—";
      if ($("topMintAvailable")) $("topMintAvailable").textContent = "—";
      if ($("networkLabel")) $("networkLabel").textContent = "Disconnected";
      if ($("networkDot")) $("networkDot").className = "dot";
    }
  }

  function init() {
    updateConnectButton(false);

    if ($("refreshBtn")) $("refreshBtn").onclick = refresh;
    if ($("lookupBtn")) $("lookupBtn").onclick = lookupClient;
    if ($("createBtn")) $("createBtn").onclick = createClient;
    if ($("approveBtn")) $("approveBtn").onclick = approveUsdc;
    if ($("fundBtn")) $("fundBtn").onclick = fundEscrow;
    if ($("availabilityBtn")) $("availabilityBtn").onclick = readAvailability;

    document.addEventListener("osnias:wallet-connected", e => applyWalletState(e.detail));
    document.addEventListener("osnias:wallet-restored", e => applyWalletState(e.detail));
    document.addEventListener("osnias:wallet-account-changed", e => applyWalletState(e.detail));
    document.addEventListener("osnias:wallet-chain-changed", e => applyWalletState(e.detail));
    document.addEventListener("osnias:wallet-disconnected", () => applyWalletState({ connected:false }));
    document.addEventListener("osnias:wallet-wrong-network", () => {
      updateConnectButton(false);
      if ($("networkLabel")) $("networkLabel").textContent = "Wrong network";
      if ($("networkDot")) $("networkDot").className = "dot bad";
    });

    if (window.OsniasWallet) applyWalletState(window.OsniasWallet.getState());
  }

  window.OsniasEvmEscrow = Object.freeze({
    connect, disconnect, refresh, lookupClient, createClient,
    approveUsdc, fundEscrow, readAvailability
  });

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init, { once:true });
  } else {
    init();
  }
})();
