/*
 * Osnias Clearing — Wallet Connect
 * Shared wallet connection layer
 *
 * File: /assets/wallet-connect.js
 * Version: 1.0.0
 *
 * Responsibilities:
 * - Connect to injected EIP-1193 wallets
 * - Support MetaMask and Trust Wallet where an injected provider is available
 * - Enforce Sei Atlantic-2 Testnet (Chain ID 1328 / 0x530)
 * - Expose an ethers v6 BrowserProvider + Signer
 * - Synchronize wallet state with end-user-frame.js
 * - React to accountsChanged / chainChanged / disconnect
 *
 * Requirements:
 * - ethers.js v6 available as window.ethers
 * - /assets/end-user-frame.js loaded
 * - /assets/sei-provider.js loaded
 *
 * Security principles:
 * - No private key handling
 * - No automatic transaction signing
 * - Every state-changing operation still requires explicit wallet approval
 * - Chain ID is checked before a signer is returned
 */

(() => {
  "use strict";

  const VERSION = "1.0.0";

  const SEI = Object.freeze({
    chainId: 1328,
    chainIdHex: "0x530",
    chainName: "Sei Atlantic-2 Testnet",
    nativeCurrency: Object.freeze({
      name: "Sei",
      symbol: "SEI",
      decimals: 18
    }),
    rpcUrls: Object.freeze([
      "https://evm-rpc-testnet.sei-apis.com"
    ]),
    blockExplorerUrls: Object.freeze([
      "https://testnet.seiscan.io"
    ])
  });

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
      throw new WalletConnectError(
        "ethers.js v6 is required by wallet-connect.js."
      );
    }
  }

  function isMobile() {
    return /Android|iPhone|iPad|iPod/i.test(
      navigator.userAgent || ""
    );
  }

  function isMetaMaskProvider(provider) {
    return Boolean(provider?.isMetaMask);
  }

  function isTrustProvider(provider) {
    return Boolean(
      provider?.isTrust ||
      provider?.isTrustWallet ||
      provider?.isTrustWalletBrowser
    );
  }

  function providerName(provider) {
    if (isTrustProvider(provider)) return "Trust Wallet";
    if (isMetaMaskProvider(provider)) return "MetaMask";
    return "Injected Wallet";
  }

  /*
   * Some browser environments expose several EIP-1193 providers.
   * Prefer a provider explicitly identified as MetaMask or Trust Wallet.
   */
  function getInjectedProvider() {
    const ethereum = window.ethereum;

    if (!ethereum) {
      return null;
    }

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
    if (typeof value === "number") {
      return value;
    }

    if (typeof value === "bigint") {
      return Number(value);
    }

    const text = String(value || "");

    if (/^0x[0-9a-fA-F]+$/.test(text)) {
      return Number(BigInt(text));
    }

    if (/^\d+$/.test(text)) {
      return Number(text);
    }

    throw new WalletConnectError(
      `Invalid chain ID returned by wallet: ${value}`
    );
  }

  function normalizeAccount(address) {
    requireEthers();

    try {
      return window.ethers.getAddress(address);
    } catch {
      throw new WalletConnectError(
        `Invalid wallet account returned: ${address}`
      );
    }
  }

  async function request(provider, method, params = []) {
    try {
      return await provider.request({
        method,
        params
      });
    } catch (error) {
      const message =
        error?.message ||
        error?.data?.message ||
        `Wallet request failed: ${method}`;

      throw new WalletConnectError(
        message,
        error?.code ?? null,
        error
      );
    }
  }

  function emit(name, detail = {}) {
    document.dispatchEvent(
      new CustomEvent(name, {
        detail
      })
    );
  }

  function syncFrame() {
    if (!window.OsniasFrame) {
      return;
    }

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
      walletName: state.walletName
    });
  }

  async function getCurrentChainId(provider = state.eip1193) {
    if (!provider) {
      throw new WalletConnectError("No wallet provider available.");
    }

    return normalizeChainId(
      await request(provider, "eth_chainId")
    );
  }

  async function switchToSei(provider = state.eip1193) {
    if (!provider) {
      throw new WalletConnectError("No wallet provider available.");
    }

    const current = await getCurrentChainId(provider);

    if (current === SEI.chainId) {
      return true;
    }

    try {
      await request(
        provider,
        "wallet_switchEthereumChain",
        [
          {
            chainId: SEI.chainIdHex
          }
        ]
      );

      return true;
    } catch (error) {
      /*
       * EIP-3326 wallets commonly return 4902 when the network is unknown.
       */
      if (error.code !== 4902) {
        throw error;
      }
    }

    await request(
      provider,
      "wallet_addEthereumChain",
      [
        {
          chainId: SEI.chainIdHex,
          chainName: SEI.chainName,
          nativeCurrency: SEI.nativeCurrency,
          rpcUrls: [...SEI.rpcUrls],
          blockExplorerUrls: [...SEI.blockExplorerUrls]
        }
      ]
    );

    const afterAdd = await getCurrentChainId(provider);

    if (afterAdd !== SEI.chainId) {
      await request(
        provider,
        "wallet_switchEthereumChain",
        [
          {
            chainId: SEI.chainIdHex
          }
        ]
      );
    }

    return true;
  }

  async function buildSigner(provider) {
    requireEthers();

    const browserProvider =
      new window.ethers.BrowserProvider(
        provider,
        "any"
      );

    const signer =
      await browserProvider.getSigner();

    const account =
      normalizeAccount(
        await signer.getAddress()
      );

    return {
      browserProvider,
      signer,
      account
    };
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

      emit("osnias:wallet-unavailable", {
        message: error.message,
        mobile: isMobile()
      });

      throw error;
    }

    state.eip1193 = provider;
    state.walletName = providerName(provider);

    const accounts = await request(
      provider,
      "eth_requestAccounts"
    );

    if (!Array.isArray(accounts) || accounts.length === 0) {
      throw new WalletConnectError(
        "The wallet did not return an account."
      );
    }

    await switchToSei(provider);

    const chainId = await getCurrentChainId(provider);

    if (chainId !== SEI.chainId) {
      throw new WalletConnectError(
        `Wrong network. Expected Sei Atlantic-2 Testnet (${SEI.chainId}).`
      );
    }

    const signerData =
      await buildSigner(provider);

    state.browserProvider =
      signerData.browserProvider;

    state.signer =
      signerData.signer;

    state.account =
      signerData.account;

    state.chainId =
      chainId;

    state.connected =
      true;

    bindProviderEvents(provider);

    syncFrame();

    emit("osnias:wallet-connected", {
      ...publicState()
    });

    return publicState();
  }

  async function restore() {
    requireEthers();

    const provider = getInjectedProvider();

    if (!provider) {
      return publicState();
    }

    state.eip1193 = provider;
    state.walletName = providerName(provider);

    const accounts = await request(
      provider,
      "eth_accounts"
    );

    if (!Array.isArray(accounts) || accounts.length === 0) {
      syncFrame();
      return publicState();
    }

    const chainId =
      await getCurrentChainId(provider);

    state.chainId = chainId;

    if (chainId !== SEI.chainId) {
      state.connected = false;
      state.account =
        normalizeAccount(accounts[0]);

      syncFrame();

      emit("osnias:wallet-wrong-network", {
        account: state.account,
        chainId
      });

      return publicState();
    }

    const signerData =
      await buildSigner(provider);

    state.browserProvider =
      signerData.browserProvider;

    state.signer =
      signerData.signer;

    state.account =
      signerData.account;

    state.connected =
      true;

    bindProviderEvents(provider);

    syncFrame();

    emit("osnias:wallet-restored", {
      ...publicState()
    });

    return publicState();
  }

  function resetState() {
    state.browserProvider = null;
    state.signer = null;
    state.account = null;
    state.chainId = null;
    state.connected = false;
    state.walletName = null;

    syncFrame();
  }

  function disconnectLocal() {
    /*
     * Most injected wallets do not expose a standards-based programmatic
     * disconnect. This only clears Osnias' local session state.
     */
    resetState();

    emit("osnias:wallet-disconnected", {
      localOnly: true
    });
  }

  async function handleAccountsChanged(accounts) {
    if (!Array.isArray(accounts) || accounts.length === 0) {
      resetState();

      emit("osnias:wallet-disconnected", {
        reason: "accountsChanged"
      });

      return;
    }

    const account =
      normalizeAccount(accounts[0]);

    state.account =
      account;

    const chainId =
      await getCurrentChainId();

    state.chainId =
      chainId;

    if (chainId !== SEI.chainId) {
      state.connected = false;
      state.signer = null;
      state.browserProvider = null;

      syncFrame();

      emit("osnias:wallet-wrong-network", {
        account,
        chainId
      });

      return;
    }

    const signerData =
      await buildSigner(state.eip1193);

    state.browserProvider =
      signerData.browserProvider;

    state.signer =
      signerData.signer;

    state.connected =
      true;

    syncFrame();

    emit("osnias:wallet-account-changed", {
      ...publicState()
    });
  }

  async function handleChainChanged(chainIdValue) {
    let chainId;

    try {
      chainId =
        normalizeChainId(chainIdValue);
    } catch {
      resetState();
      return;
    }

    state.chainId = chainId;

    if (chainId !== SEI.chainId) {
      state.connected = false;
      state.signer = null;
      state.browserProvider = null;

      syncFrame();

      emit("osnias:wallet-wrong-network", {
        account: state.account,
        chainId
      });

      return;
    }

    if (state.account && state.eip1193) {
      try {
        const signerData =
          await buildSigner(state.eip1193);

        state.browserProvider =
          signerData.browserProvider;

        state.signer =
          signerData.signer;

        state.account =
          signerData.account;

        state.connected =
          true;
      } catch {
        state.connected = false;
      }
    }

    syncFrame();

    emit("osnias:wallet-chain-changed", {
      ...publicState()
    });
  }

  function handleDisconnect(error) {
    resetState();

    emit("osnias:wallet-disconnected", {
      reason: "providerDisconnect",
      error:
        error?.message ||
        null
    });
  }

  const boundProviders =
    new WeakSet();

  function bindProviderEvents(provider) {
    if (
      !provider ||
      typeof provider.on !== "function" ||
      boundProviders.has(provider)
    ) {
      return;
    }

    provider.on(
      "accountsChanged",
      handleAccountsChanged
    );

    provider.on(
      "chainChanged",
      handleChainChanged
    );

    provider.on(
      "disconnect",
      handleDisconnect
    );

    boundProviders.add(provider);
  }

  async function ensureConnected() {
    if (!state.connected) {
      await connect();
    }

    await ensureSeiNetwork();

    return publicState();
  }

  async function ensureSeiNetwork() {
    if (!state.eip1193) {
      throw new WalletConnectError(
        "Wallet is not connected."
      );
    }

    const chainId =
      await getCurrentChainId();

    if (chainId !== SEI.chainId) {
      await switchToSei();
    }

    const finalChainId =
      await getCurrentChainId();

    if (finalChainId !== SEI.chainId) {
      throw new WalletConnectError(
        "Wallet is not connected to Sei Atlantic-2 Testnet."
      );
    }

    state.chainId =
      finalChainId;

    return true;
  }

  async function getSigner() {
    await ensureConnected();

    if (!state.signer) {
      const signerData =
        await buildSigner(state.eip1193);

      state.browserProvider =
        signerData.browserProvider;

      state.signer =
        signerData.signer;

      state.account =
        signerData.account;
    }

    /*
     * Re-check the chain immediately before returning a signer.
     * Callers should request the signer immediately before
     * eth_sendTransaction / contract writes.
     */
    await ensureSeiNetwork();

    return state.signer;
  }

  async function getAccount() {
    if (!state.connected || !state.account) {
      await ensureConnected();
    }

    return state.account;
  }

  /*
   * Frame integration.
   */
  document.addEventListener(
    "osnias:wallet-connect-request",
    async () => {
      try {
        await connect();
      } catch (error) {
        emit("osnias:wallet-error", {
          message:
            error?.message ||
            "Wallet connection failed.",
          code:
            error?.code ??
            null
        });
      }
    }
  );

  /*
   * Public API
   */
  window.OsniasWallet =
    Object.freeze({
      version: VERSION,
      sei: SEI,

      connect,
      restore,
      disconnectLocal,

      ensureConnected,
      ensureSeiNetwork,

      getSigner,
      getAccount,

      getState: publicState,

      hasInjectedWallet:
        () => Boolean(
          getInjectedProvider()
        ),

      walletName:
        () => state.walletName
    });

  /*
   * Attempt a silent restoration after the DOM is available.
   * eth_accounts does not request a new wallet permission prompt.
   */
  async function init() {
    try {
      await restore();
    } catch (error) {
      emit("osnias:wallet-error", {
        message:
          error?.message ||
          "Wallet session restoration failed.",
        code:
          error?.code ??
          null
      });
    }

    emit("osnias:wallet-connect-ready", {
      version: VERSION
    });
  }

  if (document.readyState === "loading") {
    document.addEventListener(
      "DOMContentLoaded",
      init,
      {
        once: true
      }
    );
  } else {
    init();
  }
})();
