/**
 * Osnias Network — Node 1 Admin Authentication API
 * Version 0.1.0
 *
 * Endpoints:
 *   POST /api/node/admin/auth/challenge
 *   POST /api/node/admin/auth/verify
 *   POST /api/node/admin/auth/logout
 *
 * Security model:
 *   - exact allow-listed admin wallet (server-side ENV)
 *   - one-time challenge / nonce
 *   - 5 minute challenge expiry
 *   - EIP-191 personal message signature verification
 *   - 30 minute server-side session
 *   - HttpOnly + Secure + SameSite=Strict cookie
 *   - same-origin protection
 *
 * IMPORTANT:
 *   - For a multi-instance / production deployment, replace the in-memory
 *     challenge/session stores with Redis or SQL.
 *   - Never expose ADMIN_WALLET in a public API response.
 */

"use strict";

const express = require("express");
const crypto = require("crypto");
const { getAddress, verifyMessage } = require("ethers");

const app = express();

const PORT = Number(process.env.PORT || 3000);
const PUBLIC_ORIGIN =
  process.env.PUBLIC_ORIGIN || "https://www.osnias-clearing.com";

if (!process.env.ADMIN_WALLET) {
  throw new Error("Missing required environment variable ADMIN_WALLET.");
}

const ADMIN_WALLET = getAddress(process.env.ADMIN_WALLET);

const CHALLENGE_TTL_MS = 5 * 60 * 1000;
const SESSION_TTL_MS = 30 * 60 * 1000;
const COOKIE_NAME = "osnias_node_admin";

app.disable("x-powered-by");
app.use(express.json({ limit: "32kb" }));

/**
 * In-memory stores for the first implementation.
 * Replace with Redis or SQL before horizontal scaling.
 */
const challenges = new Map();
const sessions = new Map();

function nowIso() {
  return new Date().toISOString();
}

function randomToken(bytes = 32) {
  return crypto.randomBytes(bytes).toString("hex");
}

function sha256(value) {
  return crypto.createHash("sha256").update(value).digest("hex");
}

function parseCookies(req) {
  const raw = req.headers.cookie || "";
  const result = {};

  raw.split(";").forEach((part) => {
    const idx = part.indexOf("=");
    if (idx === -1) return;

    const key = part.slice(0, idx).trim();
    const value = part.slice(idx + 1).trim();

    if (key) result[key] = decodeURIComponent(value);
  });

  return result;
}

function clearAdminCookie(res) {
  res.setHeader(
    "Set-Cookie",
    `${COOKIE_NAME}=; Path=/api/node/admin; HttpOnly; Secure; SameSite=Strict; Max-Age=0`
  );
}

function setAdminCookie(res, token) {
  const maxAgeSeconds = Math.floor(SESSION_TTL_MS / 1000);

  res.setHeader(
    "Set-Cookie",
    `${COOKIE_NAME}=${encodeURIComponent(token)}; Path=/api/node/admin; HttpOnly; Secure; SameSite=Strict; Max-Age=${maxAgeSeconds}`
  );
}

function safeAddress(value) {
  try {
    return getAddress(String(value || ""));
  } catch {
    return null;
  }
}

/**
 * Require requests to originate from the Osnias site.
 * This is an additional protection; SameSite cookies remain enabled.
 */
function requireSameOrigin(req, res, next) {
  const origin = req.get("origin");

  if (origin && origin !== PUBLIC_ORIGIN) {
    return res.status(403).json({
      error: "ORIGIN_NOT_ALLOWED"
    });
  }

  next();
}

/**
 * Session middleware for every protected admin route.
 */
function requireAdminSession(req, res, next) {
  const cookies = parseCookies(req);
  const rawToken = cookies[COOKIE_NAME];

  if (!rawToken) {
    return res.status(401).json({
      error: "ADMIN_AUTH_REQUIRED"
    });
  }

  const tokenHash = sha256(rawToken);
  const session = sessions.get(tokenHash);

  if (!session) {
    clearAdminCookie(res);
    return res.status(401).json({
      error: "ADMIN_SESSION_INVALID"
    });
  }

  if (Date.now() > session.expiresAt) {
    sessions.delete(tokenHash);
    clearAdminCookie(res);

    return res.status(401).json({
      error: "ADMIN_SESSION_EXPIRED"
    });
  }

  if (session.address !== ADMIN_WALLET) {
    sessions.delete(tokenHash);
    clearAdminCookie(res);

    return res.status(403).json({
      error: "ADMIN_SESSION_FORBIDDEN"
    });
  }

  req.admin = {
    address: session.address,
    authenticatedAt: session.authenticatedAt,
    sessionId: session.sessionId
  };

  next();
}

/**
 * POST /api/node/admin/auth/challenge
 *
 * Body:
 *   { "address": "0x..." }
 *
 * Returns:
 *   {
 *     "nonce": "...",
 *     "message": "...",
 *     "expiresAt": "..."
 *   }
 */
app.post(
  "/api/node/admin/auth/challenge",
  requireSameOrigin,
  (req, res) => {
    const address = safeAddress(req.body?.address);

    // Intentionally generic response for unauthorized wallets.
    if (!address || address !== ADMIN_WALLET) {
      return res.status(403).json({
        error: "ADMIN_ACCESS_DENIED"
      });
    }

    const nonce = randomToken(24);
    const issuedAt = new Date();
    const expiresAt = new Date(issuedAt.getTime() + CHALLENGE_TTL_MS);

    const message = [
      "Osnias Network — Node 1",
      "Administrator authentication",
      "",
      `Wallet: ${address}`,
      `Nonce: ${nonce}`,
      `Issued at: ${issuedAt.toISOString()}`,
      `Expires at: ${expiresAt.toISOString()}`,
      "Network: Ethereum Sepolia",
      "",
      "This signature does not authorize any blockchain transaction."
    ].join("\n");

    challenges.set(nonce, {
      address,
      message,
      createdAt: issuedAt.getTime(),
      expiresAt: expiresAt.getTime(),
      used: false
    });

    return res.json({
      nonce,
      message,
      expiresAt: expiresAt.toISOString()
    });
  }
);

/**
 * POST /api/node/admin/auth/verify
 *
 * Body:
 *   {
 *     "address": "0x...",
 *     "nonce": "...",
 *     "signature": "0x..."
 *   }
 *
 * On success:
 *   - challenge becomes unusable
 *   - secure server session is created
 *   - HttpOnly cookie is sent
 */
app.post(
  "/api/node/admin/auth/verify",
  requireSameOrigin,
  (req, res) => {
    const address = safeAddress(req.body?.address);
    const nonce = String(req.body?.nonce || "");
    const signature = String(req.body?.signature || "");

    if (!address || address !== ADMIN_WALLET) {
      return res.status(403).json({
        error: "ADMIN_ACCESS_DENIED"
      });
    }

    if (!nonce || !signature) {
      return res.status(400).json({
        error: "MISSING_SIGNATURE_DATA"
      });
    }

    const challenge = challenges.get(nonce);

    if (!challenge) {
      return res.status(401).json({
        error: "CHALLENGE_INVALID"
      });
    }

    if (challenge.used) {
      challenges.delete(nonce);

      return res.status(401).json({
        error: "CHALLENGE_ALREADY_USED"
      });
    }

    if (Date.now() > challenge.expiresAt) {
      challenges.delete(nonce);

      return res.status(401).json({
        error: "CHALLENGE_EXPIRED"
      });
    }

    if (challenge.address !== address) {
      challenges.delete(nonce);

      return res.status(401).json({
        error: "CHALLENGE_ADDRESS_MISMATCH"
      });
    }

    let recoveredAddress;

    try {
      recoveredAddress = getAddress(
        verifyMessage(challenge.message, signature)
      );
    } catch {
      challenges.delete(nonce);

      return res.status(401).json({
        error: "SIGNATURE_INVALID"
      });
    }

    // Challenge is strictly one-time, even if the signature is wrong.
    challenge.used = true;
    challenges.delete(nonce);

    if (
      recoveredAddress !== address ||
      recoveredAddress !== ADMIN_WALLET
    ) {
      return res.status(403).json({
        error: "SIGNER_NOT_AUTHORIZED"
      });
    }

    const sessionToken = randomToken(32);
    const tokenHash = sha256(sessionToken);
    const authenticatedAt = Date.now();
    const expiresAt = authenticatedAt + SESSION_TTL_MS;

    sessions.set(tokenHash, {
      sessionId: randomToken(16),
      address,
      authenticatedAt,
      expiresAt
    });

    setAdminCookie(res, sessionToken);

    return res.json({
      ok: true,
      authenticated: true,
      expiresAt: new Date(expiresAt).toISOString()
    });
  }
);

/**
 * POST /api/node/admin/auth/logout
 */
app.post(
  "/api/node/admin/auth/logout",
  requireSameOrigin,
  (req, res) => {
    const cookies = parseCookies(req);
    const rawToken = cookies[COOKIE_NAME];

    if (rawToken) {
      sessions.delete(sha256(rawToken));
    }

    clearAdminCookie(res);

    return res.json({
      ok: true,
      authenticated: false
    });
  }
);

/**
 * Protected admin session test.
 * Useful before implementing the KYC / Registry routes.
 */
app.get(
  "/api/node/admin/auth/session",
  requireSameOrigin,
  requireAdminSession,
  (req, res) => {
    return res.json({
      authenticated: true,
      authenticatedAt: new Date(
        req.admin.authenticatedAt
      ).toISOString()
    });
  }
);

/**
 * Placeholders for the admin API already expected by node-admin.js.
 * These are protected by requireAdminSession.
 *
 * Replace their empty responses when the SQL layer is implemented.
 */
app.get(
  "/api/node/admin/kyc/requests",
  requireSameOrigin,
  requireAdminSession,
  (req, res) => {
    res.json({ requests: [] });
  }
);

app.get(
  "/api/node/admin/registry",
  requireSameOrigin,
  requireAdminSession,
  (req, res) => {
    res.json({ registry: [] });
  }
);

app.get(
  "/api/node/admin/audit",
  requireSameOrigin,
  requireAdminSession,
  (req, res) => {
    res.json({ events: [] });
  }
);

app.post(
  "/api/node/admin/kyc/review",
  requireSameOrigin,
  requireAdminSession,
  (req, res) => {
    res.status(501).json({
      error: "NOT_IMPLEMENTED"
    });
  }
);

app.post(
  "/api/node/admin/kyc/approve",
  requireSameOrigin,
  requireAdminSession,
  (req, res) => {
    res.status(501).json({
      error: "NOT_IMPLEMENTED"
    });
  }
);

app.post(
  "/api/node/admin/kyc/reject",
  requireSameOrigin,
  requireAdminSession,
  (req, res) => {
    res.status(501).json({
      error: "NOT_IMPLEMENTED"
    });
  }
);

/**
 * Generic error handling.
 */
app.use((err, req, res, next) => {
  console.error(err);

  res.status(500).json({
    error: "INTERNAL_SERVER_ERROR"
  });
});

/**
 * Periodic cleanup of expired challenges and sessions.
 */
setInterval(() => {
  const now = Date.now();

  for (const [nonce, challenge] of challenges.entries()) {
    if (now > challenge.expiresAt) {
      challenges.delete(nonce);
    }
  }

  for (const [tokenHash, session] of sessions.entries()) {
    if (now > session.expiresAt) {
      sessions.delete(tokenHash);
    }
  }
}, 60_000).unref();

app.listen(PORT, () => {
  console.log(
    `Osnias Node 1 admin API listening on port ${PORT} — ${nowIso()}`
  );
});

module.exports = {
  app,
  requireAdminSession
};
