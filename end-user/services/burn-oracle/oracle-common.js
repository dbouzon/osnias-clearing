
import crypto from "node:crypto";
import fs from "node:fs";
import https from "node:https";

export function required(name) {
  const v = process.env[name];
  if (!v) throw new Error(`Missing required environment variable: ${name}`);
  return v;
}

export function canonicalize(message) {
  const keys = Object.keys(message).sort();
  const ordered = {};
  for (const k of keys) ordered[k] = message[k];
  return JSON.stringify(ordered);
}

export function makeMessageId(message) {
  return "0x" + crypto.createHash("sha256").update(canonicalize(message)).digest("hex");
}

export function signMessage(message, privateKeyPem) {
  const data = Buffer.from(canonicalize(message));
  return crypto.sign(null, data, privateKeyPem).toString("base64");
}

export function verifyMessage(message, signatureB64, publicKeyPem) {
  const data = Buffer.from(canonicalize(message));
  return crypto.verify(null, data, publicKeyPem, Buffer.from(signatureB64, "base64"));
}

export function mtlsAgent() {
  return new https.Agent({
    cert: fs.readFileSync(required("MTLS_CERT")),
    key: fs.readFileSync(required("MTLS_KEY")),
    ca: fs.readFileSync(required("MTLS_CA")),
    rejectUnauthorized: true,
    keepAlive: true,
    maxSockets: 256
  });
}

export async function postJson(url, payload, agent) {
  const body = JSON.stringify(payload);
  const u = new URL(url);

  return new Promise((resolve, reject) => {
    const req = https.request({
      protocol: u.protocol,
      hostname: u.hostname,
      port: u.port || 443,
      path: u.pathname + u.search,
      method: "POST",
      agent,
      headers: {
        "content-type": "application/json",
        "content-length": Buffer.byteLength(body)
      }
    }, (res) => {
      let raw = "";
      res.setEncoding("utf8");
      res.on("data", c => raw += c);
      res.on("end", () => {
        if (res.statusCode >= 200 && res.statusCode < 300) {
          resolve(raw ? JSON.parse(raw) : {});
        } else {
          reject(new Error(`HTTP ${res.statusCode}: ${raw}`));
        }
      });
    });

    req.on("error", reject);
    req.write(body);
    req.end();
  });
}

export class MemoryIdempotency {
  constructor() { this.seen = new Set(); }
  has(id) { return this.seen.has(id); }
  add(id) { this.seen.add(id); }
}

export class SerialTxQueue {
  constructor() {
    this.tail = Promise.resolve();
  }

  enqueue(fn) {
    const run = this.tail.then(fn, fn);
    this.tail = run.catch(() => {});
    return run;
  }
}
