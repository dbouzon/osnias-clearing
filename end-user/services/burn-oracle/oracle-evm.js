
import crypto from "node:crypto";
import fs from "node:fs";
import https from "node:https";
import { ethers } from "ethers";
import {
  required, makeMessageId, signMessage, verifyMessage,
  mtlsAgent, postJson, MemoryIdempotency, SerialTxQueue
} from "./oracle-common.js";

const EVM_ABI = [
  "event SeiReleaseConfirmationRequested(bytes32 indexed burnRequestId,uint256 amount,bytes32 indexed releaseRef)",
  "event SeiCancellationConfirmationRequested(bytes32 indexed burnRequestId)",
  "function receiveBurnOpen(bytes32 burnRequestId,bytes32 osniasId,bytes32 escrowId,uint256 amount,uint256 seiChainId,uint64 seiDeadline,bytes32 messageId)",
  "function receiveBurnClose(bytes32 burnRequestId,bytes32 messageId)",
  "function reservations(bytes32) view returns (bytes32 osniasId,bytes32 escrowId,uint256 amount,uint256 seiChainId,uint64 seiDeadline,uint8 status)"
];

const provider = new ethers.JsonRpcProvider(required("EVM_RPC_URL"));
const signer = new ethers.Wallet(required("EVM_ORACLE_TX_PRIVATE_KEY"), provider);
const burnEvm = new ethers.Contract(required("BURN_EVM_ADDRESS"), EVM_ABI, signer);

const signingPrivateKey = fs.readFileSync(required("ORACLE_SIGNING_PRIVATE_KEY"), "utf8");
const seiSigningPublicKey = fs.readFileSync(required("PEER_SIGNING_PUBLIC_KEY"), "utf8");
const agent = mtlsAgent();
const outboundUrl = required("SEI_ORACLE_URL").replace(/\/$/, "");
const seen = new MemoryIdempotency();
const txQueue = new SerialTxQueue();

function envelope(type, body) {
  const message = {
    protocol: "OSNIAS-BURN-1",
    type,
    source: "EVM",
    destination: "SEI",
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

burnEvm.on("SeiReleaseConfirmationRequested", async (burnRequestId, amount, releaseRef) => {
  try {
    const r = await burnEvm.reservations(burnRequestId);
    await send("RELEASED", {
      burnRequestId,
      amount: amount.toString(),
      releaseRef,
      sourceChainId: (await provider.getNetwork()).chainId.toString()
    });
    console.log("sent RELEASED", burnRequestId);
  } catch (e) {
    console.error("RELEASED relay failed", burnRequestId, e);
  }
});

burnEvm.on("SeiCancellationConfirmationRequested", async (burnRequestId) => {
  try {
    await send("CANCELLED", {
      burnRequestId,
      sourceChainId: (await provider.getNetwork()).chainId.toString()
    });
    console.log("sent CANCELLED", burnRequestId);
  } catch (e) {
    console.error("CANCELLED relay failed", burnRequestId, e);
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
    if (!verifyMessage(message, signature, seiSigningPublicKey)) throw new Error("invalid peer signature");
    if (seen.has(messageId)) {
      res.writeHead(200, {"content-type":"application/json"});
      res.end(JSON.stringify({ ok: true, duplicate: true }));
      return;
    }

    if (message.protocol !== "OSNIAS-BURN-1" || message.destination !== "EVM") {
      throw new Error("invalid protocol/destination");
    }

    await txQueue.enqueue(async () => {
      if (message.type === "BURN_OPEN") {
        const tx = await burnEvm.receiveBurnOpen(
          message.burnRequestId,
          message.osniasId,
          message.escrowId,
          BigInt(message.amount),
          BigInt(message.seiChainId),
          BigInt(message.deadline),
          messageId
        );
        await tx.wait();
      } else if (message.type === "BURN_CLOSE") {
        const tx = await burnEvm.receiveBurnClose(message.burnRequestId, messageId);
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

server.listen(Number(process.env.PORT || 9444), "0.0.0.0", () => {
  console.log("osnias-burn-oracle-evm listening");
});
