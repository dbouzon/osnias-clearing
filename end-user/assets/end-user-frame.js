/*
 * Osnias Clearing — End-User Console
 * Shared frame / navigation
 *
 * Copyright © 2026 Denis Bouzon / Osnias Clearing
 * ORCID: 0009-0007-8894-8902
 * Website: https://www.osnias-clearing.com
 *
 * UNLICENSED — ALL RIGHTS RESERVED.
 * No license or permission is granted to use, copy, modify, distribute,
 * sublicense, publish, deploy, commercialize, or create derivative works
 * from this source code without the prior written authorization of the author.
 *
 * File: /assets/end-user-frame.js
 * Version: 1.9.1
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

  const FRAME_VERSION = "1.9.1";

  const DEFAULT_CONFIG = Object.freeze({
    brand: "Osnias ORUSD Clearing Desk",
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
      { key: "sent", label: "Instructions Sent", href: "invoices-sent.html" },
      { key: "received", label: "Instructions Received", href: "invoices-received.html" },
      { key: "p2p", label: "Transfers", href: "p2p-settlement.html" },
      { key: "registry", label: "Register", href: "registry.html" },
      { key: "burn", label: "Burn", href: "burn.html" }
    ]
  });

  const state = {
    walletConnected: false,
    walletAddress: "",
    cycleNumber: null,
    cycleWindow: "—",
    messagingOpen: null,
    clearingAt: null
  };

  let countdownTimer = null;

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
            class="osnias-nav__link osnias-nav__link--boxed${active ? " is-active" : ""}"
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

  function normalizeTimestamp(value) {
    if (value === null || value === undefined || value === "") return null;

    const numeric = Number(value);
    if (Number.isFinite(numeric)) {
      // Accept Unix seconds or milliseconds.
      return numeric < 1e12 ? numeric * 1000 : numeric;
    }

    const parsed = Date.parse(String(value));
    return Number.isFinite(parsed) ? parsed : null;
  }

  function countdownParts(targetMs) {
    if (!targetMs) return null;

    const diff = Math.max(0, targetMs - Date.now());
    const totalSeconds = Math.floor(diff / 1000);

    const days = Math.floor(totalSeconds / 86400);
    const hours = Math.floor((totalSeconds % 86400) / 3600);
    const minutes = Math.floor((totalSeconds % 3600) / 60);
    const seconds = totalSeconds % 60;

    return {
      diff,
      days,
      hours,
      minutes,
      seconds,
      text:
        `${String(days).padStart(2, "0")}d ` +
        `${String(hours).padStart(2, "0")}h ` +
        `${String(minutes).padStart(2, "0")}m ` +
        `${String(seconds).padStart(2, "0")}s`
    };
  }

  function clearingCountdownMarkup() {
    const targetMs = normalizeTimestamp(state.clearingAt);
    const windowName = String(state.cycleWindow || "—").toUpperCase();

    if (windowName === "CLEARING") {
      return {
        label: "CLEARING",
        value: "OPEN",
        className: " osnias-cyclebar__value--open"
      };
    }

    if (!targetMs) {
      return {
        label: "CLEARING IN",
        value: "—",
        className: ""
      };
    }

    const parts = countdownParts(targetMs);

    if (!parts || parts.diff <= 0) {
      return {
        label: "CLEARING",
        value: "PENDING",
        className: " osnias-cyclebar__value--closed"
      };
    }

    let className = " osnias-cyclebar__value--open";

    // Countdown urgency:
    // > 24 h  : normal green
    // <= 24 h : warning orange
    // <= 3 h  : critical red
    if (parts.diff <= 3 * 60 * 60 * 1000) {
      className = " osnias-cyclebar__value--closed";
    } else if (parts.diff <= 24 * 60 * 60 * 1000) {
      className = " osnias-cyclebar__value--warning";
    }

    return {
      label: "CLEARING IN",
      value: parts.text,
      className
    };
  }

  function startCountdownTimer() {
    if (countdownTimer !== null) {
      window.clearInterval(countdownTimer);
      countdownTimer = null;
    }

    if (!normalizeTimestamp(state.clearingAt)) return;

    countdownTimer = window.setInterval(() => {
      const value = document.getElementById("osnias-clearing-countdown");
      const label = document.getElementById("osnias-clearing-countdown-label");
      if (!value || !label) return;

      const countdown = clearingCountdownMarkup();
      label.textContent = countdown.label;
      value.textContent = countdown.value;
      value.className = `osnias-cyclebar__value${countdown.className}`;
    }, 1000);
  }

  function cycleMarkup() {
    const cycle =
      state.cycleNumber === null || state.cycleNumber === undefined
        ? "—"
        : String(state.cycleNumber);

    const windowName = String(state.cycleWindow || "—").toUpperCase();
    const hasWindow = ["BURN", "MINT", "CLEARING"].includes(windowName);

    const mintOpen = hasWindow ? windowName === "MINT" : null;
    const burnOpen = hasWindow ? windowName === "BURN" : null;
    const clearingOpen = hasWindow ? windowName === "CLEARING" : null;
    const clearingCountdown = clearingCountdownMarkup();

    const messagesOpen =
      state.messagingOpen === true
        ? true
        : state.messagingOpen === false
          ? false
          : hasWindow
            ? windowName !== "CLEARING"
            : null;

    function statusText(value) {
      if (value === true) return "OPEN";
      if (value === false) return "CLOSED";
      return "—";
    }

    function statusClass(value) {
      if (value === true) return " osnias-cyclebar__value--open";
      if (value === false) return " osnias-cyclebar__value--closed";
      return "";
    }

    return `
      <div class="osnias-cyclebar osnias-cyclebar--header osnias-cyclebar--operations osnias-cyclebar--fullwidth" id="osnias-cyclebar">
        <span class="osnias-cyclebar__item">
          <span class="osnias-cyclebar__label">Cycle</span>
          <span class="osnias-cyclebar__value" id="osnias-cycle-number">${escapeHtml(cycle)}</span>
        </span>

        <span class="osnias-separator" aria-hidden="true"></span>

        <span class="osnias-cyclebar__item">
          <span class="osnias-cyclebar__label">MINT</span>
          <span class="osnias-cyclebar__value${statusClass(mintOpen)}" id="osnias-mint-status">${statusText(mintOpen)}</span>
        </span>

        <span class="osnias-separator" aria-hidden="true"></span>

        <span class="osnias-cyclebar__item">
          <span class="osnias-cyclebar__label">BURN</span>
          <span class="osnias-cyclebar__value${statusClass(burnOpen)}" id="osnias-burn-status">${statusText(burnOpen)}</span>
        </span>

        <span class="osnias-separator" aria-hidden="true"></span>

        <span class="osnias-cyclebar__item">
          <span class="osnias-cyclebar__label">CLEARING</span>
          <span class="osnias-cyclebar__value${statusClass(clearingOpen)}" id="osnias-clearing-status">${statusText(clearingOpen)}</span>
        </span>

        <span class="osnias-separator" aria-hidden="true"></span>

        <span class="osnias-cyclebar__item">
          <span class="osnias-cyclebar__label">INSTRUCTIONS</span>
          <span class="osnias-cyclebar__value${statusClass(messagesOpen)}" id="osnias-messaging-status">${statusText(messagesOpen)}</span>
        </span>

        <span class="osnias-cyclebar__countdown-gap osnias-cyclebar__countdown-gap--compact" aria-hidden="true"></span>

        <span class="osnias-cyclebar__item osnias-cyclebar__item--countdown">
          <span class="osnias-cyclebar__label" id="osnias-clearing-countdown-label">${escapeHtml(clearingCountdown.label)}</span>
          <span class="osnias-cyclebar__value${clearingCountdown.className}" id="osnias-clearing-countdown">${escapeHtml(clearingCountdown.value)}</span>
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

  function networkMarkup() {
    return `
      <div class="osnias-networkbar osnias-networkbar--inline" id="osnias-networkbar">
        <span class="osnias-networkbar__wallet">
          ${walletMarkup()}
        </span>

        <span class="osnias-separator" aria-hidden="true"></span>

        <span class="osnias-cyclebar__item">
          <span class="osnias-cyclebar__label">Network</span>
          <span class="osnias-cyclebar__value" id="osnias-network-name">${escapeHtml(currentConfig.networkLabel)}</span>
        </span>

        <span class="osnias-separator" aria-hidden="true"></span>

        <span class="osnias-cyclebar__item">
          <span class="osnias-cyclebar__label">Chain ID</span>
          <span class="osnias-cyclebar__value" id="osnias-chain-id">${escapeHtml(currentConfig.chainIdLabel)}</span>
        </span>
      </div>
    `;
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
              data-osnias-logo
            >
            <span class="osnias-brand__copy">
              <span class="osnias-brand__name">${escapeHtml(currentConfig.brand)}</span>
              ${currentConfig.subtitle ? `<span class="osnias-brand__subline">${escapeHtml(currentConfig.subtitle)}</span>` : ""}
            </span>
          </a>

          <nav class="osnias-nav" aria-label="End-user navigation">
            ${renderNavigation(activeKey)}
          </nav>
        </div>

        <div class="osnias-header__network" id="osnias-header-network">
          <div class="osnias-header__network-inner">
            ${networkMarkup()}
          </div>
        </div>

        <div class="osnias-header__status" id="osnias-header-status">
          <div class="osnias-header__status-inner">
            ${cycleMarkup()}
          </div>
        </div>
      </header>
    `;

    bindLogoFallback();
    bindWalletButton();
    startCountdownTimer();
  }

  function renderNetworkBar() {
    const mount = document.getElementById("osnias-header-network");
    if (!mount) return;

    mount.innerHTML = `
      <div class="osnias-header__network-inner">
        ${networkMarkup()}
      </div>
    `;

    bindWalletButton();
  }

  function renderCycleBar() {
    const mount = document.getElementById("osnias-header-status");
    if (!mount) return;

    mount.innerHTML = `
      <div class="osnias-header__status-inner">
        ${cycleMarkup()}
      </div>
    `;

    startCountdownTimer();
  }

  function renderFooter() {
    const mount = document.getElementById("osnias-footer");
    if (!mount) return;

    const year = new Date().getUTCFullYear();

    mount.innerHTML = `
      <footer class="osnias-footer">
        Osnias Clearing · ORUSD Clearing Desk · Frame 1.9.1 · 2026-09-11
      </footer>
    `;
  }

  function bindWalletButton() {
    const button = document.getElementById("osnias-wallet-connect");
    if (!button) return;

    button.addEventListener("click", async () => {
      if (state.walletConnected) {
        button.disabled = true;
        button.textContent = "Disconnecting…";

        try {
          // Preferred path: call the wallet layer directly.
          if (
            window.OsniasWallet &&
            typeof window.OsniasWallet.disconnect === "function"
          ) {
            await window.OsniasWallet.disconnect();
            return;
          }

          // Compatibility fallback for an older cached wallet-connect.js.
          if (
            window.OsniasWallet &&
            typeof window.OsniasWallet.disconnectLocal === "function"
          ) {
            window.OsniasWallet.disconnectLocal("headerFallback");
            return;
          }

          // Last-resort event path.
          document.dispatchEvent(
            new CustomEvent("osnias:wallet-disconnect-request", {
              detail: { source: "end-user-frame" }
            })
          );
        } catch (error) {
          console.error("[OsniasFrame] wallet disconnect failed", error);

          // Fail closed on the application side even if wallet revocation fails.
          setWallet({ connected: false, address: "" });

          document.dispatchEvent(
            new CustomEvent("osnias:wallet-disconnected", {
              detail: {
                localOnly: true,
                reason: "headerDisconnectFallback",
                message: error?.message || "Wallet disconnect failed."
              }
            })
          );
        } finally {
          const current = document.getElementById("osnias-wallet-connect");
          if (current) current.disabled = false;
        }

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
    renderNetworkBar();
    renderCycleBar();
    renderFooter();
  }

  function setWallet({ connected, address } = {}) {
    state.walletConnected = Boolean(connected);
    state.walletAddress = state.walletConnected ? String(address || "") : "";
    renderNetworkBar();
  }

  function setCycle({ cycleNumber, window, messagingOpen, clearingAt } = {}) {
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

    if (clearingAt !== undefined) {
      state.clearingAt = normalizeTimestamp(clearingAt);
    }

    renderCycleBar();
  }

  function setNetwork({ name, chainId } = {}) {
    if (name) currentConfig.networkLabel = String(name);
    if (chainId !== undefined && chainId !== null) {
      currentConfig.chainIdLabel = String(chainId);
    }
    renderNetworkBar();
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
