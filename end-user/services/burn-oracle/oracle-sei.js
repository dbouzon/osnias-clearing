
import crypto from "node:crypto";
import fs from "node:fs";
import https from "node:https";
import { ethers } from "ethers";
import {
  required, canonicalize, makeMessageId, signMessage, verifyMessage,
  mtlsAgent, postJson, MemoryIdempotency, SerialTxQueue
} from "./oracle-common.js";

const SEI_ABI = [
  "event EvmOpenRequested(bytes32 indexed burnRequestId,bytes32 indexed osniasId,bytes32 indexed escrowId,uint256 amount,uint256 seiChainId,uint256 evmChainId,uint64 deadline)",
  "event EvmCloseRequested(bytes32 indexed burnRequestId)",
  "function confirmCollateralReleased(bytes32 burnRequestId,uint256 releasedAmount,uint256 sourceChainId,bytes32 evmReleaseRef,bytes32 messageId)",
  "function confirmEvmCancelled(bytes32 burnRequestId,uint256 sourceChainId,bytes32 messageId)"
];

const provider = new ethers.JsonRpcProvider(required("SEI_RPC_URL"));
const signer = new ethers.Wallet(required("SEI_ORACLE_TX_PRIVATE_KEY"), provider);
const burnSei = new ethers.Contract(required("BURN_SEI_ADDRESS"), SEI_ABI, signer);

const signingPrivateKey = fs.readFileSync(required("ORACLE_SIGNING_PRIVATE_KEY"), "utf8");
const evmSigningPublicKey = fs.readFileSync(required("PEER_SIGNING_PUBLIC_KEY"), "utf8");
const agent = mtlsAgent();
const outboundUrl = required("EVM_ORACLE_URL").replace(/\/$/, "");
const seen = new MemoryIdempotency();
const txQueue = new SerialTxQueue();

function envelope(type, body) {
  const message = {
    protocol: "OSNIAS-BURN-1",
    type,
    source: "SEI",
    destination: "EVM",
    nonce: crypto.randomUUID(),
    sentAt: Date.now(),
    ...body
  };
  const messageId = makeMessageId(message);
  return { message, messageId, signature: signMessage(message, signingPrivateKey) };
}

async function send(type, body) {
  const payload = envelope(type, body);
  await postJson(`${outboundUrl}/burn/message`, payload, agent);
}

burnSei.on("EvmOpenRequested", async (burnRequestId, osniasId, escrowId, amount, seiChainId, evmChainId, deadline) => {
  try {
    await send("BURN_OPEN", {
      burnRequestId,
      osniasId,
      escrowId,
      amount: amount.toString(),
      seiChainId: seiChainId.toString(),
      evmChainId: evmChainId.toString(),
      deadline: deadline.toString()
    });
    console.log("sent BURN_OPEN", burnRequestId);
  } catch (e) {
    console.error("BURN_OPEN relay failed", burnRequestId, e);
  }
});

burnSei.on("EvmCloseRequested", async (burnRequestId) => {
  try {
    await send("BURN_CLOSE", { burnRequestId });
    console.log("sent BURN_CLOSE", burnRequestId);
  } catch (e) {
    console.error("BURN_CLOSE relay failed", burnRequestId, e);
  }
});

const server = https.createServer({
  cert: fs.readFileSync(required("MTLS_CERT")),
  key: fs.readFileSync(required("MTLS_KEY")),
  ca: fs.readFileSync(required("MTLS_CA")),
  requestCert: true,
  rejectUnauthorized: true
}, async (req, res) => {
  if (req.method !== "POST" || req.url !== "/burn/message") {
    res.writeHead(404).end();
    return;
  }

  let raw = "";
  for await (const chunk of req) raw += chunk;

  try {
    const { message, messageId, signature } = JSON.parse(raw);
    if (makeMessageId(message) !== messageId) throw new Error("messageId mismatch");
    if (!verifyMessage(message, signature, evmSigningPublicKey)) throw new Error("invalid peer signature");
    if (seen.has(messageId)) {
      res.writeHead(200, {"content-type":"application/json"});
      res.end(JSON.stringify({ ok: true, duplicate: true }));
      return;
    }

    if (message.protocol !== "OSNIAS-BURN-1" || message.destination !== "SEI") {
      throw new Error("invalid protocol/destination");
    }

    await txQueue.enqueue(async () => {
      if (message.type === "RELEASED") {
        const tx = await burnSei.confirmCollateralReleased(
          message.burnRequestId,
          BigInt(message.amount),
          BigInt(message.sourceChainId),
          message.releaseRef,
          messageId
        );
        await tx.wait();
      } else if (message.type === "CANCELLED") {
        const tx = await burnSei.confirmEvmCancelled(
          message.burnRequestId,
          BigInt(message.sourceChainId),
          messageId
        );
        await tx.wait();
      } else {
        throw new Error(`unsupported message type: ${message.type}`);
      }
    });

    seen.add(messageId);
    res.writeHead(200, {"content-type":"application/json"});
    res.end(JSON.stringify({ ok: true }));
  } catch (e) {
    console.error(e);
    res.writeHead(400, {"content-type":"application/json"});
    res.end(JSON.stringify({ ok: false, error: e.message }));
  }
});

server.listen(Number(process.env.PORT || 9443), "0.0.0.0", () => {
  console.log("osnias-burn-oracle-sei listening");
});
