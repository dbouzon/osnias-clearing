/*
 * Osnias Clearing — Sei Provider
 * Shared read-only blockchain layer
 *
 * File: /assets/sei-provider.js
 * Version: 1.5.0
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

  const VERSION = "1.5.0";

  const CONFIG = Object.freeze({
    networkName: "Sei Atlantic-2 Testnet",
    chainId: 1328,
    chainIdHex: "0x530",
    rpcUrl: "https://evm-rpc-testnet.sei-apis.com",
    explorerBaseUrl: "https://testnet.seiscan.io",

    contracts: Object.freeze({
      orusd: "0xA4b51A41534Fd92E4C95AdEcec95B5a5E3236Aee",
      temporalOracle: "0x09ddc99ADb1dD104676dedFd16CE98BCf0399b11",
      paymentMessageRegistry: "0xe6225B4CB4a104488Df47D4496F764427dcb0c76"
    })
  });

  /*
   * OsniasTemporalOracle1W v0.4.2-testnet
   *
   * Passive reads use currentState()/currentCycleNumber() and never write.
   * Active protocol observations use syncAndGetState() with a wallet signer.
   *
   * Window enum:
   *   0 NETWORK   — Saturday
   *   1 REST      — Sunday
   *   2 BURN      — Monday
   *   3 MINT      — Tuesday through Thursday
   *   4 CLEARING  — Friday
   */
  const TEMPORAL_ORACLE_ABI = Object.freeze([
    "function initialized() view returns (bool)",
    "function cycleNumber() view returns (uint256)",
    "function currentCycleNumber() view returns (uint256)",
    "function currentWindow() view returns (string)",
    "function isBurnOpen() view returns (bool)",
    "function isMintOpen() view returns (bool)",
    "function isP2POpen() view returns (bool)",
    "function isMessageOpen() view returns (bool)",
    "function isClearingOpen() view returns (bool)",
    "function isRestDay() view returns (bool)",
    "function currentState() view returns (tuple(uint256 cycle,uint8 window,bool burnOpen,bool mintOpen,bool p2pOpen,bool messageOpen,bool clearingOpen,uint256 observedBlock,uint256 observedTimestamp,uint256 cycleStartBoundaryTimestamp,uint256 clearingBoundaryTimestamp))",
    "function currentCycleRecord() view returns (tuple(uint256 cycleNumber,uint256 calendarCycleId,tuple(uint256 boundaryTimestamp,uint256 lastObservedBlockBefore,uint256 lastObservedTimestampBefore,uint256 firstObservedBlockAfter,uint256 firstObservedTimestampAfter,bool observed) cycleStart,tuple(uint256 boundaryTimestamp,uint256 lastObservedBlockBefore,uint256 lastObservedTimestampBefore,uint256 firstObservedBlockAfter,uint256 firstObservedTimestampAfter,bool observed) clearingStart,bool exists))",
    "function syncAndGetState() returns (tuple(uint256 cycle,uint8 window,bool burnOpen,bool mintOpen,bool p2pOpen,bool messageOpen,bool clearingOpen,uint256 observedBlock,uint256 observedTimestamp,uint256 cycleStartBoundaryTimestamp,uint256 clearingBoundaryTimestamp))"
  ]);

  const MAX_LOG_BLOCK_SPAN = 1999;

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

  async function getLogsChunked({
    address,
    fromBlock,
    toBlock = "latest",
    topics = [],
    maxSpan = MAX_LOG_BLOCK_SPAN,
    onProgress = null
  }) {
    assertHexAddress(address, "log contract address");

    const start = Number(
      typeof fromBlock === "string" && fromBlock.startsWith("0x")
        ? BigInt(fromBlock)
        : fromBlock
    );

    const end = toBlock === "latest"
      ? await getBlockNumber()
      : Number(
          typeof toBlock === "string" && toBlock.startsWith("0x")
            ? BigInt(toBlock)
            : toBlock
        );

    if (!Number.isSafeInteger(start) || !Number.isSafeInteger(end)) {
      throw new RangeError("Log block range exceeds JavaScript safe integer range.");
    }

    if (start > end) return [];

    const span = Math.max(
      1,
      Math.min(Number(maxSpan) || MAX_LOG_BLOCK_SPAN, MAX_LOG_BLOCK_SPAN)
    );
    const all = [];

    for (let from = start; from <= end; from += span + 1) {
      const to = Math.min(end, from + span);

      const logs = await getLogs({
        address,
        fromBlock: from,
        toBlock: to,
        topics
      });

      all.push(...logs);

      if (typeof onProgress === "function") {
        const totalBlocks = end - start + 1;
        const scannedBlocks = to - start + 1;
        const percent = totalBlocks > 0
          ? Math.min(100, Math.round((scannedBlocks / totalBlocks) * 100))
          : 100;

        onProgress({
          fromBlock: start,
          toBlock: end,
          chunkFrom: from,
          chunkTo: to,
          scannedBlocks,
          totalBlocks,
          percent,
          logCount: all.length
        });
      }
    }

    return all.sort((a, b) => {
      const blockA = Number(BigInt(a.blockNumber));
      const blockB = Number(BigInt(b.blockNumber));

      if (blockA !== blockB) return blockA - blockB;

      const indexA = Number(a.logIndex ?? a.index ?? 0);
      const indexB = Number(b.logIndex ?? b.index ?? 0);
      return indexA - indexB;
    });
  }

  function requireEthers() {
    if (
      typeof window.ethers === "undefined" ||
      typeof window.ethers.Contract !== "function"
    ) {
      throw new OsniasRpcError(
        "ethers.js v6 is required by sei-provider.js."
      );
    }
  }

  function readTemporalOracle() {
    requireEthers();

    const provider = new window.ethers.JsonRpcProvider(
      CONFIG.rpcUrl,
      CONFIG.chainId
    );

    return new window.ethers.Contract(
      CONFIG.contracts.temporalOracle,
      TEMPORAL_ORACLE_ABI,
      provider
    );
  }

  function writeTemporalOracle(signer) {
    requireEthers();

    if (!signer) {
      throw new OsniasRpcError(
        "A wallet signer is required to synchronize the Temporal Oracle."
      );
    }

    return new window.ethers.Contract(
      CONFIG.contracts.temporalOracle,
      TEMPORAL_ORACLE_ABI,
      signer
    );
  }

  function windowNameFromEnum(value) {
    const n = Number(value);

    if (n === 0) return "NETWORK";
    if (n === 1) return "REST";
    if (n === 2) return "BURN";
    if (n === 3) return "MINT";
    if (n === 4) return "CLEARING";

    return "UNKNOWN";
  }

  /*
   * PASSIVE READ API
   * ----------------
   * No signature. No gas. No state modification.
   */
  async function getTemporalState() {
    const oracle = readTemporalOracle();
    const t = await oracle.currentState();

    return Object.freeze({
      cycleNumber: Number(t.cycle),
      window: windowNameFromEnum(t.window),
      burnOpen: Boolean(t.burnOpen),
      mintOpen: Boolean(t.mintOpen),
      p2pOpen: Boolean(t.p2pOpen),
      messagingOpen: Boolean(t.messageOpen),
      clearingOpen: Boolean(t.clearingOpen),
      observedBlock: Number(t.observedBlock),
      observedTimestamp: Number(t.observedTimestamp),
      cycleStartBoundaryTimestamp: Number(t.cycleStartBoundaryTimestamp),
      clearingBoundaryTimestamp: Number(t.clearingBoundaryTimestamp)
    });
  }

  async function getCurrentWindow() {
    return (await getTemporalState()).window;
  }

  async function getCycleNumber() {
    const oracle = readTemporalOracle();
    return Number(await oracle.currentCycleNumber());
  }

  /*
   * The v0.4.2 1W oracle does not expose a synthetic cycle-start block.
   * For RPC indexing, use the first Osnias-observed block after the current
   * Saturday boundary when one has actually been materialized.
   *
   * If the current cycle has not yet been observed by sync(), return null.
   * Passive UI reads must never fail merely because a cycle record is not
   * materialized yet.
   */
  async function getCycleStartBlock() {
    const oracle = readTemporalOracle();

    const initialized = await oracle.initialized();

    if (!initialized) {
      throw new OsniasRpcError(
        "Temporal Oracle is not initialized yet. The first active Osnias request must synchronize it."
      );
    }

    const c = await oracle.currentCycleRecord();

    if (!Boolean(c.exists)) {
      return null;
    }

    const block = BigInt(c.cycleStart.firstObservedBlockAfter);

    if (block <= 0n) {
      return null;
    }

    if (block > BigInt(Number.MAX_SAFE_INTEGER)) {
      throw new RangeError(
        "Cycle start observation block exceeds JavaScript safe integer range."
      );
    }

    return Number(block);
  }

  async function getCycleState() {
    const temporal = await getTemporalState();
    const currentBlock = await getBlockNumber();

    let cycleStartBlock = null;

    if (temporal.cycleNumber > 0) {
      cycleStartBlock = await getCycleStartBlock();
    }

    return Object.freeze({
      ...temporal,
      cycleStartBlock,
      currentBlock
    });
  }

  /*
   * ACTIVE OBSERVATION API
   * ----------------------
   * Sends syncAndGetState() to OsniasTemporalOracle1W v0.4.2.
   * This is a state-changing transaction and therefore requires a wallet
   * signature. The caller supplies no timestamp and no block number.
   */
  async function syncTemporalOracle(signer) {
    const oracle = writeTemporalOracle(signer);

    const tx = await oracle.syncAndGetState();
    const receipt = await tx.wait();

    const state = await getCycleState();

    document.dispatchEvent(
      new CustomEvent("osnias:temporal-synchronized", {
        detail: {
          transactionHash: receipt.hash || tx.hash,
          state
        }
      })
    );

    return {
      transactionHash: receipt.hash || tx.hash,
      receipt,
      state
    };
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
        burnOpen: state.burnOpen,
        mintOpen: state.mintOpen,
        p2pOpen: state.p2pOpen,
        messagingOpen: state.messagingOpen,
        clearingOpen: state.clearingOpen,
        // Temporal Oracle is the canonical source of the Friday clearing boundary.
        // clearingBoundaryTimestamp is returned in Unix seconds.
        clearingAt: state.clearingBoundaryTimestamp
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
    rpc,
    ethCall,
    getLogs,
    getLogsChunked,
    getBlockNumber,
    getChainId,
    assertCorrectNetwork,
    getCode,
    isContract,
    temporalOracleAbi: TEMPORAL_ORACLE_ABI,
    getTemporalState,
    getCurrentWindow,
    getCycleStartBlock,
    getCycleNumber,
    getCycleState,
    syncTemporalOracle,
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
