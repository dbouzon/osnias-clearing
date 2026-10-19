(function(){
  "use strict";

  const defaults = {
    siteName: "Osnias Clearing",
    siteUrl: "https://www.osnias-clearing.com/",
    type: "website",
    email: "contact@osnias-clearing.com",
    linkedin: "https://www.linkedin.com/in/denis-bouzon-3b766a437/",
    telegram: "https://t.me/osnas_clearing",
    orcid: "https://orcid.org/0009-0007-8894-8902",
    zenodo: "https://zenodo.org/",
    ogImage: "https://www.osnias-clearing.com/index-picture.png",
    ogImageAlt: "Osnias Clearing — Multilateral Netting and Multi-Node Clearing Infrastructure",
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
    const language = String(lang).toLowerCase();
    if(language.startsWith("fr")) return "fr_FR";
    if(language.startsWith("es")) return "es_ES";
    return defaults.locale;
  }

  function applyAlternates(page){
    const alternates = page.alternates || {};
    Object.keys(alternates).forEach(function(lang){
      if(!document.querySelector('link[rel="alternate"][hreflang="' + lang + '"]')){
        upsertLink("alternate", cleanUrl(alternates[lang]), lang);
      }
    });

    if(page.xDefault && !document.querySelector('link[rel="alternate"][hreflang="x-default"]')){
      upsertLink("alternate", cleanUrl(page.xDefault), "x-default");
    }
  }

  function buildOrganizationData(){
    return {
      "@context": "https://schema.org",
      "@type": "Organization",
      "@id": defaults.siteUrl + "#organization",
      "name": defaults.siteName,
      "url": defaults.siteUrl,
      "email": defaults.email,
      "logo": {
        "@type": "ImageObject",
        "url": defaults.ogImage
      },
      "contactPoint": {
        "@type": "ContactPoint",
        "contactType": "general inquiries",
        "email": defaults.email,
        "availableLanguage": ["fr", "en", "es"]
      }
    };
  }

  function breadcrumbLabel(slug, isLast){
    if(isLast){
      const h1 = document.querySelector("h1");
      if(h1 && h1.textContent.trim()) return h1.textContent.trim();
    }

    const labels = {
      osniasid: "Osnias-ID",
      architecture: "Architecture",
      rules: "Rules",
      cycle: "Clearing Cycles",
      cycles: "Clearing Cycles",
      security: "Security",
      onboarding: "Onboarding",
      regulation: "Regulation",
      deployment: "Deployment",
      roadmap: "Roadmap",
      partnership: "Partnership",
      faq: "FAQ",
      vida2030: "ViDA 2030",
      pop: "O-POP",
      oracle: "Oracle",
      escrow: "Escrow",
      node: "Node",
      sei: "Sei"
    };

    if(labels[slug]) return labels[slug];
    return slug
      .replace(/[-_]+/g, " ")
      .replace(/\b\w/g, function(letter){ return letter.toUpperCase(); });
  }

  function buildBreadcrumbData(url, lang){
    let parsed;
    try {
      parsed = new URL(url, defaults.siteUrl);
    } catch(e){
      return null;
    }

    const rawSegments = parsed.pathname.split("/").filter(Boolean);
    let languagePrefix = "";
    let segments = rawSegments.slice();

    if(segments[0] === "en" || segments[0] === "es"){
      languagePrefix = segments.shift();
    }

    if(!segments.length) return null;

    const language = String(lang || "").toLowerCase();
    const homeName = language.startsWith("fr") ? "Accueil" :
      (language.startsWith("es") ? "Inicio" : "Home");
    const homePath = languagePrefix ? "/" + languagePrefix + "/" : "/";
    const items = [
      {
        "@type": "ListItem",
        "position": 1,
        "name": homeName,
        "item": new URL(homePath, defaults.siteUrl).href
      }
    ];

    const builtSegments = [];
    segments.forEach(function(segment, index){
      builtSegments.push(segment);
      const path = (languagePrefix ? "/" + languagePrefix : "") +
        "/" + builtSegments.join("/") + "/";
      items.push({
        "@type": "ListItem",
        "position": index + 2,
        "name": breadcrumbLabel(segment, index === segments.length - 1),
        "item": new URL(path, defaults.siteUrl).href
      });
    });

    return {
      "@context": "https://schema.org",
      "@type": "BreadcrumbList",
      "@id": url + "#breadcrumb",
      "itemListElement": items
    };
  }

  function buildStructuredData(page, data){
    const authorId = defaults.siteUrl + "#denis-bouzon";
    const organizationId = defaults.siteUrl + "#organization";
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
        "inLanguage": ["en", "fr", "es"],
        "creator": { "@id": authorId },
        "publisher": { "@id": organizationId }
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
          "Multilateral netting",
          "Multi-node clearing infrastructure",
          "Net settlement positions",
          "Liquidity savings",
          "Cross-jurisdiction clearing",
          "Enterprise clearing",
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
      document.title ||
      page.title ||
      defaults.title;

    const description =
      document.querySelector('meta[name="description"]')?.getAttribute("content") ||
      page.description ||
      defaults.description;

    const keywords =
      page.keywords ||
      defaults.keywords;

    const url = cleanUrl(
      canonicalUrl()
    );

    const image =
      document.querySelector('meta[property="og:image"]')?.content || page.image || defaults.ogImage;

    const imageAlt =
      page.imageAlt || defaults.ogImageAlt;

    const lang = languageFromPage(page);
    const locale = localeFromLanguage(lang, page);

    if(title){ document.title = title; }

    upsertMeta("description", description);

    // Kept for compatibility with secondary search engines and internal tooling.
    // Google does not use the keywords meta tag as a ranking signal.
    upsertMeta("keywords", keywords);

    // Static indexing directives are authoritative. Never opt a page into indexing.
    const robots = document.querySelector('meta[name="robots"]');
    if(!robots){ upsertMeta("robots", page.robots || "noindex,follow"); }
    if(!document.querySelector('meta[name="googlebot"]')){
      upsertMeta("googlebot", document.querySelector('meta[name="robots"]')?.content || "noindex,follow");
    }
    upsertMeta("author", page.author || "Denis Bouzon");

    // Canonical and language links are supplied by the page HTML.
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

    // Global Organization entity, independent of page-specific JSON-LD.
    upsertJsonLd("osnias-organization-jsonld", buildOrganizationData());

    // Breadcrumb structured data for each non-home page.
    const breadcrumbData = buildBreadcrumbData(url, lang);
    if(breadcrumbData){
      upsertJsonLd("osnias-breadcrumb-jsonld", breadcrumbData);
    }

    // Page-specific HTML now carries the authoritative structured data.
    // Add the generic graph only when no JSON-LD is already present.
    if(!document.querySelector('script[type="application/ld+json"]')){
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
  }

  if(document.readyState === "loading"){
    document.addEventListener("DOMContentLoaded", applySEO, { once: true });
  }else{
    applySEO();
  }

})();
