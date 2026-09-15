OSNIAS NETWORK — NODE 1 ADMIN AUTH API
Version 0.1.0

PURPOSE
-------
Implements the server-side authentication required by:

  /node/node-admin.html

The browser:
  1. connects the administrator wallet;
  2. requests a one-time challenge;
  3. signs the message;
  4. sends the signature to the API.

The server:
  1. checks the requested wallet against ADMIN_WALLET;
  2. creates a cryptographically random one-time nonce;
  3. verifies the EIP-191 signature with ethers.verifyMessage();
  4. creates an HttpOnly / Secure / SameSite=Strict admin session;
  5. requires that session for every protected /api/node/admin/* route.

INSTALL
-------
npm install

CONFIGURATION
-------------
Copy .env.example values into the environment used by the Node process.

Required:

  ADMIN_WALLET=<admin address>

Recommended:

  PUBLIC_ORIGIN=https://www.osnias-clearing.com
  PORT=3000

START
-----
npm start

PUBLIC ROUTING
--------------
Your web server / reverse proxy must route:

  https://www.osnias-clearing.com/api/node/admin/*

to this Node.js application.

The static pages remain under:

  https://www.osnias-clearing.com/node/

IMPORTANT
---------
The current implementation keeps challenges and sessions in memory.

That is acceptable for a first single-process test, but before:
- multiple server instances,
- container restarts,
- production HA,

move challenges/sessions to Redis or SQL.

The KYC approval / SQL registry endpoints are intentionally placeholders.
They already require a valid admin session but return empty data / 501 until
the SQL layer is implemented.
