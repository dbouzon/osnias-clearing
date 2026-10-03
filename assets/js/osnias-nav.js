
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

  function buildMainNavigation(){
    const nav = document.querySelector(".site-nav-buttons");

    if(!nav) return;

    const prefix = getPrefix();

    const items = [
      { label: "1-Osnias-ID", href: prefix + "/osniasid/" },
      { label: "2-Architecture", href: prefix + "/architecture/" },
      { label: "3-Rules", href: prefix + "/rules/" },
      { label: "4-Cycles", href: prefix + "/cycle/" },
      { label: "5-Security", href: prefix + "/security/" },
      { label: "6-Onboarding", href: prefix + "/onboarding/" },
      { label: "7-POP Osnias", href: prefix + "/pop/" },
      { label: "A-Regulation", href: prefix + "/regulation/" },
      { label: "B-Official Deployment", href: prefix + "/deployment/" },
      { label: "D-Partnership", href: prefix + "/partnership/" },
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
      notice.textContent =
        "UNDER DEVELOPMENT · NO TOKEN SALE";
    }

    notice.style.position = "static";
    notice.style.transform = "none";
    notice.style.margin = "0";
    notice.style.whiteSpace = "nowrap";
    notice.style.justifySelf = "center";
    notice.style.textAlign = "center";

    topLine.appendChild(notice);
  }

  function addSitemapLastmod(){

    if(
      document.querySelector(
        ".site-nav-lastmod"
      )
    ) return;

    const topLine = ensureHeaderTopline();

    if(!topLine) return;

    const prefix = getPrefix();
    const sitemapUrl = prefix + "/sitemap.xml";

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

        if(xml.querySelector("parsererror")){
          throw new Error("Invalid sitemap.xml");
        }

        const siteRoot =
          window.location.origin + prefix + "/";

        let lastmod = "";

        xml.querySelectorAll("url").forEach(function(entry){
          const loc = entry.querySelector("loc");
          const mod = entry.querySelector("lastmod");

          if(
            !lastmod &&
            loc &&
            mod &&
            loc.textContent.trim() === siteRoot
          ){
            lastmod = mod.textContent.trim();
          }
        });

        if(!lastmod){
          const dates = Array.from(
            xml.querySelectorAll("lastmod")
          )
            .map(function(node){
              return node.textContent.trim();
            })
            .filter(Boolean)
            .sort();

          lastmod = dates.length
            ? dates[dates.length - 1]
            : "";
        }

        if(!lastmod) return;

        const stamp = document.createElement("span");
        stamp.className = "site-nav-lastmod";
        stamp.textContent = "LAST UPDATE · " + lastmod;
        stamp.setAttribute(
          "title",
          "Date read automatically from sitemap.xml"
        );

        stamp.style.whiteSpace = "nowrap";
        stamp.style.fontSize = "14px";
        stamp.style.letterSpacing = "1px";
        stamp.style.opacity = "1";
        stamp.style.fontWeight = "700";
        stamp.style.color = "#ffffff";
        stamp.style.textTransform = "uppercase";
        stamp.style.marginLeft = "0";
        stamp.style.justifySelf = "end";
        stamp.style.textAlign = "right";

        topLine.appendChild(stamp);
      })
      .catch(function(){
        /* Silent fallback: navigation remains fully functional. */
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