/*
 * Osnias Network — Node Administration
 * Version 0.3.1
 *
 * TESTNET SECURITY NOTICE
 *
 * This Node 1 administrator interface currently uses a static client-side
 * wallet address check to authorize access.
 *
 * This mechanism is acceptable for TESTNET development only.
 *
 * BEFORE MAINNET DEPLOYMENT:
 * - authorization MUST be enforced server-side;
 * - the administrator MUST authenticate through a signed wallet challenge;
 * - protected actions MUST require a secure authenticated session;
 * - the authorized administrator address MUST NOT be relied upon solely
 *   from public client-side JavaScript;
 * - administrative API endpoints MUST independently verify authorization.
 *
 * Do not use this static authorization model in production/mainnet.
 */

(function () {
  "use strict";

  const ADMIN_WALLET = "0x9FcC10582df1dC2E7Da79653535Fd3e5b8De2570".toLowerCase();
  const $ = (id) => document.getElementById(id);

  let connectedAdminWallet = null;

  function authMessage(message, type) {
    const el = $("adminAuthMessage");
    if (!el) return;

    el.className = "osnias-alert";
    if (type === "success") el.classList.add("osnias-alert--success");
    if (type === "danger") el.classList.add("osnias-alert--danger");
    el.textContent = message;
  }

  function setAdminAccess(allowed) {
    const protectedContent = $("adminProtectedContent");
    const badge = $("adminAuthStatus");
    const disconnectButton = $("adminDisconnectWallet");
    const gate = $("adminAuthGate");

    if (protectedContent) protectedContent.hidden = !allowed;
    if (disconnectButton) disconnectButton.hidden = !connectedAdminWallet;

    if (badge) {
      if (allowed) {
        badge.textContent = "CONNECTÉ";
        badge.className = "osnias-badge osnias-badge--success";
      } else if (connectedAdminWallet) {
        badge.textContent = "NON AUTORISÉ";
        badge.className = "osnias-badge osnias-badge--danger";
      } else {
        badge.textContent = "NON CONNECTÉ";
        badge.className = "osnias-badge osnias-badge--pending";
      }
    }

    if (gate) {
      gate.classList.toggle("osnias-card--success", Boolean(allowed));
    }
  }

  function updateWalletDisplay(address) {
    const field = $("adminConnectedWallet");
    const headerAddress = $("adminHeaderWalletAddress");
    const authorization = $("adminWalletAuthorization");
    const allowed = Boolean(address && address.toLowerCase() === ADMIN_WALLET);

    if (field) {
      field.textContent = address || "—";
      field.style.color = allowed ? "var(--green)" : address ? "var(--red)" : "var(--white)";
      field.style.borderColor = allowed ? "#24563f" : address ? "#683838" : "var(--line)";
    }

    if (headerAddress) {
      headerAddress.textContent = address || "—";
      headerAddress.style.color = allowed ? "var(--green)" : address ? "var(--red)" : "var(--white)";
    }

    if (!authorization) return;

    if (!address) {
      authorization.textContent = "En attente";
      authorization.style.color = "var(--white)";
      return;
    }

    authorization.textContent = allowed ? "AUTORISÉ" : "NON AUTORISÉ";
    authorization.style.color = allowed ? "var(--green)" : "var(--red)";
  }

  async function connectAdminWallet() {
    if (!window.OsniasWallet || typeof window.OsniasWallet.connect !== "function") {
      throw new Error(
        "wallet-connect.js n’est pas disponible. Vérifiez /node/assets/wallet-connect.js."
      );
    }

    const walletState = await window.OsniasWallet.connect();
    const address = walletState?.account;

    if (!address) {
      throw new Error("Le wallet n’a retourné aucune adresse.");
    }

    connectedAdminWallet = address;
    updateWalletDisplay(address);

    if (address.toLowerCase() !== ADMIN_WALLET) {
      setAdminAccess(false);
      authMessage(
        "Accès refusé : ce wallet n’est pas autorisé comme administrateur du Node 1.",
        "danger"
      );
      return;
    }

    setAdminAccess(true);
    authMessage(
      "Wallet Node 1 Admin reconnu sur Ethereum Sepolia. Accès testnet autorisé.",
      "success"
    );

    // Testnet only: protected admin content is now visible.
    // Backend/API data can be loaded when available.
    if (typeof window.OSNIAS_ADMIN_LOAD_ALL === "function") {
      await window.OSNIAS_ADMIN_LOAD_ALL();
    }
  }

  function disconnectAdminWallet() {
    if (
      window.OsniasWallet &&
      typeof window.OsniasWallet.disconnectLocal === "function"
    ) {
      window.OsniasWallet.disconnectLocal("adminLogout");
    }

    connectedAdminWallet = null;
    updateWalletDisplay(null);
    setAdminAccess(false);

    const badge = $("adminAuthStatus");
    if (badge) {
      badge.textContent = "NON CONNECTÉ";
      badge.className = "osnias-badge osnias-badge--pending";
    }

    authMessage("Wallet administrateur déconnecté.");
  }

  $("adminConnectWallet")?.addEventListener("click", async () => {
    try {
      await connectAdminWallet();
    } catch (error) {
      setAdminAccess(false);
      authMessage(error?.message || "Connexion wallet impossible.", "danger");
    }
  });

  $("adminDisconnectWallet")?.addEventListener("click", disconnectAdminWallet);

  document.addEventListener("osnias:wallet-account-changed", (event) => {
    const address = event.detail?.account || null;
    connectedAdminWallet = address;
    updateWalletDisplay(address);

    const allowed = address && address.toLowerCase() === ADMIN_WALLET;
    setAdminAccess(Boolean(allowed));

    if (allowed) {
      authMessage(
        "Wallet Node 1 Admin reconnu. Accès testnet autorisé.",
        "success"
      );
    } else if (address) {
      authMessage(
        "Accès refusé : wallet administrateur incorrect.",
        "danger"
      );
    } else {
      authMessage("Aucun wallet connecté.");
    }
  });

  document.addEventListener("osnias:wallet-restored", (event) => {
    const address = event.detail?.account || null;
    connectedAdminWallet = address;
    updateWalletDisplay(address);

    const allowed = address && address.toLowerCase() === ADMIN_WALLET;
    setAdminAccess(Boolean(allowed));

    if (allowed) {
      authMessage(
        "Wallet Node 1 Admin restauré. Accès testnet autorisé.",
        "success"
      );
    }
  });

  document.addEventListener("osnias:wallet-disconnected", () => {
    connectedAdminWallet = null;
    updateWalletDisplay(null);
    setAdminAccess(false);

    const badge = $("adminAuthStatus");
    if (badge) {
      badge.textContent = "NON CONNECTÉ";
      badge.className = "osnias-badge osnias-badge--pending";
    }

    authMessage("Wallet administrateur déconnecté.");
  });

  document.addEventListener("osnias:wallet-wrong-network", () => {
    setAdminAccess(false);
    authMessage(
      "Ethereum Sepolia est requis pour l’administration du Node 1.",
      "danger"
    );
  });

  document.addEventListener("osnias:wallet-error", (event) => {
    setAdminAccess(false);
    authMessage(event.detail?.message || "Erreur wallet.", "danger");
  });

  updateWalletDisplay(null);
  setAdminAccess(false);

  const badge = $("adminAuthStatus");
  if (badge) {
    badge.textContent = "NON CONNECTÉ";
    badge.className = "osnias-badge osnias-badge--pending";
  }
})();

/*
 * Osnias Network — Node Administration
 * Version 0.3.1
 *
 * UI scaffold.
 * TESTNET UI scaffold. Server-side authorization is required before mainnet.
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

  renderCounters();
})();
