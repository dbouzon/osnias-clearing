(function(){
  "use strict";

  function norm(path){
    return (path || "/").replace(/\/index\.html$/,"/").replace(/\/+$/,"/");
  }

  function markActive(){
    const current = norm(window.location.pathname);

    document.querySelectorAll(".site-nav-buttons a[href]").forEach(function(link){
      try{
        const target = norm(
          new URL(link.href, window.location.origin).pathname
        );

        const dir = target.replace(/[^/]+$/,"");

        if(
          current === target ||
          (
            dir !== "/osnias-clearing/" &&
            current.startsWith(dir)
          )
        ){
          link.setAttribute("aria-current","page");
        }else{
          link.removeAttribute("aria-current");
        }

      }catch(e){}
    });
  }

  function setupMenu(){
    const toggle = document.getElementById("menuToggle");
    const nav = document.getElementById("mainNav");

    if(!toggle || !nav) return;

    toggle.addEventListener("click",function(){
      const open = nav.classList.toggle("open");

      toggle.setAttribute(
        "aria-expanded",
        open ? "true" : "false"
      );

      toggle.textContent = open ? "Close" : "Menu";
    });

    nav.querySelectorAll("a").forEach(function(link){

      link.addEventListener("click",function(){

        nav.classList.remove("open");

        toggle.setAttribute(
          "aria-expanded",
          "false"
        );

        toggle.textContent = "Menu";
      });

    });
  }

  function secureBlankLinks(){

    document
      .querySelectorAll('a[target="_blank"]')
      .forEach(function(link){

        const rel = new Set(
          (link.getAttribute("rel") || "")
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

  document.addEventListener(
    "DOMContentLoaded",
    function(){
      markActive();
      setupMenu();
      secureBlankLinks();
    }
  );

})();
