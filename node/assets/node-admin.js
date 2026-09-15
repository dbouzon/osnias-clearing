/*
 * Osnias Network — Node Administration
 * Version 0.1.0
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

  // No automatic loading in production until server-side admin authentication exists.
  renderCounters();
})();
