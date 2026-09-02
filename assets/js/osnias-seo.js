(function(){
  "use strict";

  const defaults = {
    title: "Osnias Clearing — Blockchain Clearing Infrastructure",

    description:
      "Osnias Clearing is a multichain blockchain clearing infrastructure project built around Sei EVM, on-chain clearing registries, institutional settlement, oracle-controlled execution, restricted EOA-to-EOA transfers and public smart-contract deployment verification.",

    keywords: [
      "Osnias Clearing",
      "blockchain clearing",
      "blockchain clearing infrastructure",
      "onchain clearing",
      "on-chain clearing",
      "clearing protocol",
      "clearing registry",
      "crypto clearing registry",
      "blockchain registry",
      "digital asset clearing",
      "crypto settlement infrastructure",
      "institutional settlement",
      "institutional blockchain infrastructure",
      "financial market infrastructure blockchain",
      "tokenized settlement",
      "distributed ledger clearing",
      "DLT clearing",
      "EVM clearing",
      "EVM clearing infrastructure",
      "EVM compatible clearing",
      "multichain clearing",
      "multichain clearing infrastructure",
      "multi-chain clearing",
      "Sei",
      "Sei blockchain",
      "Sei EVM",
      "Sei EVM clearing",
      "Sei clearing infrastructure",
      "Sei settlement infrastructure",
      "Sei institutional finance",
      "Sei financial infrastructure",
      "Sei testnet",
      "Sei mainnet",
      "oracle-controlled clearing",
      "oracle-controlled registry",
      "blockchain oracle settlement",
      "oracle settlement infrastructure",
      "P2P clearing",
      "P2P blockchain clearing",
      "peer-to-peer clearing",
      "peer-to-peer settlement",
      "clearing-only blockchain",
      "clearing-only protocol",
      "non-DeFi clearing",
      "non-DeFi financial infrastructure",
      "EOA-only",
      "EOA-only token",
      "EOA-to-EOA transfer",
      "restricted ERC-20",
      "restricted clearing token",
      "blockchain compensation registry",
      "clearing register token",
      "USD clearing token",
      "EUR clearing token",
      "governance token Sei",
      "crypto clearing infrastructure",
      "blockchain settlement layer",
      "public deployment registry",
      "official deployment registry",
      "smart contract deployment registry",
      "verified smart contract",
      "SeiScan verified contract",
      "ORUSD",
      "OEURO",
      "OSNIAS"
    ].join(", "),

    siteName: "Osnias Clearing",
    type: "website"
  };

  function upsertMeta(name, content){
    if(!content) return;

    let el = document.querySelector(
      'meta[name="' + name + '"]'
    );

    if(!el){
      el = document.createElement("meta");
      el.setAttribute("name", name);
      document.head.appendChild(el);
    }

    el.setAttribute("content", content);
  }

  function upsertProperty(property, content){
    if(!content) return;

    let el = document.querySelector(
      'meta[property="' + property + '"]'
    );

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
    const canonical =
      document.querySelector(
        'link[rel="canonical"]'
      );

    if(canonical && canonical.href){
      return canonical.href;
    }

    return window.location.href
      .split("#")[0]
      .split("?")[0];
  }

  function ensureCanonical(url){
    let canonical =
      document.querySelector(
        'link[rel="canonical"]'
      );

    if(!canonical){
      canonical = document.createElement("link");

      canonical.setAttribute(
        "rel",
        "canonical"
      );

      document.head.appendChild(
        canonical
      );
    }

    canonical.setAttribute(
      "href",
      url
    );
  }

  function applySEO(){
    const page = getPageConfig();

    const title =
      page.title ||
      document.title ||
      defaults.title;

    const description =
      page.description ||
      document
        .querySelector(
          'meta[name="description"]'
        )
        ?.getAttribute("content") ||
      defaults.description;

    const keywords =
      page.keywords ||
      defaults.keywords;

    const url =
      page.canonical ||
      canonicalUrl();

    document.title = title;

    upsertMeta(
      "description",
      description
    );

    upsertMeta(
      "keywords",
      keywords
    );

    upsertMeta(
      "robots",
      page.robots || "index,follow"
    );

    ensureCanonical(url);

    upsertProperty(
      "og:title",
      title
    );

    upsertProperty(
      "og:description",
      description
    );

    upsertProperty(
      "og:type",
      page.type || defaults.type
    );

    upsertProperty(
      "og:site_name",
      defaults.siteName
    );

    upsertProperty(
      "og:url",
      url
    );

    upsertMeta(
      "twitter:card",
      "summary"
    );

    upsertMeta(
      "twitter:title",
      title
    );

    upsertMeta(
      "twitter:description",
      description
    );
  }

  if(document.readyState === "loading"){
    document.addEventListener(
      "DOMContentLoaded",
      applySEO
    );
  }else{
    applySEO();
  }

})();