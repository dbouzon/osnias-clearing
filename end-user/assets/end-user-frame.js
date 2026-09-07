/*
 * Osnias Clearing — End-User Console
 * Shared frame / navigation
 *
 * File: /assets/end-user-frame.js
 * Version: 1.4.1
 *
 * Responsibilities:
 * - Render the common institutional header
 * - Render main navigation
 * - Expose wallet/cycle placeholders for later provider modules
 * - Highlight the active page
 * - Render a common footer
 *
 * This file does NOT connect to the blockchain by itself.
 * Blockchain state will be injected later by sei-provider.js and wallet-connect.js.
 */

(() => {
  "use strict";

  const FRAME_VERSION = "1.4.1";

  const DEFAULT_CONFIG = Object.freeze({
    brand: "Osnias Orusd End User",
    subtitle: "",
    logoUrl: "logo.jpg",
    logoFallbackUrls: [
      "./logo.jpg",
      "./assets/logo.jpg",
      "/end-user/logo.jpg",
      "/end-user/assets/logo.jpg",
      "/logo.jpg",
      "/assets/logo.jpg"
    ],
    rootPath: "/end-user/",
    documentationUrl: "/documentation/",
    networkLabel: "Sei Atlantic-2",
    chainIdLabel: "1328",
    nav: [
      { key: "dashboard", label: "Dashboard", href: "index.html" },
      { key: "sent", label: "Invoice Sent", href: "invoices-sent.html" },
      { key: "received", label: "Invoice Received", href: "invoices-received.html" },
      { key: "p2p", label: "Settlement", href: "p2p-settlement.html" },
      { key: "registry", label: "Registry", href: "registry.html" }
    ]
  });

  const state = {
    walletConnected: false,
    walletAddress: "",
    cycleNumber: null,
    cycleWindow: "—",
    messagingOpen: null
  };

  let currentConfig = { ...DEFAULT_CONFIG };

  function escapeHtml(value) {
    return String(value ?? "")
      .replaceAll("&", "&amp;")
      .replaceAll("<", "&lt;")
      .replaceAll(">", "&gt;")
      .replaceAll('"', "&quot;")
      .replaceAll("'", "&#039;");
  }

  function normalizePath(path) {
    return String(path || "").replace(/\\/g, "/");
  }

  function currentFileName() {
    const path = normalizePath(window.location.pathname);
    const last = path.split("/").filter(Boolean).pop();
    return last || "index.html";
  }

  function detectActiveKey() {
    const file = currentFileName();

    const found = currentConfig.nav.find((item) => {
      return item.href === file || item.href.endsWith("/" + file);
    });

    return found ? found.key : "dashboard";
  }

  function resolveHref(href) {
    if (
      href.startsWith("http://") ||
      href.startsWith("https://") ||
      href.startsWith("/") ||
      href.startsWith("#")
    ) {
      return href;
    }

    return href;
  }

  function shortAddress(address) {
    const value = String(address || "");
    if (value.length < 12) return value;
    return `${value.slice(0, 6)}…${value.slice(-4)}`;
  }

  function renderNavigation(activeKey) {
    return currentConfig.nav
      .map((item) => {
        const active = item.key === activeKey;
        return `
          <a
            class="osnias-nav__link${active ? " is-active" : ""}"
            href="${escapeHtml(resolveHref(item.href))}"
            data-osnias-nav="${escapeHtml(item.key)}"
            ${active ? 'aria-current="page"' : ""}
          >${escapeHtml(item.label)}</a>
        `;
      })
      .join("");
  }

  function walletMarkup() {
    const connectedClass = state.walletConnected ? " is-connected" : "";
    const address = state.walletConnected
      ? shortAddress(state.walletAddress)
      : "Not connected";

    return `
      <div class="osnias-wallet${connectedClass}" id="osnias-wallet-status">
        <span class="osnias-wallet__dot" aria-hidden="true"></span>
        <span
          class="osnias-wallet__address"
          id="osnias-wallet-address"
          title="${escapeHtml(state.walletAddress)}"
        >${escapeHtml(address)}</span>
        <button
          type="button"
          class="osnias-btn osnias-btn--primary"
          id="osnias-wallet-connect"
        >${state.walletConnected ? "Wallet Connected" : "Connect Wallet"}</button>
      </div>
    `;
  }

  function cycleMarkup() {
    const cycle =
      state.cycleNumber === null || state.cycleNumber === undefined
        ? "—"
        : String(state.cycleNumber);

    const windowName = state.cycleWindow || "—";

    let windowClass = "";
    if (windowName === "CLEARING") {
      windowClass = " osnias-cyclebar__value--closed";
    } else if (windowName === "BURN" || windowName === "MINT") {
      windowClass = " osnias-cyclebar__value--open";
    }

    let messagingValue = "—";
    let messagingClass = "";

    if (state.messagingOpen === true) {
      messagingValue = "OPEN";
      messagingClass = " osnias-cyclebar__value--open";
    } else if (state.messagingOpen === false) {
      messagingValue = "CLOSED";
      messagingClass = " osnias-cyclebar__value--closed";
    }

    return `
      <div class="osnias-cyclebar" id="osnias-cyclebar">
        <span class="osnias-cyclebar__item">
          <span class="osnias-cyclebar__label">Network</span>
          <span class="osnias-cyclebar__value" id="osnias-network-name">
            ${escapeHtml(currentConfig.networkLabel)}
          </span>
        </span>

        <span class="osnias-separator" aria-hidden="true"></span>

        <span class="osnias-cyclebar__item">
          <span class="osnias-cyclebar__label">Chain ID</span>
          <span class="osnias-cyclebar__value" id="osnias-chain-id">
            ${escapeHtml(currentConfig.chainIdLabel)}
          </span>
        </span>

        <span class="osnias-separator" aria-hidden="true"></span>

        <span class="osnias-cyclebar__item">
          <span class="osnias-cyclebar__label">Cycle</span>
          <span class="osnias-cyclebar__value" id="osnias-cycle-number">
            ${escapeHtml(cycle)}
          </span>
        </span>

        <span class="osnias-separator" aria-hidden="true"></span>

        <span class="osnias-cyclebar__item">
          <span class="osnias-cyclebar__label">Window</span>
          <span
            class="osnias-cyclebar__value${windowClass}"
            id="osnias-cycle-window"
          >${escapeHtml(windowName)}</span>
        </span>

        <span class="osnias-separator" aria-hidden="true"></span>

        <span class="osnias-cyclebar__item">
          <span class="osnias-cyclebar__label">Messages</span>
          <span
            class="osnias-cyclebar__value${messagingClass}"
            id="osnias-messaging-status"
          >${escapeHtml(messagingValue)}</span>
        </span>
      </div>
    `;
  }


  function bindLogoFallback() {
    const img = document.querySelector("[data-osnias-logo]");
    if (!img) return;

    const candidates = [
      currentConfig.logoUrl,
      ...(Array.isArray(currentConfig.logoFallbackUrls)
        ? currentConfig.logoFallbackUrls
        : [])
    ].filter(Boolean);

    const uniqueCandidates = [...new Set(candidates)];
    let candidateIndex = 0;

    const tryNext = () => {
      candidateIndex += 1;

      if (candidateIndex < uniqueCandidates.length) {
        img.hidden = false;
        img.src = uniqueCandidates[candidateIndex];
        return;
      }

      // Never display a broken-image icon in the institutional header.
      img.hidden = true;
    };

    img.addEventListener("load", () => {
      img.hidden = false;
    }, { passive: true });

    img.addEventListener("error", tryNext, { passive: true });
  }

  function renderHeader() {
    const mount = document.getElementById("osnias-header");
    if (!mount) return;

    const activeKey = mount.dataset.active || detectActiveKey();

    mount.innerHTML = `
      <header class="osnias-header">
        <div class="osnias-header__inner">
          <a class="osnias-brand" href="${escapeHtml(currentConfig.rootPath)}">
            <img
              class="osnias-brand__logo"
              src="${escapeHtml(currentConfig.logoUrl)}"
              alt="Osnias Clearing"
              width="42"
              height="42"
              data-osnias-logo
            >
            <span class="osnias-brand__copy">
              <span class="osnias-brand__name">${escapeHtml(currentConfig.brand)}</span>
              ${currentConfig.subtitle ? `<span class="osnias-brand__subline">${escapeHtml(currentConfig.subtitle)}</span>` : ""}
            </span>
          </a>

          <div class="osnias-header__right osnias-header__wallet-first">
            ${walletMarkup()}
          </div>

          <nav class="osnias-nav" aria-label="End-user navigation">
            ${renderNavigation(activeKey)}
          </nav>
        </div>
      </header>
    `;

    bindLogoFallback();
    bindWalletButton();
  }

  function renderCycleBar() {
    const mount = document.getElementById("osnias-cycle-status");
    if (!mount) return;
    mount.innerHTML = cycleMarkup();
  }

  function renderFooter() {
    const mount = document.getElementById("osnias-footer");
    if (!mount) return;

    const year = new Date().getUTCFullYear();

    mount.innerHTML = `
      <footer class="osnias-footer">
        Osnias Clearing · End-User Console · Frame 1.4.1 · 2026-09-07
      </footer>
    `;
  }

  function bindWalletButton() {
    const button = document.getElementById("osnias-wallet-connect");
    if (!button) return;

    button.addEventListener("click", () => {
      if (state.walletConnected) {
        document.dispatchEvent(
          new CustomEvent("osnias:wallet-disconnect-request", {
            detail: { source: "end-user-frame" }
          })
        );
        return;
      }

      document.dispatchEvent(
        new CustomEvent("osnias:wallet-connect-request", {
          detail: { source: "end-user-frame" }
        })
      );
    });
  }

  function refreshFrame() {
    renderHeader();
    renderCycleBar();
    renderFooter();
  }

  function setWallet({ connected, address } = {}) {
    state.walletConnected = Boolean(connected);
    state.walletAddress = state.walletConnected ? String(address || "") : "";
    renderHeader();
  }

  function setCycle({ cycleNumber, window, messagingOpen } = {}) {
    if (cycleNumber !== undefined) {
      state.cycleNumber = cycleNumber;
    }

    if (window !== undefined) {
      state.cycleWindow = String(window || "—").toUpperCase();
    }

    if (messagingOpen !== undefined) {
      state.messagingOpen = Boolean(messagingOpen);
    } else if (window !== undefined) {
      state.messagingOpen = state.cycleWindow !== "CLEARING";
    }

    renderCycleBar();
  }

  function setNetwork({ name, chainId } = {}) {
    if (name) currentConfig.networkLabel = String(name);
    if (chainId !== undefined && chainId !== null) {
      currentConfig.chainIdLabel = String(chainId);
    }
    renderCycleBar();
  }

  function configure(options = {}) {
    currentConfig = {
      ...currentConfig,
      ...options,
      nav: Array.isArray(options.nav) ? options.nav : currentConfig.nav
    };

    refreshFrame();
  }

  function init() {
    refreshFrame();

    document.dispatchEvent(
      new CustomEvent("osnias:frame-ready", {
        detail: {
          version: FRAME_VERSION
        }
      })
    );
  }

  window.OsniasFrame = Object.freeze({
    version: FRAME_VERSION,
    init,
    refresh: refreshFrame,
    configure,
    setWallet,
    setCycle,
    setNetwork,
    getState: () => ({ ...state })
  });

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init, { once: true });
  } else {
    init();
  }
})();
