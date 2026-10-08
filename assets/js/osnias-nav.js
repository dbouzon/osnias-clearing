
  (function(){
  "use strict";

  function norm(path){
    return (path || "/")
      .replace(/\/index\.html$/,"/")
      .replace(/\/+$/,"/");
  }

  function getPrefix(){
    const path = window.location.pathname || "/";

    return path.startsWith("/osnias-clearing/")
      ? "/osnias-clearing"
      : "";
  }


  function getLocalPath(){
    const prefix = getPrefix();
    const path = window.location.pathname || "/";
    return prefix && path.startsWith(prefix)
      ? (path.slice(prefix.length) || "/")
      : path;
  }

  function isEnglishPage(){
    const path = getLocalPath();
    return path === "/en" || path.startsWith("/en/");
  }

  function buildMainNavigation(){
    const nav = document.querySelector(".site-nav-buttons");

    if(!nav) return;

    const prefix = getPrefix();

    const isEn = isEnglishPage();

    const items = isEn ? [
      { label: "1-Osnias-ID", href: prefix + "/en/osniasid/" },
      { label: "2-Architecture", href: prefix + "/en/architecture/" },
      { label: "3-Rules", href: prefix + "/en/rules/" },
      { label: "4-Cycles", href: prefix + "/en/cycle/" },
      { label: "5-Security", href: prefix + "/en/security/" },
      { label: "6-Onboarding", href: prefix + "/en/onboarding/" },
      { label: "7-POP Osnias", href: prefix + "/en/pop/" },
      { label: "A-Regulation", href: prefix + "/en/regulation/" },
      { label: "B-Official Deployment", href: prefix + "/en/deployment/" },
      { label: "C-Roadmap", href: prefix + "/en/roadmap/" },
      { label: "D-Partnership", href: prefix + "/en/partnership/" },
      { label: "E-FAQ", href: prefix + "/en/faq/" }
    ] : [
      { label: "1-Osnias-ID", href: prefix + "/osniasid/" },
      { label: "2-Architecture", href: prefix + "/architecture/" },
      { label: "3-Règles", href: prefix + "/rules/" },
      { label: "4-Cycles", href: prefix + "/cycle/" },
      { label: "5-Sécurité", href: prefix + "/security/" },
      { label: "6-Onboarding", href: prefix + "/onboarding/" },
      { label: "7-POP Osnias", href: prefix + "/pop/" },
      { label: "A-Régulation", href: prefix + "/regulation/" },
      { label: "B-Déploiement", href: prefix + "/deployment/" },
      { label: "C-Roadmap", href: prefix + "/roadmap/" },
      { label: "D-Partenariat", href: prefix + "/partnership/" },
      { label: "E-FAQ", href: prefix + "/faq/" }
    ];

    nav.replaceChildren();

    items.forEach(function(item){

      const link = document.createElement("a");

      link.href = item.href;
      link.textContent = item.label;

      nav.appendChild(link);

    });
  }

  function markActive(){
    const current = norm(window.location.pathname);

    document
      .querySelectorAll(".site-nav-buttons a[href]")
      .forEach(function(link){

        try{

          const target = norm(
            new URL(
              link.href,
              window.location.origin
            ).pathname
          );

          const dir = target.replace(/[^/]+$/,"");

          if(
            current === target ||
            (
              dir !== "/" &&
              current.startsWith(dir)
            )
          ){
            link.setAttribute(
              "aria-current",
              "page"
            );
          }else{
            link.removeAttribute(
              "aria-current"
            );
          }

        }catch(e){}

      });
  }

  function secureBlankLinks(){

    document
      .querySelectorAll(
        'a[target="_blank"]'
      )
      .forEach(function(link){

        const rel = new Set(
          (
            link.getAttribute("rel") || ""
          )
          .split(/\s+/)
          .filter(Boolean)
        );

        rel.add("noopener");
        rel.add("noreferrer");

        link.setAttribute(
          "rel",
          Array.from(rel).join(" ")
        );

      });
  }

  function ensureHeaderTopline(){

    const navInner =
      document.querySelector(
        ".site-nav-inner"
      );

    const brand =
      document.querySelector(
        ".site-nav-brand"
      );

    const nav =
      document.querySelector(
        ".site-nav-buttons"
      );

    if(!navInner || !brand) return null;

    let topLine =
      navInner.querySelector(
        ".site-nav-topline"
      );

    if(!topLine){
      topLine = document.createElement("div");
      topLine.className = "site-nav-topline";

      topLine.style.display = "grid";
      topLine.style.gridTemplateColumns = "1fr auto 1fr";
      topLine.style.alignItems = "center";
      topLine.style.columnGap = "16px";
      topLine.style.width = "100%";

      navInner.insertBefore(topLine, nav || navInner.firstChild);
      topLine.appendChild(brand);
    }

    navInner.style.display = "flex";
    navInner.style.flexDirection = "column";
    navInner.style.alignItems = "stretch";
    navInner.style.gap = "10px";

    if(nav){
      nav.style.width = "100%";
      nav.style.marginLeft = "0";
      nav.style.justifyContent = "flex-start";
    }

    brand.style.justifySelf = "start";

    return topLine;
  }


  function addDevNotice(){

    const topLine = ensureHeaderTopline();

    if(!topLine) return;

    let notice =
      document.querySelector(
        ".site-nav-notice"
      );

    if(!notice){
      notice = document.createElement("span");
      notice.className = "site-nav-notice";
      notice.textContent = isEnglishPage()
        ? "UNDER DEVELOPMENT · NO TOKEN SALE"
        : "EN DÉVELOPPEMENT · AUCUNE VENTE DE TOKEN";
    }

    notice.style.position = "static";
    notice.style.transform = "none";
    notice.style.margin = "0";
    notice.style.whiteSpace = "nowrap";
    notice.style.justifySelf = "center";
    notice.style.textAlign = "center";
    notice.style.gridColumn = "2";

    topLine.appendChild(notice);
  }

  /* ---------- Date de dernière mise à jour (lue dans sitemap.xml) ---------- */

  function injectToplineStyles(){
    if(document.getElementById("osnias-topline-style")) return;
    const style = document.createElement("style");
    style.id = "osnias-topline-style";
    style.textContent =
      "@media (max-width:900px){" +
        ".site-nav-topline{display:flex !important;flex-wrap:wrap;justify-content:space-between;row-gap:6px}" +
        ".site-nav-topline .site-nav-notice{order:3;flex-basis:100%;text-align:left !important;white-space:normal !important}" +
        ".site-nav-topline .site-nav-lastmod{order:2}" +
      "}";
    document.head.appendChild(style);
  }

  function formatLastmod(raw, isEn){
    /* Accepte "AAAA-MM-JJ" ou un horodatage ISO complet ("AAAA-MM-JJThh:mm:ss+00:00"). */
    const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(raw || "");
    if(!m) return raw || "";
    const date = new Date(Date.UTC(+m[1], +m[2] - 1, +m[3]));
    try{
      return new Intl.DateTimeFormat(isEn ? "en-GB" : "fr-FR", {
        day: "numeric", month: "long", year: "numeric", timeZone: "UTC"
      }).format(date);
    }catch(e){
      return m[3] + "/" + m[2] + "/" + m[1];
    }
  }

  function latestLastmod(xml){
    /* Plus récente <lastmod> du sitemap (insensible aux espaces de noms). */
    const nodes = xml.getElementsByTagNameNS
      ? xml.getElementsByTagNameNS("*", "lastmod")
      : xml.getElementsByTagName("lastmod");
    let best = "", bestKey = "";
    Array.prototype.forEach.call(nodes, function(node){
      const value = (node.textContent || "").trim();
      const key = value.slice(0, 10);
      if(/^\d{4}-\d{2}-\d{2}$/.test(key) && key > bestKey){
        bestKey = key;
        best = value;
      }
    });
    return best;
  }

  function addSitemapLastmod(){

    if(
      document.querySelector(
        ".site-nav-lastmod"
      )
    ) return;

    const topLine = ensureHeaderTopline();

    if(!topLine) return;

    injectToplineStyles();

    const isEn = isEnglishPage();

    /* L'emplacement est créé tout de suite (colonne de droite), le texte arrive après lecture du sitemap. */
    const stamp = document.createElement("span");
    stamp.className = "site-nav-lastmod";
    stamp.setAttribute(
      "title",
      isEn
        ? "Date read automatically from sitemap.xml"
        : "Date lue automatiquement depuis sitemap.xml"
    );
    stamp.style.gridColumn = "3";
    stamp.style.justifySelf = "end";
    stamp.style.textAlign = "right";
    stamp.style.whiteSpace = "nowrap";
    stamp.style.margin = "0";

    /* Même typographie que le bandeau « En développement ». */
    const notice = topLine.querySelector(".site-nav-notice");
    if(notice && window.getComputedStyle){
      const cs = window.getComputedStyle(notice);
      stamp.style.fontSize = cs.fontSize;
      stamp.style.fontWeight = cs.fontWeight;
      stamp.style.letterSpacing = cs.letterSpacing;
      stamp.style.color = cs.color;
      stamp.style.textTransform = cs.textTransform;
      stamp.style.fontFamily = cs.fontFamily;
    }

    topLine.appendChild(stamp);

    const sitemapUrl = getPrefix() + "/sitemap.xml";

    fetch(sitemapUrl, { cache: "no-store" })
      .then(function(response){
        if(!response.ok){
          throw new Error("Unable to load sitemap.xml");
        }
        return response.text();
      })
      .then(function(xmlText){
        const xml = new DOMParser().parseFromString(
          xmlText,
          "application/xml"
        );

        if(xml.getElementsByTagName("parsererror").length){
          throw new Error("Invalid sitemap.xml");
        }

        const raw = latestLastmod(xml);

        if(!raw){
          stamp.remove();
          return;
        }

        const label = formatLastmod(raw, isEn);
        stamp.textContent = isEn
          ? "Last updated " + label
          : "Dernière mise à jour le " + label;

        const time = document.createElement("time");
        time.setAttribute("datetime", raw.slice(0, 10));
        time.hidden = true;
        stamp.appendChild(time);
      })
      .catch(function(){
        /* Repli silencieux : la navigation reste pleinement fonctionnelle. */
        stamp.remove();
      });
  }

  function init(){

    buildMainNavigation();
    markActive();
    secureBlankLinks();
    addDevNotice();
    addSitemapLastmod();

  }

  if(
    document.readyState === "loading"
  ){
    document.addEventListener(
      "DOMContentLoaded",
      init
    );
  }else{
    init();
  }

})();