/*
 * Osnias Clearing — Sei Provider
 * Shared read-only blockchain layer
 *
 * File: /assets/sei-provider.js
 * Version: 1.0.0
 *
 * Responsibilities:
 * - Centralize Sei Atlantic-2 Testnet configuration
 * - Centralize canonical Osnias contract addresses
 * - Provide JSON-RPC helpers
 * - Read OsniasTemporalOracle state
 * - Expose cycle-bounded block ranges for log indexing
 *
 * This module is read-only.
 * Wallet signing / eth_sendTransaction will be handled separately.
 */

(() => {
  "use strict";

  const VERSION = "1.0.0";

  const CONFIG = Object.freeze({
    networkName: "Sei Atlantic-2 Testnet",
    chainId: 1328,
    chainIdHex: "0x530",
    rpcUrl: "https://evm-rpc-testnet.sei-apis.com",
    explorerBaseUrl: "https://testnet.seiscan.io",

    contracts: Object.freeze({
      orusd: "0xA4b51A41534Fd92E4C95AdEcec95B5a5E3236Aee",
      temporalOracle: "0xbD1eAf727f4E98DD23455317172034b5D35c955d",
      paymentMessageRegistry: "0xe6225B4CB4a104488Df47D4496F764427dcb0c76"
    })
  });

  /*
   * Function selectors already used by the Osnias end-user interface.
   *
   * currentWindow()       -> 0xba0bafb4
   * getCycleStartBlock()  -> 0x645661d4
   *
   * cycleNumber() selector is derived from:
   * keccak256("cycleNumber()")[0:4]
   *
   * To avoid introducing a hidden dependency on a hard-coded selector that
   * has not already been part of the public front-end, cycle number is read
   * through status() when ethers is available, or through a configurable ABI
   * helper in later modules.
   */
  const SELECTORS = Object.freeze({
    currentWindow: "0xba0bafb4",
    getCycleStartBlock: "0x645661d4"
  });

  let rpcId = 1;

  class OsniasRpcError extends Error {
    constructor(message, data = null) {
      super(message);
      this.name = "OsniasRpcError";
      this.data = data;
    }
  }

  function assertHexAddress(address, label = "address") {
    const value = String(address || "");

    if (!/^0x[a-fA-F0-9]{40}$/.test(value)) {
      throw new TypeError(`Invalid ${label}: ${value}`);
    }

    return value;
  }

  function toHexBlock(value) {
    if (
      typeof value === "string" &&
      (value === "latest" ||
       value === "earliest" ||
       value === "pending" ||
       value === "safe" ||
       value === "finalized")
    ) {
      return value;
    }

    const n = typeof value === "bigint"
      ? value
      : BigInt(value);

    if (n < 0n) {
      throw new RangeError("Block number cannot be negative.");
    }

    return `0x${n.toString(16)}`;
  }

  function hexToBigInt(hex) {
    if (typeof hex !== "string" || !/^0x[0-9a-fA-F]+$/.test(hex)) {
      throw new TypeError(`Invalid hex quantity: ${hex}`);
    }

    return BigInt(hex);
  }

  function hexToNumber(hex) {
    const n = hexToBigInt(hex);

    if (n > BigInt(Number.MAX_SAFE_INTEGER)) {
      throw new RangeError("RPC quantity exceeds JavaScript safe integer range.");
    }

    return Number(n);
  }

  async function rpc(method, params = []) {
    const response = await fetch(CONFIG.rpcUrl, {
      method: "POST",
      headers: {
        "Content-Type": "application/json"
      },
      body: JSON.stringify({
        jsonrpc: "2.0",
        id: rpcId++,
        method,
        params
      })
    });

    if (!response.ok) {
      throw new OsniasRpcError(
        `RPC HTTP error ${response.status}: ${response.statusText}`
      );
    }

    const payload = await response.json();

    if (payload.error) {
      throw new OsniasRpcError(
        payload.error.message || "Unknown RPC error",
        payload.error
      );
    }

    return payload.result;
  }

  async function ethCall({
    to,
    data,
    blockTag = "latest"
  }) {
    assertHexAddress(to, "contract address");

    if (typeof data !== "string" || !/^0x[0-9a-fA-F]*$/.test(data)) {
      throw new TypeError("eth_call data must be a hex string.");
    }

    return rpc("eth_call", [
      {
        to,
        data
      },
      blockTag
    ]);
  }

  async function getBlockNumber() {
    return hexToNumber(
      await rpc("eth_blockNumber")
    );
  }

  async function getChainId() {
    return hexToNumber(
      await rpc("eth_chainId")
    );
  }

  async function assertCorrectNetwork() {
    const chainId = await getChainId();

    if (chainId !== CONFIG.chainId) {
      throw new OsniasRpcError(
        `Unexpected RPC network. Expected chain ID ${CONFIG.chainId}, received ${chainId}.`
      );
    }

    return true;
  }

  async function getCode(address, blockTag = "latest") {
    assertHexAddress(address);
    return rpc("eth_getCode", [address, blockTag]);
  }

  async function isContract(address) {
    const code = await getCode(address);
    return typeof code === "string" && code !== "0x" && code !== "0x0";
  }

  async function getLogs({
    address,
    fromBlock,
    toBlock = "latest",
    topics = []
  }) {
    assertHexAddress(address, "log contract address");

    const filter = {
      address,
      fromBlock: toHexBlock(fromBlock),
      toBlock: toHexBlock(toBlock),
      topics
    };

    return rpc("eth_getLogs", [filter]);
  }

  /*
   * ABI decoding helpers
   */

  function strip0x(value) {
    return String(value || "").replace(/^0x/, "");
  }

  function decodeUint256(result) {
    const hex = strip0x(result);

    if (!hex || hex.length < 64) {
      throw new OsniasRpcError("Invalid uint256 ABI response.");
    }

    return BigInt(`0x${hex.slice(0, 64)}`);
  }

  function decodeDynamicString(result) {
    const hex = strip0x(result);

    if (hex.length < 128) {
      throw new OsniasRpcError("Invalid dynamic string ABI response.");
    }

    const offsetBytes = Number(BigInt(`0x${hex.slice(0, 64)}`));
    const offset = offsetBytes * 2;

    const lengthHex = hex.slice(offset, offset + 64);

    if (lengthHex.length !== 64) {
      throw new OsniasRpcError("Invalid string length in ABI response.");
    }

    const lengthBytes = Number(BigInt(`0x${lengthHex}`));
    const dataStart = offset + 64;
    const dataEnd = dataStart + (lengthBytes * 2);
    const stringHex = hex.slice(dataStart, dataEnd);

    if (stringHex.length !== lengthBytes * 2) {
      throw new OsniasRpcError("Incomplete string payload in ABI response.");
    }

    const bytes = new Uint8Array(
      stringHex.match(/.{1,2}/g)?.map((byte) => parseInt(byte, 16)) || []
    );

    return new TextDecoder("utf-8", {
      fatal: false
    }).decode(bytes);
  }

  /*
   * OsniasTemporalOracle — read-only API
   */

  async function getCurrentWindow() {
    const result = await ethCall({
      to: CONFIG.contracts.temporalOracle,
      data: SELECTORS.currentWindow
    });

    return decodeDynamicString(result).toUpperCase();
  }

  async function getCycleStartBlock() {
    const result = await ethCall({
      to: CONFIG.contracts.temporalOracle,
      data: SELECTORS.getCycleStartBlock
    });

    const block = decodeUint256(result);

    if (block > BigInt(Number.MAX_SAFE_INTEGER)) {
      throw new RangeError("Cycle start block exceeds JavaScript safe integer range.");
    }

    return Number(block);
  }

  /*
   * cycleNumber() is intentionally obtained through ethers when present.
   * This keeps the low-level selector list limited to signatures already
   * confirmed in the existing Osnias front-end.
   */
  async function getCycleNumber() {
    if (
      typeof window.ethers === "undefined" ||
      typeof window.ethers.Contract !== "function"
    ) {
      throw new OsniasRpcError(
        "ethers.js is required for getCycleNumber() in sei-provider v1.0.0."
      );
    }

    const provider = new window.ethers.JsonRpcProvider(
      CONFIG.rpcUrl,
      CONFIG.chainId
    );

    const oracle = new window.ethers.Contract(
      CONFIG.contracts.temporalOracle,
      [
        "function cycleNumber() view returns (uint256)"
      ],
      provider
    );

    const value = await oracle.cycleNumber();
    return Number(value);
  }

  async function getCycleState() {
    const [
      cycleNumber,
      windowName,
      cycleStartBlock,
      currentBlock
    ] = await Promise.all([
      getCycleNumber(),
      getCurrentWindow(),
      getCycleStartBlock(),
      getBlockNumber()
    ]);

    const messagingOpen = windowName !== "CLEARING";

    return Object.freeze({
      cycleNumber,
      window: windowName,
      messagingOpen,
      cycleStartBlock,
      currentBlock
    });
  }

  async function syncFrame() {
    const state = await getCycleState();

    if (window.OsniasFrame) {
      window.OsniasFrame.setNetwork({
        name: CONFIG.networkName,
        chainId: CONFIG.chainId
      });

      window.OsniasFrame.setCycle({
        cycleNumber: state.cycleNumber,
        window: state.window,
        messagingOpen: state.messagingOpen
      });
    }

    document.dispatchEvent(
      new CustomEvent("osnias:cycle-state", {
        detail: state
      })
    );

    return state;
  }

  function explorerAddressUrl(address) {
    assertHexAddress(address);
    return `${CONFIG.explorerBaseUrl}/address/${address}`;
  }

  function explorerTxUrl(txHash) {
    const value = String(txHash || "");

    if (!/^0x[a-fA-F0-9]{64}$/.test(value)) {
      throw new TypeError(`Invalid transaction hash: ${value}`);
    }

    return `${CONFIG.explorerBaseUrl}/tx/${value}`;
  }

  window.OsniasSei = Object.freeze({
    version: VERSION,
    config: CONFIG,
    selectors: SELECTORS,
    rpc,
    ethCall,
    getLogs,
    getBlockNumber,
    getChainId,
    assertCorrectNetwork,
    getCode,
    isContract,
    getCurrentWindow,
    getCycleStartBlock,
    getCycleNumber,
    getCycleState,
    syncFrame,
    explorerAddressUrl,
    explorerTxUrl,
    toHexBlock,
    hexToBigInt,
    hexToNumber
  });

  document.dispatchEvent(
    new CustomEvent("osnias:sei-provider-ready", {
      detail: {
        version: VERSION,
        network: CONFIG.networkName,
        chainId: CONFIG.chainId
      }
    })
  );
})();
