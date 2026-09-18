(function(){
  "use strict";

  const defaults = {
    title: "Osnias Clearing — Blockchain Clearing Infrastructure",

    description:
      "Osnias Clearing is a blockchain clearing infrastructure on Sei Network with strict functional separation between clearing and settlement. Clearing cycles of 1, 4 and 13 weeks. Non-custodial. ISO 20022. Public testnet deployments.",

    keywords: [
      "Osnias Clearing",
      "blockchain clearing",
      "blockchain clearing infrastructure",
      "onchain clearing",
      "on-chain clearing",
      "clearing protocol",
      "clearing registry",
      "digital asset clearing",
      "institutional settlement",
      "institutional blockchain infrastructure",
      "financial market infrastructure blockchain",
      "distributed ledger clearing",
      "DLT clearing",
      "EVM clearing",
      "multichain clearing",
      "multi-chain clearing",
      "Sei Network",
      "Sei blockchain",
      "Sei EVM",
      "Sei EVM clearing",
      "Sei clearing infrastructure",
      "Sei testnet",
      "P2P clearing",
      "peer-to-peer clearing",
      "non-custodial clearing",
      "EOA-only",
      "EOA-to-EOA transfer",
      "clearing register token",
      "USD clearing token",
      "EUR clearing token",
      "multilateral netting",
      "ISO 20022 blockchain",
      "ISO 4217",
      "MOD-97",
      "CPMI-IOSCO",
      "EMIR",
      "FINMA",
      "NNN node",
      "Osnias-ID",
      "ORUSD",
      "OEURO",
      "public deployment registry",
      "smart contract deployment registry",
      "verified smart contract",
      "SeiScan verified contract",
      "Denis Bouzon",
      "Osnias Clearing Denis Bouzon",
      "ORCID 0009-0007-8894-8902"
    ].join(", "),

    siteName: "Osnias Clearing",
    type: "website",
    email: "contact@osnias-clearing.com",
    linkedin: "https://www.linkedin.com/in/denis-bouzon-3b766a437/",
    telegram: "https://t.me/osnas_clearing",
    ogImage: "https://www.osnias-clearing.com/index-picture.png",
    ogImageAlt: "Osnias Clearing — Blockchain Clearing Infrastructure on Sei Network",
    locale: "en_US"
  };

  function upsertMeta(name, content){
    if(!content) return;
    let el = document.querySelector('meta[name="' + name + '"]');
    if(!el){
      el = document.createElement("meta");
      el.setAttribute("name", name);
      document.head.appendChild(el);
    }
    el.setAttribute("content", content);
  }

  function upsertProperty(property, content){
    if(!content) return;
    let el = document.querySelector('meta[property="' + property + '"]');
    if(!el){
      el = document.createElement("meta");
      el.setAttribute("property", property);
      document.head.appendChild(el);
    }
    el.setAttribute("content", content);
  }

  function getPageConfig(){
    return window.OSNIAS_SEO || {};
  }

  function canonicalUrl(){
    const canonical = document.querySelector('link[rel="canonical"]');
    if(canonical && canonical.href){ return canonical.href; }
    return window.location.href.split("#")[0].split("?")[0];
  }

  function ensureCanonical(url){
    let canonical = document.querySelector('link[rel="canonical"]');
    if(!canonical){
      canonical = document.createElement("link");
      canonical.setAttribute("rel", "canonical");
      document.head.appendChild(canonical);
    }
    canonical.setAttribute("href", url);
  }

  function applySEO(){
    const page = getPageConfig();

    const title =
      page.title ||
      document.title ||
      defaults.title;

    const description =
      page.description ||
      document.querySelector('meta[name="description"]')?.getAttribute("content") ||
      defaults.description;

    const keywords =
      page.keywords ||
      defaults.keywords;

    const url =
      page.canonical ||
      canonicalUrl();

    const image =
      page.image || defaults.ogImage;

    const imageAlt =
      page.imageAlt || defaults.ogImageAlt;

    document.title = title;

    upsertMeta("description", description);
    upsertMeta("keywords", keywords);
    upsertMeta("robots", page.robots || "index,follow");
    upsertMeta("author", "Denis Bouzon");
    upsertMeta("author-linkedin", defaults.linkedin);
    upsertMeta("contact-email", defaults.email);
    upsertMeta("contact-telegram", defaults.telegram);

    ensureCanonical(url);

    upsertProperty("og:title", title);
    upsertProperty("og:description", description);
    upsertProperty("og:type", page.type || defaults.type);
    upsertProperty("og:site_name", defaults.siteName);
    upsertProperty("og:url", url);
    upsertProperty("og:locale", defaults.locale);
    upsertProperty("og:image", image);
    upsertProperty("og:image:secure_url", image);
    upsertProperty("og:image:alt", imageAlt);
    upsertProperty("og:profile:first_name", "Denis");
    upsertProperty("og:profile:last_name", "Bouzon");
    upsertProperty("article:author", defaults.linkedin);
  }

  if(document.readyState === "loading"){
    document.addEventListener("DOMContentLoaded", applySEO);
  }else{
    applySEO();
  }

})();
