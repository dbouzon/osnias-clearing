/*
 * Osnias Clearing — Payment Messages
 * Payment Message Registry integration
 *
 * File: /assets/payment-messages.js
 * Version: 1.0.0
 *
 * Contract:
 * OsniasPaymentMessageRegistry v2.0.2-testnet
 * 0xe6225B4CB4a104488Df47D4496F764427dcb0c76
 *
 * Responsibilities:
 * - Reconstruct current-cycle Invoices Sent / Invoices Received
 * - Start RPC log retrieval at OsniasTemporalOracle.getCycleStartBlock()
 * - Filter PaymentRequestCreated by indexed requester / payer
 * - Cache rows by wallet + cycle
 * - Refresh effective request status from the registry
 * - Expose write helpers for create / accept / reject / cancel / settle
 *
 * Requirements:
 * - /assets/sei-provider.js loaded first
 * - ethers.js v6 available as window.ethers
 *
 * This module does not choose or connect a wallet.
 * A signer must be supplied by the future wallet-connect.js module.
 */

(() => {
  "use strict";

  const VERSION = "1.0.0";
  const CACHE_VERSION = "1";
  const ORUSD_DECIMALS = 6;
  const ZERO_BYTES32 = "0x" + "00".repeat(32);

  const STATUS = Object.freeze({
    0: "NONE",
    1: "PENDING",
    2: "ACCEPTED",
    3: "REJECTED",
    4: "SETTLED",
    5: "FORCLOSED",
    6: "CANCELLED"
  });

  const ABI = Object.freeze([
    // Events
    "event PaymentRequestCreated(bytes32 indexed requestId,address indexed requester,address indexed payer,uint256 amount,string invoiceReference,string nature,uint256 cycleId,bytes32 previousRequestId,uint256 createdAt,uint256 createdBlock)",
    "event PaymentRequestAccepted(bytes32 indexed requestId,address indexed payer,uint256 timestamp)",
    "event PaymentRequestRejected(bytes32 indexed requestId,address indexed payer,uint256 timestamp)",
    "event PaymentRequestCancelled(bytes32 indexed requestId,address indexed requester,uint256 timestamp)",
    "event PaymentRequestSettled(bytes32 indexed requestId,address indexed payer,bytes32 indexed settlementTxHash,uint256 timestamp)",
    "event PaymentRequestForclosed(bytes32 indexed requestId,uint256 indexed originalCycleId,uint256 indexed currentCycleId,uint256 timestamp)",

    // Read API
    "function getPaymentRequest(bytes32 requestId) view returns ((bytes32 requestId,address requester,address payer,uint256 amount,string invoiceReference,string nature,uint256 cycleId,uint256 createdAt,uint256 createdBlock,uint256 updatedAt,bytes32 previousRequestId,bytes32 settlementTxHash,uint8 storedStatus) request)",
    "function statusOf(bytes32 requestId) view returns (uint8)",
    "function currentCycleId() view returns (uint256)",
    "function currentWindow() view returns (string)",
    "function exists(bytes32 requestId) view returns (bool)",
    "function totalRequests() view returns (uint256)",
    "function MAX_TEXT_BYTES() view returns (uint256)",

    // Write API
    "function createPaymentRequest(address payer,uint256 amount,string invoiceReference,string nature,bytes32 previousRequestId) returns (bytes32 requestId)",
    "function acceptPaymentRequest(bytes32 requestId)",
    "function rejectPaymentRequest(bytes32 requestId)",
    "function cancelPaymentRequest(bytes32 requestId)",
    "function markSettled(bytes32 requestId,bytes32 settlementTxHash)",
    "function materializeForclosure(bytes32 requestId) returns (bool changed)"
  ]);

  class PaymentMessagesError extends Error {
    constructor(message, data = null) {
      super(message);
      this.name = "PaymentMessagesError";
      this.data = data;
    }
  }

  function requireDependencies() {
    if (!window.OsniasSei) {
      throw new PaymentMessagesError(
        "OsniasSei is not available. Load sei-provider.js first."
      );
    }

    if (
      typeof window.ethers === "undefined" ||
      typeof window.ethers.Interface !== "function"
    ) {
      throw new PaymentMessagesError(
        "ethers.js v6 is required by payment-messages.js."
      );
    }
  }

  function normalizeAddress(address) {
    requireDependencies();

    try {
      return window.ethers.getAddress(String(address || ""));
    } catch {
      throw new TypeError(`Invalid EVM address: ${address}`);
    }
  }

  function normalizeBytes32(value, label = "bytes32") {
    const v = String(value || "");

    if (!/^0x[a-fA-F0-9]{64}$/.test(v)) {
      throw new TypeError(`Invalid ${label}: ${v}`);
    }

    return v.toLowerCase();
  }

  function utf8ByteLength(value) {
    return new TextEncoder().encode(String(value ?? "")).length;
  }

  function validateText(value, label) {
    const text = String(value ?? "");
    const bytes = utf8ByteLength(text);

    if (bytes === 0) {
      throw new PaymentMessagesError(`${label} is required.`);
    }

    if (bytes > 50) {
      throw new PaymentMessagesError(
        `${label} exceeds the 50 UTF-8 byte protocol limit.`
      );
    }

    return text;
  }

  function formatOrusd(baseUnits) {
    requireDependencies();
    return window.ethers.formatUnits(BigInt(baseUnits), ORUSD_DECIMALS);
  }

  function parseOrusd(displayAmount) {
    requireDependencies();

    const amount = window.ethers.parseUnits(
      String(displayAmount).trim(),
      ORUSD_DECIMALS
    );

    if (amount <= 0n) {
      throw new PaymentMessagesError("Amount must be greater than zero.");
    }

    return amount;
  }

  function statusName(value) {
    const n = Number(value);
    return STATUS[n] || `UNKNOWN_${n}`;
  }

  function cacheKey(direction, wallet, cycleId) {
    return [
      "osnias",
      "payment-messages",
      `v${CACHE_VERSION}`,
      direction,
      wallet.toLowerCase(),
      `cycle-${cycleId}`
    ].join(":");
  }

  function safeReadCache(key) {
    try {
      const raw = localStorage.getItem(key);
      if (!raw) return null;

      const parsed = JSON.parse(raw);

      if (
        !parsed ||
        parsed.cacheVersion !== CACHE_VERSION ||
        !Array.isArray(parsed.rows)
      ) {
        return null;
      }

      return parsed;
    } catch {
      return null;
    }
  }

  function safeWriteCache(key, payload) {
    try {
      localStorage.setItem(
        key,
        JSON.stringify({
          cacheVersion: CACHE_VERSION,
          savedAt: Date.now(),
          ...payload
        })
      );
    } catch {
      // Cache failure must never prevent on-chain reads.
    }
  }

  function clearWalletCache(wallet) {
    const target = String(wallet || "").toLowerCase();

    for (let i = localStorage.length - 1; i >= 0; i--) {
      const key = localStorage.key(i);

      if (
        key &&
        key.startsWith(`osnias:payment-messages:v${CACHE_VERSION}:`) &&
        key.includes(`:${target}:`)
      ) {
        localStorage.removeItem(key);
      }
    }
  }

  function getInterface() {
    requireDependencies();
    return new window.ethers.Interface(ABI);
  }

  function readContract() {
    requireDependencies();

    const provider = new window.ethers.JsonRpcProvider(
      window.OsniasSei.config.rpcUrl,
      window.OsniasSei.config.chainId
    );

    return new window.ethers.Contract(
      window.OsniasSei.config.contracts.paymentMessageRegistry,
      ABI,
      provider
    );
  }

  function writeContract(signer) {
    requireDependencies();

    if (!signer) {
      throw new PaymentMessagesError("A wallet signer is required.");
    }

    return new window.ethers.Contract(
      window.OsniasSei.config.contracts.paymentMessageRegistry,
      ABI,
      signer
    );
  }

  function topicAddress(address) {
    requireDependencies();

    return window.ethers.zeroPadValue(
      normalizeAddress(address),
      32
    );
  }

  function createdEventTopic() {
    return getInterface().getEvent("PaymentRequestCreated").topicHash;
  }

  function creationTopics(direction, wallet) {
    const addressTopic = topicAddress(wallet);
    const eventTopic = createdEventTopic();

    if (direction === "sent") {
      // topic0 event signature
      // topic1 requestId
      // topic2 requester
      // topic3 payer
      return [
        eventTopic,
        null,
        addressTopic,
        null
      ];
    }

    if (direction === "received") {
      return [
        eventTopic,
        null,
        null,
        addressTopic
      ];
    }

    throw new PaymentMessagesError(`Unknown direction: ${direction}`);
  }

  function creationLogToRow(log) {
    const parsed = getInterface().parseLog({
      topics: log.topics,
      data: log.data
    });

    if (!parsed || parsed.name !== "PaymentRequestCreated") {
      throw new PaymentMessagesError("Unexpected log type.");
    }

    const a = parsed.args;

    return {
      requestId: String(a.requestId).toLowerCase(),
      requester: normalizeAddress(a.requester),
      payer: normalizeAddress(a.payer),
      amountBaseUnits: a.amount.toString(),
      amount: formatOrusd(a.amount),
      invoiceReference: String(a.invoiceReference),
      nature: String(a.nature),
      cycleId: Number(a.cycleId),
      previousRequestId: String(a.previousRequestId).toLowerCase(),
      createdAt: Number(a.createdAt),
      createdBlock: Number(a.createdBlock),
      transactionHash: String(log.transactionHash || "").toLowerCase(),
      logIndex: Number(log.logIndex ?? log.index ?? 0),
      statusCode: 1,
      status: "PENDING",
      updatedAt: Number(a.createdAt),
      settlementTxHash: ZERO_BYTES32
    };
  }

  function mergeRows(existingRows, newRows) {
    const map = new Map();

    for (const row of [...existingRows, ...newRows]) {
      map.set(row.requestId.toLowerCase(), row);
    }

    return Array.from(map.values()).sort((a, b) => {
      if (a.createdBlock !== b.createdBlock) {
        return b.createdBlock - a.createdBlock;
      }

      return b.logIndex - a.logIndex;
    });
  }

  async function hydrateRow(row, contract = null) {
    const registry = contract || readContract();

    try {
      const r = await registry.getPaymentRequest(row.requestId);

      const statusCode = Number(r.storedStatus);

      return {
        ...row,
        requester: normalizeAddress(r.requester),
        payer: normalizeAddress(r.payer),
        amountBaseUnits: r.amount.toString(),
        amount: formatOrusd(r.amount),
        invoiceReference: String(r.invoiceReference),
        nature: String(r.nature),
        cycleId: Number(r.cycleId),
        createdAt: Number(r.createdAt),
        createdBlock: Number(r.createdBlock),
        updatedAt: Number(r.updatedAt),
        previousRequestId: String(r.previousRequestId).toLowerCase(),
        settlementTxHash: String(r.settlementTxHash).toLowerCase(),
        statusCode,
        status: statusName(statusCode)
      };
    } catch (error) {
      return {
        ...row,
        statusReadError: String(error?.shortMessage || error?.message || error)
      };
    }
  }

  async function mapWithConcurrency(items, limit, mapper) {
    const results = new Array(items.length);
    let cursor = 0;

    async function worker() {
      while (true) {
        const index = cursor++;
        if (index >= items.length) return;

        results[index] = await mapper(items[index], index);
      }
    }

    const workerCount = Math.max(
      1,
      Math.min(Number(limit) || 1, items.length || 1)
    );

    await Promise.all(
      Array.from({ length: workerCount }, () => worker())
    );

    return results;
  }

  async function hydrateRows(rows) {
    if (rows.length === 0) return [];

    const contract = readContract();

    return mapWithConcurrency(
      rows,
      6,
      (row) => hydrateRow(row, contract)
    );
  }

  async function loadDirection(
    direction,
    wallet,
    {
      forceRpc = false,
      hydrate = true
    } = {}
  ) {
    requireDependencies();

    const account = normalizeAddress(wallet);

    const cycle = await window.OsniasSei.getCycleState();

    const key = cacheKey(
      direction,
      account,
      cycle.cycleNumber
    );

    const cached = forceRpc ? null : safeReadCache(key);

    let rows = cached?.rows || [];

    let fromBlock = cycle.cycleStartBlock;

    if (
      cached &&
      Number.isInteger(cached.lastScannedBlock) &&
      cached.lastScannedBlock >= cycle.cycleStartBlock
    ) {
      fromBlock = cached.lastScannedBlock + 1;
    }

    const toBlock = cycle.currentBlock;

    if (fromBlock <= toBlock) {
      const logs = await window.OsniasSei.getLogs({
        address:
          window.OsniasSei.config.contracts.paymentMessageRegistry,
        fromBlock,
        toBlock,
        topics: creationTopics(direction, account)
      });

      const newRows = logs.map(creationLogToRow);
      rows = mergeRows(rows, newRows);
    }

    /*
     * Lifecycle actions can happen without a new PaymentRequestCreated event.
     * Therefore effective status is refreshed from the contract even when no
     * new creation logs were found.
     */
    if (hydrate) {
      rows = await hydrateRows(rows);
    }

    safeWriteCache(key, {
      direction,
      wallet: account,
      cycleId: cycle.cycleNumber,
      cycleStartBlock: cycle.cycleStartBlock,
      lastScannedBlock: toBlock,
      rows
    });

    const result = Object.freeze({
      direction,
      wallet: account,
      cycleId: cycle.cycleNumber,
      window: cycle.window,
      messagingOpen: cycle.messagingOpen,
      cycleStartBlock: cycle.cycleStartBlock,
      currentBlock: toBlock,
      rows
    });

    document.dispatchEvent(
      new CustomEvent("osnias:payment-messages-loaded", {
        detail: result
      })
    );

    return result;
  }

  async function loadSent(wallet, options = {}) {
    return loadDirection("sent", wallet, options);
  }

  async function loadReceived(wallet, options = {}) {
    return loadDirection("received", wallet, options);
  }

  async function getRequest(requestId) {
    normalizeBytes32(requestId, "requestId");

    const row = {
      requestId: requestId.toLowerCase()
    };

    return hydrateRow(row);
  }

  async function createPaymentRequest(
    signer,
    {
      payer,
      amount,
      amountBaseUnits,
      invoiceReference,
      nature,
      previousRequestId = ZERO_BYTES32
    }
  ) {
    const payee = normalizeAddress(payer);
    const ref = validateText(invoiceReference, "Invoice reference");
    const paymentNature = validateText(nature, "Nature");
    const previous = normalizeBytes32(
      previousRequestId,
      "previousRequestId"
    );

    const value =
      amountBaseUnits !== undefined
        ? BigInt(amountBaseUnits)
        : parseOrusd(amount);

    if (value <= 0n) {
      throw new PaymentMessagesError("Amount must be greater than zero.");
    }

    const state = await window.OsniasSei.getCycleState();

    if (!state.messagingOpen || state.window === "CLEARING") {
      throw new PaymentMessagesError(
        "New payment requests are closed during CLEARING."
      );
    }

    const contract = writeContract(signer);

    const tx = await contract.createPaymentRequest(
      payee,
      value,
      ref,
      paymentNature,
      previous
    );

    const receipt = await tx.wait();

    let requestId = null;

    for (const log of receipt.logs || []) {
      try {
        const parsed = getInterface().parseLog(log);

        if (parsed?.name === "PaymentRequestCreated") {
          requestId = String(parsed.args.requestId).toLowerCase();
          break;
        }
      } catch {
        // Ignore unrelated logs.
      }
    }

    document.dispatchEvent(
      new CustomEvent("osnias:payment-message-created", {
        detail: {
          requestId,
          transactionHash: receipt.hash || tx.hash
        }
      })
    );

    return {
      requestId,
      transactionHash: receipt.hash || tx.hash,
      receipt
    };
  }

  async function executeRequestAction(
    signer,
    method,
    requestId,
    extraArgs = []
  ) {
    const id = normalizeBytes32(requestId, "requestId");
    const contract = writeContract(signer);

    const tx = await contract[method](id, ...extraArgs);
    const receipt = await tx.wait();

    document.dispatchEvent(
      new CustomEvent("osnias:payment-message-action", {
        detail: {
          method,
          requestId: id,
          transactionHash: receipt.hash || tx.hash
        }
      })
    );

    return {
      transactionHash: receipt.hash || tx.hash,
      receipt
    };
  }

  async function acceptPaymentRequest(signer, requestId) {
    return executeRequestAction(
      signer,
      "acceptPaymentRequest",
      requestId
    );
  }

  async function rejectPaymentRequest(signer, requestId) {
    return executeRequestAction(
      signer,
      "rejectPaymentRequest",
      requestId
    );
  }

  async function cancelPaymentRequest(signer, requestId) {
    return executeRequestAction(
      signer,
      "cancelPaymentRequest",
      requestId
    );
  }

  async function markSettled(
    signer,
    requestId,
    settlementTxHash
  ) {
    const txHash = normalizeBytes32(
      settlementTxHash,
      "settlementTxHash"
    );

    if (txHash === ZERO_BYTES32) {
      throw new PaymentMessagesError(
        "Settlement transaction hash cannot be zero."
      );
    }

    return executeRequestAction(
      signer,
      "markSettled",
      requestId,
      [txHash]
    );
  }

  async function materializeForclosure(signer, requestId) {
    return executeRequestAction(
      signer,
      "materializeForclosure",
      requestId
    );
  }

  function explorerRequestTx(row) {
    if (!row?.transactionHash) return null;
    return window.OsniasSei.explorerTxUrl(row.transactionHash);
  }

  window.OsniasPaymentMessages = Object.freeze({
    version: VERSION,
    abi: ABI,
    status: STATUS,
    zeroBytes32: ZERO_BYTES32,
    orusdDecimals: ORUSD_DECIMALS,

    loadSent,
    loadReceived,
    getRequest,

    createPaymentRequest,
    acceptPaymentRequest,
    rejectPaymentRequest,
    cancelPaymentRequest,
    markSettled,
    materializeForclosure,

    formatOrusd,
    parseOrusd,
    statusName,
    utf8ByteLength,
    clearWalletCache,
    explorerRequestTx
  });

  document.dispatchEvent(
    new CustomEvent("osnias:payment-messages-ready", {
      detail: {
        version: VERSION,
        contract:
          window.OsniasSei?.config?.contracts?.paymentMessageRegistry || null
      }
    })
  );
})();
