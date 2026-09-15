/*
 * Osnias Clearing — Corporate KYC Controller
 * Version 0.1.0 — Ethereum Sepolia Testnet
 *
 * KYC creates a provisional corporate User ID (UID): KK-NNN-SSSSSSS.
 * It does NOT create an Osnias-ID (OID) clearing account.
 * The Node validates the UID before clearing-room forms become accessible.
 */
(() => {
  "use strict";

  const CONFIG = Object.freeze({
    environment: "testnet",
    kk: "01",
    nnn: "001",
    submitEndpoint: "",
    provisionalUidStorageKey: "osnias.kyc.pending",
    ...(window.OSNIAS_KYC_CONFIG || {})
  });

  const EXPECTED_CHAIN_ID = 11155111;
  const state = { connected: false, account: "", chainId: null };

  const $ = (id) => document.getElementById(id);
  const form = $("kyc-form");
  const fieldset = $("kyc-fields");

  function setAlert(el, message, kind = "neutral") {
    if (!el) return;
    el.textContent = message;
    el.classList.remove("osnias-alert--success", "osnias-alert--danger");
    if (kind === "success") el.classList.add("osnias-alert--success");
    if (kind === "danger") el.classList.add("osnias-alert--danger");
  }

  function setWalletState(walletState = {}) {
    state.connected = Boolean(walletState.connected && walletState.account);
    state.account = state.connected ? String(walletState.account) : "";
    state.chainId = walletState.chainId ?? null;

    $("wallet-address").textContent = state.account || "—";
    $("bound-wallet").textContent = state.account || "—";
    $("connect-wallet").disabled = state.connected;
    $("disconnect-wallet").disabled = !state.connected;
    fieldset.disabled = !state.connected;

    const badge = $("wallet-status-badge");
    badge.className = "osnias-badge " + (state.connected ? "osnias-badge--success" : "osnias-badge--pending");
    badge.textContent = state.connected ? "Wallet vérifié sur Sepolia" : "Non connecté";

    setAlert(
      $("wallet-message"),
      state.connected
        ? `Wallet ${state.account} connecté sur Ethereum Sepolia. Le dossier KYC peut être complété.`
        : "Le formulaire reste verrouillé tant qu'un wallet n'est pas connecté sur Sepolia.",
      state.connected ? "success" : "neutral"
    );
  }

  async function connectWallet() {
    try {
      if (!window.OsniasWallet) throw new Error("Module wallet Osnias indisponible.");
      const walletState = await window.OsniasWallet.connect();
      if (Number(walletState.chainId) !== EXPECTED_CHAIN_ID) throw new Error("Le wallet n'est pas connecté à Ethereum Sepolia.");
      setWalletState(walletState);
    } catch (error) {
      setAlert($("wallet-message"), error?.message || "Échec de connexion du wallet.", "danger");
    }
  }

  async function disconnectWallet() {
    try {
      if (window.OsniasWallet) await window.OsniasWallet.disconnect();
    } finally {
      setWalletState({ connected: false });
    }
  }

  function randomDigits(length) {
    const max = 10 ** length;
    const array = new Uint32Array(1);
    crypto.getRandomValues(array);
    return String(array[0] % max).padStart(length, "0");
  }

  function makeCaseReference() {
    const now = new Date();
    const y = now.getUTCFullYear();
    const m = String(now.getUTCMonth() + 1).padStart(2, "0");
    const d = String(now.getUTCDate()).padStart(2, "0");
    return `KYC-${y}${m}${d}-${randomDigits(6)}`;
  }

  function makeProvisionalUid() {
    // Testnet-only fallback. Production allocation must be authoritative server-side.
    return `${String(CONFIG.kk).padStart(2, "0")}-${String(CONFIG.nnn).padStart(3, "0")}-${randomDigits(7)}`;
  }

  function formObject(formElement) {
    const data = new FormData(formElement);
    const out = {};
    for (const [key, value] of data.entries()) out[key] = value;
    out.controllerPep = data.has("controllerPep");
    out.authorityDeclaration = data.has("authorityDeclaration");
    out.accuracyDeclaration = data.has("accuracyDeclaration");
    out.privacyDeclaration = data.has("privacyDeclaration");
    return out;
  }

  async function signKycIntent(caseReference) {
    const signer = await window.OsniasWallet.getSigner();
    const nonce = randomDigits(10);
    const issuedAt = new Date().toISOString();
    const message = [
      "OSNIAS CLEARING — KYC REGISTRATION",
      `Environment: ${CONFIG.environment}`,
      `Network: Ethereum Sepolia (${EXPECTED_CHAIN_ID})`,
      `Case: ${caseReference}`,
      `Wallet: ${state.account}`,
      `Nonce: ${nonce}`,
      `Issued-At: ${issuedAt}`,
      "Purpose: confirm control of the wallet associated with this corporate KYC application.",
      "This signature does not authorize a blockchain transaction or transfer of assets."
    ].join("\n");

    const signature = await signer.signMessage(message);
    return { message, signature, nonce, issuedAt };
  }

  async function submitToBackend(payload) {
    if (!CONFIG.submitEndpoint) return null;
    const response = await fetch(CONFIG.submitEndpoint, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "same-origin",
      body: JSON.stringify(payload)
    });
    if (!response.ok) throw new Error(`Le serveur KYC a retourné HTTP ${response.status}.`);
    return response.json();
  }

  async function handleSubmit(event) {
    event.preventDefault();
    if (!form.reportValidity()) return;

    const submit = $("submit-kyc");
    submit.disabled = true;
    submit.textContent = "Signature et soumission…";
    $("submission-badge").className = "osnias-badge osnias-badge--pending";
    $("submission-badge").textContent = "En cours";

    try {
      const walletState = await window.OsniasWallet.ensureConnected();
      if (Number(walletState.chainId) !== EXPECTED_CHAIN_ID || walletState.account !== state.account) {
        throw new Error("Le wallet ou le réseau a changé. Reconnectez le wallet avant de soumettre.");
      }

      const caseReference = makeCaseReference();
      const walletProof = await signKycIntent(caseReference);
      const corporateKyc = formObject(form);

      const basePayload = {
        schema: "osnias-corporate-kyc-v0.1",
        environment: CONFIG.environment,
        status: "PENDING",
        caseReference,
        network: { name: "Ethereum Sepolia", chainId: EXPECTED_CHAIN_ID, kk: CONFIG.kk },
        node: { nnn: CONFIG.nnn },
        wallet: { address: state.account, proof: walletProof },
        corporateKyc,
        requestedAt: new Date().toISOString(),
        requestedAction: "ALLOCATE_PROVISIONAL_UID"
      };

      let serverResult = await submitToBackend(basePayload);
      const uid = serverResult?.provisionalUid || makeProvisionalUid();
      const finalCaseReference = serverResult?.caseReference || caseReference;

      const pendingRecord = {
        ...basePayload,
        caseReference: finalCaseReference,
        provisionalUid: uid,
        status: "PENDING_NODE_VALIDATION",
        allocationMode: serverResult ? "server" : "testnet-local"
      };

      localStorage.setItem(CONFIG.provisionalUidStorageKey, JSON.stringify(pendingRecord));

      $("provisional-uid").textContent = uid;
      $("result-wallet").textContent = state.account;
      $("case-reference").textContent = finalCaseReference;
      $("uid-result").hidden = false;
      $("submission-badge").className = "osnias-badge osnias-badge--pending";
      $("submission-badge").textContent = "PENDING";

      setAlert(
        $("submission-message"),
        serverResult
          ? `Dossier transmis. UID provisoire ${uid} réservé et placé en PENDING jusqu'à validation du Node.`
          : `Mode testnet local : UID provisoire ${uid} généré et placé en PENDING. Pour la production, renseigner submitEndpoint afin que l'allocation et l'e-mail soient effectués par le serveur du Node.`,
        "success"
      );
    } catch (error) {
      $("submission-badge").className = "osnias-badge osnias-badge--danger";
      $("submission-badge").textContent = "Échec";
      setAlert($("submission-message"), error?.message || "La soumission KYC a échoué.", "danger");
    } finally {
      submit.disabled = false;
      submit.textContent = "Signer et soumettre le dossier KYC";
    }
  }

  function bindEvents() {
    $("connect-wallet").addEventListener("click", connectWallet);
    $("disconnect-wallet").addEventListener("click", disconnectWallet);
    form.addEventListener("submit", handleSubmit);

    document.addEventListener("osnias:wallet-connected", (event) => setWalletState(event.detail));
    document.addEventListener("osnias:wallet-restored", (event) => setWalletState(event.detail));
    document.addEventListener("osnias:wallet-account-changed", (event) => setWalletState(event.detail));
    document.addEventListener("osnias:wallet-chain-changed", (event) => setWalletState(event.detail));
    document.addEventListener("osnias:wallet-disconnected", () => setWalletState({ connected: false }));
    document.addEventListener("osnias:wallet-wrong-network", () => {
      setWalletState({ connected: false });
      setAlert($("wallet-message"), "Réseau incorrect. Connectez le wallet à Ethereum Sepolia.", "danger");
    });
  }

  async function init() {
    bindEvents();
    if (window.OsniasWallet) {
      try {
        const walletState = await window.OsniasWallet.restore();
        setWalletState(walletState);
      } catch {
        setWalletState({ connected: false });
      }
    }
  }

  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", init, { once: true });
  else init();
})();
