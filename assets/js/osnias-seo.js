(function(){
  "use strict";

  const defaults = {
    title: "Osnias Clearing — Blockchain Clearing & Multilateral Netting Infrastructure",

    description:
      "Osnias Clearing is a non-custodial blockchain clearing and multilateral netting infrastructure for enterprise payments, with functional separation between clearing and settlement, ISO 20022 messaging and public testnet deployments.",

    keywords: [
      "Osnias Clearing",
      "blockchain clearing",
      "blockchain clearing infrastructure",
      "clearing and settlement",
      "clearing infrastructure",
      "multilateral netting",
      "blockchain multilateral netting",
      "payment netting",
      "enterprise payment netting",
      "enterprise payments",
      "payment infrastructure",
      "cross-border payments",
      "cross-border clearing",
      "blockchain settlement",
      "on-chain settlement",
      "onchain clearing",
      "on-chain clearing",
      "clearing protocol",
      "clearing registry",
      "distributed ledger clearing",
      "DLT clearing",
      "DLT settlement",
      "financial infrastructure",
      "digital financial infrastructure",
      "financial market infrastructure blockchain",
      "institutional blockchain infrastructure",
      "institutional settlement",
      "institutional blockchain",
      "fintech infrastructure",
      "fintech blockchain",
      "Web3 finance",
      "Web3 infrastructure",
      "tokenized finance",
      "smart contracts finance",
      "non-custodial clearing",
      "non-custodial finance",
      "P2P clearing",
      "peer-to-peer clearing",
      "EVM clearing",
      "multichain clearing",
      "multi-chain clearing",
      "Sei Network",
      "Sei blockchain",
      "Sei EVM",
      "Sei EVM clearing",
      "Sei clearing infrastructure",
      "Sei testnet",
      "EOA-only",
      "EOA-to-EOA transfer",
      "clearing register token",
      "USD clearing token",
      "EUR clearing token",
      "ISO 20022 blockchain",
      "ISO 4217",
      "CPMI-IOSCO",
      "Osnias-ID",
      "ORUSD",
      "OEURO",
      "public deployment registry",
      "smart contract deployment registry",
      "verified smart contract",
      "Denis Bouzon",
      "Osnias Clearing Denis Bouzon",
      "ORCID 0009-0007-8894-8902"
    ].join(", "),

    siteName: "Osnias Clearing",
    siteUrl: "https://www.osnias-clearing.com/",
    type: "website",
    email: "contact@osnias-clearing.com",
    linkedin: "https://www.linkedin.com/in/denis-bouzon-3b766a437/",
    telegram: "https://t.me/osnas_clearing",
    orcid: "https://orcid.org/0009-0007-8894-8902",
    zenodo: "https://zenodo.org/",
    ogImage: "https://www.osnias-clearing.com/index-picture.png",
    ogImageAlt: "Osnias Clearing — Blockchain Clearing and Multilateral Netting Infrastructure",
    locale: "en_US",
    twitterCard: "summary_large_image"
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

  function upsertLink(rel, href, hreflang){
    if(!href) return;

    let selector = 'link[rel="' + rel + '"]';
    if(hreflang){
      selector += '[hreflang="' + hreflang + '"]';
    }

    let el = document.querySelector(selector);
    if(!el){
      el = document.createElement("link");
      el.setAttribute("rel", rel);
      if(hreflang) el.setAttribute("hreflang", hreflang);
      document.head.appendChild(el);
    }
    el.setAttribute("href", href);
  }

  function upsertJsonLd(id, data){
    if(!data) return;
    let el = document.getElementById(id);
    if(!el){
      el = document.createElement("script");
      el.type = "application/ld+json";
      el.id = id;
      document.head.appendChild(el);
    }
    el.textContent = JSON.stringify(data);
  }

  function getPageConfig(){
    return window.OSNIAS_SEO || {};
  }

  function cleanUrl(url){
    try {
      const parsed = new URL(url, window.location.origin);
      parsed.hash = "";
      parsed.search = "";
      return parsed.href;
    } catch(e){
      return url;
    }
  }

  function canonicalUrl(){
    const canonical = document.querySelector('link[rel="canonical"]');
    if(canonical && canonical.href){ return cleanUrl(canonical.href); }
    return cleanUrl(window.location.href);
  }

  function ensureCanonical(url){
    upsertLink("canonical", cleanUrl(url));
  }

  function languageFromPage(page){
    return page.lang || document.documentElement.lang || "en";
  }

  function localeFromLanguage(lang, page){
    if(page.locale) return page.locale;
    return String(lang).toLowerCase().startsWith("fr") ? "fr_FR" : defaults.locale;
  }

  function applyAlternates(page){
    const alternates = page.alternates || {};
    Object.keys(alternates).forEach(function(lang){
      upsertLink("alternate", cleanUrl(alternates[lang]), lang);
    });

    if(page.xDefault){
      upsertLink("alternate", cleanUrl(page.xDefault), "x-default");
    }
  }

  function buildStructuredData(page, data){
    const authorId = defaults.siteUrl + "#denis-bouzon";
    const websiteId = defaults.siteUrl + "#website";
    const webpageId = data.url + "#webpage";

    const graph = [
      {
        "@type": "Person",
        "@id": authorId,
        "name": "Denis Bouzon",
        "url": defaults.siteUrl,
        "sameAs": [
          defaults.linkedin,
          defaults.orcid
        ]
      },
      {
        "@type": "WebSite",
        "@id": websiteId,
        "url": defaults.siteUrl,
        "name": defaults.siteName,
        "description": defaults.description,
        "inLanguage": ["en", "fr"],
        "creator": { "@id": authorId }
      },
      {
        "@type": page.schemaType || "WebPage",
        "@id": webpageId,
        "url": data.url,
        "name": data.title,
        "headline": data.title,
        "description": data.description,
        "isPartOf": { "@id": websiteId },
        "creator": { "@id": authorId },
        "author": { "@id": authorId },
        "inLanguage": data.lang,
        "primaryImageOfPage": {
          "@type": "ImageObject",
          "url": data.image,
          "caption": data.imageAlt
        },
        "about": (page.about || [
          "Blockchain clearing",
          "Multilateral netting",
          "Clearing and settlement",
          "Enterprise payments",
          "Financial infrastructure",
          "ISO 20022",
          "Distributed ledger technology"
        ]).map(function(name){
          return { "@type": "Thing", "name": name };
        })
      }
    ];

    if(page.datePublished){ graph[2].datePublished = page.datePublished; }
    if(page.dateModified){ graph[2].dateModified = page.dateModified; }
    if(page.doi){ graph[2].sameAs = [page.doi]; }
    if(Array.isArray(page.sameAs)){
      graph[2].sameAs = (graph[2].sameAs || []).concat(page.sameAs);
    }

    return {
      "@context": "https://schema.org",
      "@graph": graph
    };
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

    const url = cleanUrl(
      page.canonical ||
      canonicalUrl()
    );

    const image =
      page.image || defaults.ogImage;

    const imageAlt =
      page.imageAlt || defaults.ogImageAlt;

    const lang = languageFromPage(page);
    const locale = localeFromLanguage(lang, page);

    document.title = title;

    upsertMeta("description", description);

    // Kept for compatibility with secondary search engines and internal tooling.
    // Google does not use the keywords meta tag as a ranking signal.
    upsertMeta("keywords", keywords);

    upsertMeta("robots", page.robots || "index,follow,max-image-preview:large,max-snippet:-1,max-video-preview:-1");
    upsertMeta("googlebot", page.googlebot || "index,follow,max-image-preview:large,max-snippet:-1,max-video-preview:-1");
    upsertMeta("author", page.author || "Denis Bouzon");

    ensureCanonical(url);
    applyAlternates(page);

    upsertProperty("og:title", title);
    upsertProperty("og:description", description);
    upsertProperty("og:type", page.type || defaults.type);
    upsertProperty("og:site_name", defaults.siteName);
    upsertProperty("og:url", url);
    upsertProperty("og:locale", locale);
    upsertProperty("og:image", image);
    upsertProperty("og:image:secure_url", image);
    upsertProperty("og:image:alt", imageAlt);

    if(page.localeAlternate){
      const alternates = Array.isArray(page.localeAlternate) ? page.localeAlternate : [page.localeAlternate];
      alternates.forEach(function(item){
        upsertProperty("og:locale:alternate", item);
      });
    }

    upsertMeta("twitter:card", page.twitterCard || defaults.twitterCard);
    upsertMeta("twitter:title", title);
    upsertMeta("twitter:description", description);
    upsertMeta("twitter:image", image);
    upsertMeta("twitter:image:alt", imageAlt);

    upsertJsonLd(
      "osnias-seo-jsonld",
      buildStructuredData(page, {
        title: title,
        description: description,
        url: url,
        image: image,
        imageAlt: imageAlt,
        lang: lang
      })
    );
  }

  if(document.readyState === "loading"){
    document.addEventListener("DOMContentLoaded", applySEO, { once: true });
  }else{
    applySEO();
  }

})();
