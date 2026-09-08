# Compatibility review — OSNIAS Burn Oracle V0.2

## Existing browser JS

The current end-user JS files are compatible because they are separate browser modules:

- `end-user-frame.js`: rendering/navigation only; it explicitly does not connect to blockchain itself.
- `wallet-connect.js`: injected EIP-1193 wallet and Sei Atlantic-2 signer.
- `sei-provider.js`: browser-side Sei provider helpers.
- `payment-messages.js`: payment-message registry only.

The burn oracle daemons in this folder are server-only and MUST NOT be placed under `/assets/`.

## One interface issue found in burn-sei.sol V0.1

`EvmOpenRequested` currently emits:

```solidity
event EvmOpenRequested(
    bytes32 indexed burnRequestId,
    bytes32 indexed escrowId,
    uint256 amount,
    uint256 evmChainId,
    uint64 deadline
);
```

But `burn-evm.sol::receiveBurnOpen()` also requires:

- `osniasId`
- `seiChainId`

Therefore V0.1 cannot relay a complete BURN_OPEN message from the event alone.

Recommended V0.2 event:

```solidity
event EvmOpenRequested(
    bytes32 indexed burnRequestId,
    bytes32 indexed osniasId,
    bytes32 indexed escrowId,
    uint256 amount,
    uint256 seiChainId,
    uint256 evmChainId,
    uint64 deadline
);
```

Then emit `block.chainid` as `seiChainId`.

This is the only material contract/JS compatibility gap found in the first burn draft.

## Concurrency

The services accept concurrent HTTPS requests, while writes from a single oracle wallet are serialized through `SerialTxQueue`. This avoids nonce collisions for bursts such as 100 simultaneous burns.

For production, replace the in-memory idempotency set with PostgreSQL/Redis and persist the event cursor so restarts cannot lose or replay work.
