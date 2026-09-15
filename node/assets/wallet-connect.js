/*
 * Osnias Clearing — Wallet Connect
 * Shared EIP-1193 wallet connection layer
 *
 * File: /end-user/assets/wallet-connect.js
 * Version: 1.2.1
 *
 * Default network: Sei Atlantic-2 Testnet.
 * A page may override the target network BEFORE this file loads by defining:
 *
 * window.OSNIAS_WALLET_NETWORK = {
 *   chainId: 11155111,
 *   chainIdHex: "0xaa36a7",
 *   chainName: "Ethereum Sepolia",
 *   nativeCurrency: { name: "Sepolia ETH", symbol: "ETH", decimals: 18 },
 *   rpcUrls: ["https://ethereum-sepolia-rpc.publicnode.com"],
 *   blockExplorerUrls: ["https://sepolia.etherscan.io"]
 * };
 *
 * Security:
 * - no private key handling;
 * - no automatic signing;
 * - target chain verified before returning a signer;
 * - explicit application disconnect supported.
 */

(() => {
  "use strict";

  const VERSION = "1.2.1";

  const DEFAULT_NETWORK = Object.freeze({
    chainId: 1328,
    chainIdHex: "0x530",
    chainName: "Sei Atlantic-2 Testnet",
    nativeCurrency: Object.freeze({
      name: "Sei",
      symbol: "SEI",
      decimals: 18
    }),
    rpcUrls: Object.freeze(["https://evm-rpc-testnet.sei-apis.com"]),
    blockExplorerUrls: Object.freeze(["https://testnet.seiscan.io"])
  });

  function normalizeNetwork(candidate) {
    if (!candidate) return DEFAULT_NETWORK;

    const chainId = Number(candidate.chainId);
    if (!Number.isInteger(chainId) || chainId <= 0) {
      throw new Error("Invalid OSNIAS_WALLET_NETWORK.chainId");
    }

    return Object.freeze({
      chainId,
      chainIdHex: candidate.chainIdHex || ("0x" + chainId.toString(16)),
      chainName: candidate.chainName || `EVM Chain ${chainId}`,
      nativeCurrency: Object.freeze(candidate.nativeCurrency || {
        name: "Native",
        symbol: "ETH",
        decimals: 18
      }),
      rpcUrls: Object.freeze([...(candidate.rpcUrls || [])]),
      blockExplorerUrls: Object.freeze([...(candidate.blockExplorerUrls || [])])
    });
  }

  const NETWORK = normalizeNetwork(window.OSNIAS_WALLET_NETWORK);

  const state = {
    eip1193: null,
    browserProvider: null,
    signer: null,
    account: null,
    chainId: null,
    connected: false,
    walletName: null
  };

  class WalletConnectError extends Error {
    constructor(message, code = null, data = null) {
      super(message);
      this.name = "WalletConnectError";
      this.code = code;
      this.data = data;
    }
  }

  function requireEthers() {
    if (
      typeof window.ethers === "undefined" ||
      typeof window.ethers.BrowserProvider !== "function"
    ) {
      throw new WalletConnectError("ethers.js v6 is required by wallet-connect.js.");
    }
  }

  function isMobile() {
    return /Android|iPhone|iPad|iPod/i.test(navigator.userAgent || "");
  }

  function isMetaMaskProvider(provider) {
    return Boolean(provider?.isMetaMask);
  }

  function isTrustProvider(provider) {
    return Boolean(provider?.isTrust || provider?.isTrustWallet || provider?.isTrustWalletBrowser);
  }

  function providerName(provider) {
    if (isTrustProvider(provider)) return "Trust Wallet";
    if (isMetaMaskProvider(provider)) return "MetaMask";
    return "Injected Wallet";
  }

  function getInjectedProvider() {
    const ethereum = window.ethereum;
    if (!ethereum) return null;

    if (Array.isArray(ethereum.providers) && ethereum.providers.length) {
      const trust = ethereum.providers.find(isTrustProvider);
      if (trust) return trust;
      const metamask = ethereum.providers.find(isMetaMaskProvider);
      if (metamask) return metamask;
      return ethereum.providers[0];
    }

    return ethereum;
  }

  function normalizeChainId(value) {
    if (typeof value === "number") return value;
    if (typeof value === "bigint") return Number(value);

    const text = String(value || "");
    if (/^0x[0-9a-fA-F]+$/.test(text)) return Number(BigInt(text));
    if (/^\d+$/.test(text)) return Number(text);

    throw new WalletConnectError(`Invalid chain ID returned by wallet: ${value}`);
  }

  function normalizeAccount(address) {
    requireEthers();
    try {
      return window.ethers.getAddress(address);
    } catch {
      throw new WalletConnectError(`Invalid wallet account returned: ${address}`);
    }
  }

  async function request(provider, method, params = []) {
    try {
      return await provider.request({ method, params });
    } catch (error) {
      throw new WalletConnectError(
        error?.message || error?.data?.message || `Wallet request failed: ${method}`,
        error?.code ?? null,
        error
      );
    }
  }

  function emit(name, detail = {}) {
    document.dispatchEvent(new CustomEvent(name, { detail }));
  }

  function syncFrame() {
    if (!window.OsniasFrame) return;
    window.OsniasFrame.setWallet({
      connected: state.connected,
      address: state.account || ""
    });
  }

  function publicState() {
    return Object.freeze({
      connected: state.connected,
      account: state.account,
      chainId: state.chainId,
      walletName: state.walletName,
      network: NETWORK
    });
  }

  async function getCurrentChainId(provider = state.eip1193) {
    if (!provider) throw new WalletConnectError("No wallet provider available.");
    return normalizeChainId(await request(provider, "eth_chainId"));
  }

  function isTargetChainId(chainId) {
    return Number(chainId) === Number(NETWORK.chainId);
  }

  function chainLabel(chainId) {
    const id = Number(chainId);
    if (id === 1328) return "Sei Atlantic-2 Testnet";
    if (id === 1329) return "Sei Network Mainnet";
    if (id === 11155111) return "Ethereum Sepolia";
    if (id === 1) return "Ethereum Mainnet";
    return `EVM Chain ${id}`;
  }

  async function getStableChainId(provider = state.eip1193) {
    /*
     * Some injected wallets briefly return the previous chain immediately
     * after a page reload or network switch. Read twice before declaring
     * a wrong-network state so the UI does not retain a transient false
     * "WRONG" status.
     */
    const first = await getCurrentChainId(provider);

    await new Promise((resolve) => setTimeout(resolve, 120));

    const second = await getCurrentChainId(provider);

    if (first !== second) {
      console.info(
        "[OsniasWallet] chain changed during verification:",
        first,
        "->",
        second
      );
    }

    return second;
  }

  function emitWrongNetwork(chainId) {
    emit("osnias:wallet-wrong-network", {
      account: state.account,
      chainId,
      chainName: chainLabel(chainId),
      expectedChainId: NETWORK.chainId,
      expectedNetwork: NETWORK.chainName
    });
  }

  async function switchToTargetNetwork(provider = state.eip1193) {
    if (!provider) throw new WalletConnectError("No wallet provider available.");

    const current = await getCurrentChainId(provider);
    if (isTargetChainId(current)) return true;

    try {
      await request(provider, "wallet_switchEthereumChain", [{ chainId: NETWORK.chainIdHex }]);
      return true;
    } catch (error) {
      if (error.code !== 4902) throw error;
    }

    const params = {
      chainId: NETWORK.chainIdHex,
      chainName: NETWORK.chainName,
      nativeCurrency: NETWORK.nativeCurrency
    };

    if (NETWORK.rpcUrls.length) params.rpcUrls = [...NETWORK.rpcUrls];
    if (NETWORK.blockExplorerUrls.length) params.blockExplorerUrls = [...NETWORK.blockExplorerUrls];

    await request(provider, "wallet_addEthereumChain", [params]);

    const afterAdd = await getCurrentChainId(provider);
    if (!isTargetChainId(afterAdd)) {
      await request(provider, "wallet_switchEthereumChain", [{ chainId: NETWORK.chainIdHex }]);
    }

    return true;
  }

  async function buildSigner(provider) {
    requireEthers();
    const browserProvider = new window.ethers.BrowserProvider(provider, "any");
    const signer = await browserProvider.getSigner();
    const account = normalizeAccount(await signer.getAddress());
    return { browserProvider, signer, account };
  }

  const boundProviders = new WeakSet();

  function bindProviderEvents(provider) {
    if (!provider || typeof provider.on !== "function" || boundProviders.has(provider)) return;
    provider.on("accountsChanged", handleAccountsChanged);
    provider.on("chainChanged", handleChainChanged);
    provider.on("disconnect", handleDisconnect);
    boundProviders.add(provider);
  }

  async function connect() {
    requireEthers();
    const provider = getInjectedProvider();

    if (!provider) {
      const error = new WalletConnectError(
        isMobile()
          ? "No injected wallet was detected. Open this page inside MetaMask or Trust Wallet's in-app browser."
          : "No injected EVM wallet was detected. Install MetaMask or another EIP-1193 wallet."
      );
      emit("osnias:wallet-unavailable", { message: error.message, mobile: isMobile() });
      throw error;
    }

    state.eip1193 = provider;
    state.walletName = providerName(provider);

    const accounts = await request(provider, "eth_requestAccounts");
    if (!Array.isArray(accounts) || accounts.length === 0) {
      throw new WalletConnectError("The wallet did not return an account.");
    }

    await switchToTargetNetwork(provider);

    const chainId = await getStableChainId(provider);
    if (!isTargetChainId(chainId)) {
      throw new WalletConnectError(
        `Wrong network. Expected ${NETWORK.chainName} (${NETWORK.chainId}).`
      );
    }

    const signerData = await buildSigner(provider);
    state.browserProvider = signerData.browserProvider;
    state.signer = signerData.signer;
    state.account = signerData.account;
    state.chainId = chainId;
    state.connected = true;

    bindProviderEvents(provider);
    syncFrame();

    emit("osnias:wallet-network-valid", { ...publicState() });
    emit("osnias:wallet-connected", { ...publicState() });
    return publicState();
  }

  async function restore() {
    requireEthers();
    const provider = getInjectedProvider();
    if (!provider) return publicState();

    state.eip1193 = provider;
    state.walletName = providerName(provider);

    const accounts = await request(provider, "eth_accounts");
    if (!Array.isArray(accounts) || accounts.length === 0) {
      syncFrame();
      return publicState();
    }

    const chainId = await getStableChainId(provider);
    state.chainId = chainId;
    state.account = normalizeAccount(accounts[0]);

    if (!isTargetChainId(chainId)) {
      state.connected = false;
      syncFrame();
      emitWrongNetwork(chainId);
      return publicState();
    }

    const signerData = await buildSigner(provider);
    state.browserProvider = signerData.browserProvider;
    state.signer = signerData.signer;
    state.account = signerData.account;
    state.connected = true;

    bindProviderEvents(provider);
    syncFrame();

    emit("osnias:wallet-network-valid", { ...publicState() });
    emit("osnias:wallet-restored", { ...publicState() });
    return publicState();
  }

  function resetState({ clearProvider = true } = {}) {
    state.browserProvider = null;
    state.signer = null;
    state.account = null;
    state.chainId = null;
    state.connected = false;
    state.walletName = null;
    if (clearProvider) state.eip1193 = null;
    syncFrame();
  }

  function disconnectLocal(reason = "local") {
    resetState({ clearProvider: true });
    emit("osnias:wallet-disconnected", { localOnly: true, reason });
  }

  async function disconnect() {
    const provider = state.eip1193 || getInjectedProvider();
    let permissionRevoked = false;
    let revokeUnsupported = false;
    let revokeError = null;

    if (provider && typeof provider.request === "function") {
      try {
        await provider.request({
          method: "wallet_revokePermissions",
          params: [{ eth_accounts: {} }]
        });
        permissionRevoked = true;
      } catch (error) {
        revokeError = error;
        const code = error?.code ?? error?.data?.code ?? null;
        const message = String(error?.message || error?.data?.message || "").toLowerCase();
        revokeUnsupported =
          code === -32601 ||
          code === 4200 ||
          message.includes("method not found") ||
          message.includes("unsupported") ||
          message.includes("not supported");
      }
    }

    resetState({ clearProvider: true });

    emit("osnias:wallet-disconnected", {
      localOnly: !permissionRevoked,
      permissionRevoked,
      revokeUnsupported,
      reason: "applicationDisconnect"
    });

    if (revokeError && !revokeUnsupported) {
      emit("osnias:wallet-disconnect-warning", {
        message: revokeError?.message || "Wallet permission could not be revoked.",
        code: revokeError?.code ?? null
      });
    }

    return Object.freeze({ disconnected: true, permissionRevoked, revokeUnsupported });
  }

  async function handleAccountsChanged(accounts) {
    if (!Array.isArray(accounts) || accounts.length === 0) {
      resetState();
      emit("osnias:wallet-disconnected", { reason: "accountsChanged" });
      return;
    }

    state.account = normalizeAccount(accounts[0]);
    const chainId = await getStableChainId();
    state.chainId = chainId;

    if (!isTargetChainId(chainId)) {
      state.connected = false;
      state.signer = null;
      state.browserProvider = null;
      syncFrame();
      emitWrongNetwork(chainId);
      return;
    }

    const signerData = await buildSigner(state.eip1193);
    state.browserProvider = signerData.browserProvider;
    state.signer = signerData.signer;
    state.account = signerData.account;
    state.connected = true;

    syncFrame();
    emit("osnias:wallet-network-valid", { ...publicState() });
    emit("osnias:wallet-account-changed", { ...publicState() });
  }

  async function handleChainChanged(chainIdValue) {
    let chainId;
    try {
      chainId = normalizeChainId(chainIdValue);
    } catch {
      resetState();
      return;
    }

    state.chainId = chainId;

    if (!isTargetChainId(chainId)) {
      state.connected = false;
      state.signer = null;
      state.browserProvider = null;
      syncFrame();
      emitWrongNetwork(chainId);
      return;
    }

    if (state.account && state.eip1193) {
      try {
        const signerData = await buildSigner(state.eip1193);
        state.browserProvider = signerData.browserProvider;
        state.signer = signerData.signer;
        state.account = signerData.account;
        state.connected = true;
      } catch {
        state.connected = false;
      }
    }

    syncFrame();
    emit("osnias:wallet-network-valid", { ...publicState() });
    emit("osnias:wallet-chain-changed", { ...publicState() });
  }

  function handleDisconnect(error) {
    resetState();
    emit("osnias:wallet-disconnected", {
      reason: "providerDisconnect",
      error: error?.message || null
    });
  }

  async function ensureTargetNetwork() {
    if (!state.eip1193) throw new WalletConnectError("Wallet is not connected.");

    const chainId = await getCurrentChainId();
    if (chainId !== NETWORK.chainId) await switchToTargetNetwork();

    const finalChainId = await getCurrentChainId();
    if (!isTargetChainId(finalChainId)) {
      throw new WalletConnectError(`Wallet is not connected to ${NETWORK.chainName}.`);
    }

    state.chainId = finalChainId;
    return true;
  }

  async function ensureConnected() {
    if (!state.connected) await connect();
    await ensureTargetNetwork();
    return publicState();
  }

  async function getSigner() {
    await ensureConnected();

    if (!state.signer) {
      const signerData = await buildSigner(state.eip1193);
      state.browserProvider = signerData.browserProvider;
      state.signer = signerData.signer;
      state.account = signerData.account;
    }

    await ensureTargetNetwork();
    return state.signer;
  }

  async function getProvider() {
    await ensureConnected();
    return state.browserProvider;
  }

  async function getAccount() {
    if (!state.connected || !state.account) await ensureConnected();
    return state.account;
  }

  document.addEventListener("osnias:wallet-connect-request", async () => {
    try {
      await connect();
    } catch (error) {
      emit("osnias:wallet-error", {
        message: error?.message || "Wallet connection failed.",
        code: error?.code ?? null
      });
    }
  });

  document.addEventListener("osnias:wallet-disconnect-request", async () => {
    try {
      await disconnect();
    } catch (error) {
      resetState({ clearProvider: true });
      emit("osnias:wallet-disconnected", {
        localOnly: true,
        reason: "applicationDisconnectFallback"
      });
    }
  });

  window.OsniasWallet = Object.freeze({
    version: VERSION,
    network: NETWORK,
    connect,
    restore,
    disconnect,
    disconnectLocal,
    ensureConnected,
    ensureTargetNetwork,
    getSigner,
    getProvider,
    getAccount,
    getState: publicState,
    hasInjectedWallet: () => Boolean(getInjectedProvider()),
    walletName: () => state.walletName
  });

  async function init() {
    try {
      await restore();
    } catch (error) {
      emit("osnias:wallet-error", {
        message: error?.message || "Wallet session restoration failed.",
        code: error?.code ?? null
      });
    }

    emit("osnias:wallet-connect-ready", {
      version: VERSION,
      network: NETWORK
    });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init, { once: true });
  } else {
    init();
  }
})();
