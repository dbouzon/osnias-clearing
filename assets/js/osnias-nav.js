
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
      {
        label: "Architecture",
        href: prefix + "/architecture/architecture.html"
      },
      {
        label: "Official Deployment",
        href: prefix + "/deployment/deployment.html"
      },
      {
        label: "Security",
        href: prefix + "/security/security.html"
      },
      {
        label: "Regulation",
        href: prefix + "/regulation/regulation.html"
      },
      {
        label: "Documentation",
        href: prefix + "/documentation/documentation.html"
      }
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

  function addDevNotice(){

    if(
      document.querySelector(
        ".site-nav-notice"
      )
    ) return;

    const brand =
      document.querySelector(
        ".site-nav-brand"
      );

    if(!brand) return;

    const notice =
      document.createElement("span");

    notice.className =
      "site-nav-notice";

    notice.textContent =
      "UNDER DEVELOPMENT · NO TOKEN SALE";

    brand.insertAdjacentElement(
      "afterend",
      notice
    );
  }

  function init(){

    buildMainNavigation();
    markActive();
    secureBlankLinks();
    addDevNotice();

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