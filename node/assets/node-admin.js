/*
 * Osnias Network — Node Administration
 * Version 0.2.2
 *
 * Admin access:
 *  - exact wallet allowlist check
 *  - Sepolia network check
 *  - server-issued nonce/challenge
 *  - personal_sign
 *  - backend verification + secure session cookie
 *
 * IMPORTANT: front-end checks alone are not security.
 * Every /api/node/admin/* endpoint must verify the authenticated server session.
 */

(function () {
  "use strict";

  const ADMIN_WALLET = "0x9FcC10582df1dC2E7Da79653535Fd3e5b8De2570".toLowerCase();
  const cfg = window.OSNIAS_NODE_ADMIN_CONFIG || {};
  const $ = (id) => document.getElementById(id);

  let connectedAdminWallet = null;
  let adminAuthenticated = false;

  function authMessage(message, type) {
    const el = $("adminAuthMessage");
    if (!el) return;
    el.className = "osnias-alert";
    if (type === "success") el.classList.add("osnias-alert--success");
    if (type === "danger") el.classList.add("osnias-alert--danger");
    el.textContent = message;
  }

  function setAuthenticated(authenticated) {
    adminAuthenticated = Boolean(authenticated);
    $("adminProtectedContent").hidden = !adminAuthenticated;
    $("adminLogout").hidden = !adminAuthenticated;

    const badge = $("adminAuthStatus");
    if (adminAuthenticated) {
      badge.textContent = "AUTHENTIFIÉ";
      badge.className = "osnias-badge osnias-badge--success";
      $("adminAuthGate").classList.add("osnias-card--success");
    } else {
      badge.textContent = "NON AUTHENTIFIÉ";
      badge.className = "osnias-badge osnias-badge--pending";
      $("adminAuthGate").classList.remove("osnias-card--success");
    }
  }

  async function connectAdminWallet() {
    if (!window.OsniasWallet || typeof window.OsniasWallet.connect !== "function") {
      throw new Error(
        "wallet-connect.js n’est pas chargé. Vérifiez /node/assets/wallet-connect.js."
      );
    }

    const walletState = await window.OsniasWallet.connect();
    const address = walletState?.account;

    if (!address) {
      throw new Error("Le wallet n’a retourné aucune adresse.");
    }

    connectedAdminWallet = address;
    $("adminConnectedWallet").textContent = address;

    const authField = $("adminWalletAuthorization");

    if (address.toLowerCase() !== ADMIN_WALLET) {
      if (authField) authField.textContent = "NON AUTORISÉ";
      $("adminSignChallenge").disabled = true;
      setAuthenticated(false);
      throw new Error("Accès refusé : ce wallet n'est pas autorisé comme Node Admin.");
    }

    if (authField) authField.textContent = "AUTORISÉ";
    $("adminSignChallenge").disabled = false;
    authMessage(
      "Wallet administrateur reconnu sur Ethereum Sepolia. Signez le challenge pour ouvrir la session.",
      "success"
    );
  }

  async function authenticateAdmin() {
    if (!connectedAdminWallet || connectedAdminWallet.toLowerCase() !== ADMIN_WALLET) {
      throw new Error("Wallet administrateur non connecté.");
    }

    const challengeRes = await fetch(cfg.challengeEndpoint, {
      method: "POST",
      headers: {
        "Accept": "application/json",
        "Content-Type": "application/json"
      },
      credentials: "same-origin",
      body: JSON.stringify({ address: connectedAdminWallet })
    });

    if (!challengeRes.ok) {
      throw new Error("Impossible d'obtenir le challenge d'authentification.");
    }

    const challenge = await challengeRes.json();
    const message = challenge.message;
    if (!message || !challenge.nonce) {
      throw new Error("Challenge serveur invalide.");
    }

    if (!window.OsniasWallet || typeof window.OsniasWallet.getSigner !== "function") {
      throw new Error("Signer wallet indisponible.");
    }

    const signer = await window.OsniasWallet.getSigner();
    const signature = await signer.signMessage(message);

    const verifyRes = await fetch(cfg.verifyEndpoint, {
      method: "POST",
      headers: {
        "Accept": "application/json",
        "Content-Type": "application/json"
      },
      credentials: "same-origin",
      body: JSON.stringify({
        address: connectedAdminWallet,
        nonce: challenge.nonce,
        signature
      })
    });

    if (!verifyRes.ok) {
      throw new Error("Signature administrateur refusée par le Node.");
    }

    setAuthenticated(true);
    authMessage("Session administrateur ouverte.", "success");

    // Load protected data only after server authentication.
    if (typeof window.OSNIAS_ADMIN_LOAD_ALL === "function") {
      await window.OSNIAS_ADMIN_LOAD_ALL();
    }
  }

  async function logoutAdmin() {
    try {
      await fetch(cfg.logoutEndpoint, {
        method: "POST",
        credentials: "same-origin"
      });
    } catch (_) {}

    setAuthenticated(false);

    if (window.OsniasWallet && typeof window.OsniasWallet.disconnectLocal === "function") {
      window.OsniasWallet.disconnectLocal("adminLogout");
    }

    connectedAdminWallet = null;
    $("adminConnectedWallet").textContent = "—";
    const authField = $("adminWalletAuthorization");
    if (authField) authField.textContent = "En attente";

    authMessage("Session administrateur fermée.");
  }

  $("adminConnectWallet")?.addEventListener("click", async () => {
    try {
      await connectAdminWallet();
    } catch (e) {
      authMessage(e?.message || "Connexion refusée.", "danger");
    }
  });

  $("adminSignChallenge")?.addEventListener("click", async () => {
    try {
      $("adminSignChallenge").disabled = true;
      await authenticateAdmin();
    } catch (e) {
      setAuthenticated(false);
      authMessage(e?.message || "Authentification refusée.", "danger");
    } finally {
      if (!adminAuthenticated && connectedAdminWallet?.toLowerCase() === ADMIN_WALLET) {
        $("adminSignChallenge").disabled = false;
      }
    }
  });

  $("adminLogout")?.addEventListener("click", logoutAdmin);

  document.addEventListener("osnias:wallet-account-changed", (event) => {
    connectedAdminWallet = event.detail?.account || null;
    $("adminConnectedWallet").textContent = connectedAdminWallet || "—";

    const authField = $("adminWalletAuthorization");
    if (authField) {
      authField.textContent =
        connectedAdminWallet && connectedAdminWallet.toLowerCase() === ADMIN_WALLET
          ? "AUTORISÉ"
          : connectedAdminWallet
            ? "NON AUTORISÉ"
            : "En attente";
    }

    $("adminSignChallenge").disabled =
      !connectedAdminWallet || connectedAdminWallet.toLowerCase() !== ADMIN_WALLET;

    setAuthenticated(false);

    if (connectedAdminWallet && connectedAdminWallet.toLowerCase() !== ADMIN_WALLET) {
      authMessage("Accès refusé : wallet administrateur incorrect.", "danger");
    } else {
      authMessage("Le wallet a changé. Une nouvelle signature est requise.");
    }
  });

  document.addEventListener("osnias:wallet-disconnected", () => {
    connectedAdminWallet = null;
    $("adminConnectedWallet").textContent = "—";
    const authField = $("adminWalletAuthorization");
    if (authField) authField.textContent = "En attente";
    $("adminSignChallenge").disabled = true;
    setAuthenticated(false);
    authMessage("Wallet déconnecté.");
  });

  document.addEventListener("osnias:wallet-wrong-network", () => {
    setAuthenticated(false);
    authMessage("Ethereum Sepolia est requis pour l’administration du Node.", "danger");
  });

  document.addEventListener("osnias:wallet-error", (event) => {
    setAuthenticated(false);
    authMessage(event.detail?.message || "Erreur wallet.", "danger");
  });

  document.addEventListener("osnias:wallet-restored", (event) => {
    const address = event.detail?.account || null;
    connectedAdminWallet = address;
    $("adminConnectedWallet").textContent = address || "—";

    const authField = $("adminWalletAuthorization");
    const allowed = address && address.toLowerCase() === ADMIN_WALLET;

    if (authField) authField.textContent = allowed ? "AUTORISÉ" : address ? "NON AUTORISÉ" : "En attente";
    $("adminSignChallenge").disabled = !allowed;
  });

  setAuthenticated(false);
})();


/*
 * Osnias Network — Node Administration
 * Version 0.2.2
 *
 * UI scaffold.
 * All sensitive operations must be authorized server-side.
 */

(function () {
  "use strict";

  const $ = (id) => document.getElementById(id);
  const cfg = window.OSNIAS_NODE_ADMIN_CONFIG || {};

  const state = {
    requests: [],
    registry: [],
    audit: [],
    selectedRequest: null
  };

  function setMessage(message, type) {
    const el = $("pageMessage");
    el.className = "osnias-alert";
    if (type === "success") el.classList.add("osnias-alert--success");
    if (type === "danger") el.classList.add("osnias-alert--danger");
    el.textContent = message;
  }

  function badge(status) {
    const s = String(status || "").toUpperCase();
    const cls =
      s === "ACTIVE" || s === "APPROVED"
        ? "osnias-badge--success"
        : s === "REQUESTED" || s === "UNDER_REVIEW"
          ? "osnias-badge--pending"
          : "osnias-badge--danger";
    return `<span class="osnias-badge ${cls}">${s || "—"}</span>`;
  }

  function fmtDate(value) {
    if (!value) return "—";
    const d = new Date(value);
    return Number.isNaN(d.getTime()) ? value : d.toLocaleString("fr-CH");
  }

  function renderCounters() {
    $("pendingCount").textContent =
      state.requests.filter((r) => r.status === "REQUESTED").length;
    $("reviewCount").textContent =
      state.requests.filter((r) => r.status === "UNDER_REVIEW").length;
    $("activeUidCount").textContent =
      state.registry.filter((r) => r.status === "ACTIVE").length;
    $("activeOidCount").textContent =
      state.registry.reduce((n, r) => n + (Array.isArray(r.oids) ? r.oids.length : 0), 0);
  }

  function renderRequests() {
    const filter = $("requestFilter").value;
    const rows = state.requests.filter((r) => !filter || r.status === filter);

    const body = $("requestsTableBody");
    body.innerHTML = "";

    if (!rows.length) {
      $("requestsEmpty").hidden = false;
      $("requestsTableWrap").hidden = true;
      return;
    }

    $("requestsEmpty").hidden = true;
    $("requestsTableWrap").hidden = false;

    rows.forEach((r) => {
      const tr = document.createElement("tr");
      tr.innerHTML = `
        <td class="osnias-table__mono">${r.requestId || "—"}</td>
        <td>${r.legalName || "—"}</td>
        <td class="osnias-table__mono">${r.swissUid || "—"}</td>
        <td class="osnias-table__mono">${r.wallet || "—"}</td>
        <td>${badge(r.status)}</td>
        <td>${fmtDate(r.submittedAt)}</td>
        <td>
          <button
            class="osnias-btn osnias-btn--small"
            type="button"
            data-open-request="${r.requestId || ""}"
          >Ouvrir</button>
        </td>
      `;
      body.appendChild(tr);
    });
  }

  function renderRequestDetail(r) {
    state.selectedRequest = r || null;
    $("requestDetailCard").hidden = !r;
    if (!r) return;

    $("detailRequestId").textContent = r.requestId || "—";
    $("detailLegalName").textContent = r.legalName || "—";
    $("detailSwissUid").textContent = r.swissUid || "—";
    $("detailWallet").textContent = r.wallet || "—";
    $("detailProvisionalUid").textContent = r.provisionalUid || "—";
    $("detailEmail").textContent = r.businessEmail || "—";
    $("detailLandline").textContent = r.landline || "—";
    $("reviewNote").value = r.reviewNote || "";

    const status = $("requestDetailStatus");
    status.outerHTML = badge(r.status).replace(
      "<span ",
      '<span id="requestDetailStatus" '
    );
  }

  function renderRegistry() {
    const q = $("registrySearch").value.trim().toLowerCase();

    const rows = state.registry.filter((r) => {
      if (!q) return true;
      return JSON.stringify(r).toLowerCase().includes(q);
    });

    const body = $("registryTableBody");
    body.innerHTML = "";

    if (!rows.length) {
      $("registryEmpty").hidden = false;
      $("registryTableWrap").hidden = true;
      return;
    }

    $("registryEmpty").hidden = true;
    $("registryTableWrap").hidden = false;

    rows.forEach((r) => {
      const wallets = Array.isArray(r.wallets) ? r.wallets.join("<br>") : (r.wallet || "—");
      const oids = Array.isArray(r.oids) ? r.oids.join("<br>") : "—";

      const tr = document.createElement("tr");
      tr.innerHTML = `
        <td class="osnias-table__mono">${r.uid || "—"}</td>
        <td>${r.legalName || "—"}</td>
        <td class="osnias-table__mono">${r.swissUid || "—"}</td>
        <td class="osnias-table__mono">${wallets}</td>
        <td class="osnias-table__mono">${oids}</td>
        <td>${badge(r.status)}</td>
        <td>${fmtDate(r.activatedAt)}</td>
      `;
      body.appendChild(tr);
    });
  }

  function renderAudit() {
    const body = $("auditTableBody");
    body.innerHTML = "";

    if (!state.audit.length) {
      $("auditEmpty").hidden = false;
      $("auditTableWrap").hidden = true;
      return;
    }

    $("auditEmpty").hidden = true;
    $("auditTableWrap").hidden = false;

    state.audit.forEach((a) => {
      const tr = document.createElement("tr");
      tr.innerHTML = `
        <td>${fmtDate(a.createdAt)}</td>
        <td>${a.action || "—"}</td>
        <td class="osnias-table__mono">${a.reference || "—"}</td>
        <td>${a.operator || "—"}</td>
        <td>${a.detail || "—"}</td>
      `;
      body.appendChild(tr);
    });
  }

  async function apiGet(url) {
    const res = await fetch(url, {
      headers: { "Accept": "application/json" },
      credentials: "same-origin"
    });

    if (!res.ok) {
      throw new Error("API indisponible ou accès administrateur refusé.");
    }

    return res.json();
  }

  async function apiPost(url, payload) {
    const res = await fetch(url, {
      method: "POST",
      headers: {
        "Accept": "application/json",
        "Content-Type": "application/json"
      },
      credentials: "same-origin",
      body: JSON.stringify(payload)
    });

    if (!res.ok) {
      throw new Error("Action refusée par l’API du Node.");
    }

    return res.json();
  }

  async function loadRequests() {
    try {
      const data = await apiGet(cfg.requestsEndpoint);
      state.requests = Array.isArray(data) ? data : (data.requests || []);
      renderRequests();
      renderCounters();
      $("adminStatus").textContent = "API connectée";
      $("adminStatus").className = "osnias-badge osnias-badge--success";
      setMessage("Demandes KYC chargées.", "success");
    } catch (e) {
      $("adminStatus").textContent = "API non connectée";
      $("adminStatus").className = "osnias-badge osnias-badge--danger";
      setMessage(e.message, "danger");
    }
  }

  async function loadRegistry() {
    try {
      const data = await apiGet(cfg.registryEndpoint);
      state.registry = Array.isArray(data) ? data : (data.registry || []);
      renderRegistry();
      renderCounters();
    } catch (e) {
      setMessage(e.message, "danger");
    }
  }

  async function loadAudit() {
    try {
      const data = await apiGet(cfg.auditEndpoint);
      state.audit = Array.isArray(data) ? data : (data.events || []);
      renderAudit();
    } catch (e) {
      setMessage(e.message, "danger");
    }
  }

  async function performAction(action) {
    const r = state.selectedRequest;
    if (!r) return;

    const note = $("reviewNote").value.trim();

    const endpoint =
      action === "approve"
        ? cfg.approveEndpoint
        : action === "reject"
          ? cfg.rejectEndpoint
          : cfg.reviewEndpoint;

    try {
      const data = await apiPost(endpoint, {
        requestId: r.requestId,
        note
      });

      setMessage(data.message || "Action enregistrée.", "success");
      await loadRequests();
      await loadRegistry();
      await loadAudit();

      const refreshed = state.requests.find((x) => x.requestId === r.requestId);
      if (refreshed) renderRequestDetail(refreshed);
      else renderRequestDetail(null);
    } catch (e) {
      setMessage(e.message, "danger");
    }
  }

  $("refreshRequests").addEventListener("click", loadRequests);
  $("refreshRegistry").addEventListener("click", loadRegistry);
  $("refreshAudit").addEventListener("click", loadAudit);

  $("requestFilter").addEventListener("change", renderRequests);
  $("registrySearch").addEventListener("input", renderRegistry);

  $("requestsTableBody").addEventListener("click", (event) => {
    const btn = event.target.closest("[data-open-request]");
    if (!btn) return;

    const r = state.requests.find((x) => x.requestId === btn.dataset.openRequest);
    renderRequestDetail(r || null);
  });

  $("markReview").addEventListener("click", () => performAction("review"));
  $("approveRequest").addEventListener("click", () => performAction("approve"));
  $("rejectRequest").addEventListener("click", () => performAction("reject"));

  window.OSNIAS_ADMIN_LOAD_ALL = async function () {
    await Promise.allSettled([
      loadRequests(),
      loadRegistry(),
      loadAudit()
    ]);
  };

  // Protected content remains hidden until the wallet challenge is verified server-side.
  renderCounters();
})();
