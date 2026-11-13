// wasmcache's page side: registers the service worker, watches for a newer
// deploy and says so, without ever reloading the page by itself.
//
//   <script src="/playground/wasmcache-client.js"></script>
//   ...once the app is ready (the download is over, nothing is in the way):
//   wasmcache.start({ lang: "fr", beforeReload: () => app.saveNow() });
//
// A missing script (a site built without the cache step) leaves `wasmcache`
// undefined: guard the call with `if (globalThis.wasmcache)`.
//
// Options (all optional):
//   scope         the service worker's scope, "/playground/" ending in a slash
//                 (default: the directory of the page's own URL)
//   home          where wasmcache.json and the worker live (default: the
//                 directory this script was served from)
//   lang          the page's language, for the prompt (default: <html lang>)
//   strings       {update, reload, later} replacing the built-in words
//   version       the deploy the page was built for, when it knows it; else
//                 the worker's, else the one answered at start
//   beforeReload  function returning a promise: save the user's work here;
//                 the reload waits for it (at most five seconds)
//   intervalMs    how often to ask for a newer deploy (default ten minutes,
//                 0 = only at start and when the tab becomes visible again)
//   fetch         the fetch function, for the version file (tests)
//
// wasmcache.check()   ask now; resolves true when a newer deploy is out
// wasmcache.state()   "off" | "current" | "update"
// wasmcache.cached()  resolves once this build is in the cache (false: no cache)
// wasmcache.reload()  what the prompt's button does
(function () {
  "use strict";
  var WORDS = {
    en: { update: "A new version is available.", reload: "Reload", later: "Later" },
    fr: { update: "Une nouvelle version est disponible.", reload: "Recharger", later: "Plus tard" },
    de: { update: "Eine neue Version ist verfügbar.", reload: "Neu laden", later: "Später" },
    es: { update: "Hay una nueva versión disponible.", reload: "Recargar", later: "Más tarde" },
    it: { update: "È disponibile una nuova versione.", reload: "Ricarica", later: "Più tardi" },
    pt: { update: "Há uma nova versão disponível.", reload: "Recarregar", later: "Mais tarde" },
    pl: { update: "Dostępna jest nowa wersja.", reload: "Odśwież", later: "Później" },
    ru: { update: "Доступна новая версия.", reload: "Перезагрузить", later: "Позже" },
    uk: { update: "Доступна нова версія.", reload: "Перезавантажити", later: "Пізніше" },
    zh: { update: "有新版本可用。", reload: "重新加载", later: "稍后" },
    ja: { update: "新しいバージョンがあります。", reload: "再読み込み", later: "後で" },
    ko: { update: "새 버전이 있습니다.", reload: "새로 고침", later: "나중에" }
  };
  var here = document.currentScript && document.currentScript.src ? new URL(".", document.currentScript.src).pathname : "/";
  var o = {}, st = "off", reg = null, running = "", shown = "", node = null, cname = "";
  var cachedP = Promise.resolve(false);

  function words() {
    var l = String(o.lang || document.documentElement.lang || "en").toLowerCase().split("-")[0];
    var w = WORDS[l] || WORDS.en, s = o.strings || {};
    return { update: s.update || w.update, reload: s.reload || w.reload, later: s.later || w.later };
  }

  function ask(msg) {
    return new Promise(function (resolve) {
      var c = navigator.serviceWorker && navigator.serviceWorker.controller;
      if (!c) { resolve(null); return; }
      var t = setTimeout(function () { resolve(null); }, 1500);
      var on = function (e) {
        if (e.data && e.data.wasmcache) {
          clearTimeout(t);
          navigator.serviceWorker.removeEventListener("message", on);
          resolve(e.data.wasmcache);
        }
      };
      navigator.serviceWorker.addEventListener("message", on);
      c.postMessage(msg);
    });
  }

  function prompt() {
    if (node) { return; }
    var w = words();
    var box = document.createElement("div");
    box.id = "wasmcache-update";
    box.setAttribute("role", "status"); // polite: announced, never taking focus
    var s = box.style;
    s.position = "fixed"; s.right = "16px"; s.bottom = "16px"; s.zIndex = "2147483000";
    s.maxWidth = "min(420px, calc(100vw - 32px))"; s.display = "flex"; s.gap = "10px"; s.alignItems = "center";
    s.flexWrap = "wrap"; s.padding = "10px 14px"; s.borderRadius = "10px"; s.font = "14px/1.4 system-ui, sans-serif";
    s.background = "var(--bg-soft, #f4f6f8)"; s.color = "var(--ink, #14181c)";
    s.border = "1px solid var(--line, #b8c0c8)"; s.boxShadow = "0 6px 24px rgba(0,0,0,.2)";
    var text = document.createElement("span");
    text.textContent = w.update;
    var go = document.createElement("button");
    go.type = "button"; go.id = "wasmcache-reload"; go.textContent = w.reload;
    var no = document.createElement("button");
    no.type = "button"; no.id = "wasmcache-later"; no.textContent = w.later;
    [go, no].forEach(function (b) {
      b.style.font = "inherit"; b.style.padding = "4px 12px"; b.style.borderRadius = "8px"; b.style.cursor = "pointer";
      b.style.border = "1px solid var(--line, #b8c0c8)"; b.style.background = "transparent"; b.style.color = "inherit";
    });
    go.style.background = "var(--accent, #0079a8)"; go.style.color = "#fff"; go.style.borderColor = "transparent";
    go.addEventListener("click", function () { go.disabled = true; api.reload(); });
    no.addEventListener("click", function () { box.remove(); node = null; });
    box.append(text, go, no);
    document.body.appendChild(box);
    node = box;
  }

  // The new worker, installed and waiting (or null: it could not install).
  function waitingWorker() {
    return new Promise(function (resolve) {
      if (!reg) { resolve(null); return; }
      var done = function (w) { resolve(w); };
      var watch = function (w) {
        if (w.state === "installed") { done(w); return; }
        w.addEventListener("statechange", function () {
          if (w.state === "installed") { done(w); }
          else if (w.state === "redundant") { done(null); }
        });
      };
      reg.update().then(function () {
        if (reg.waiting) { done(reg.waiting); }
        else if (reg.installing) { watch(reg.installing); }
        else { done(null); }
      }, function () { done(null); });
    });
  }

  var api = {
    state: function () { return st; },
    cached: function () { return cachedP; },
    // the prompt's words: all languages, or one (English for an unknown one)
    words: function (l) { return l ? (WORDS[l] || WORDS.en) : WORDS; },
    check: function () {
      var f = o.fetch || fetch;
      return f(here + "wasmcache.json", { cache: "no-store", credentials: "omit" }).then(function (r) {
        if (!r.ok) { throw new Error("HTTP " + r.status); }
        return r.json();
      }).then(function (j) {
        cname = j.name || "";
        if (!running) { running = j.version; } // first visit: the page is this deploy's
        if (j.version === running || j.version === shown) { return j.version !== running; }
        shown = j.version;
        st = "update";
        // install the new build first, so that Reload has something to switch to
        return waitingWorker().then(function () { prompt(); return true; });
      }).catch(function () { return false; }); // offline, or no version file: nothing to say
    },
    reload: function () {
      var save = Promise.resolve();
      try { if (o.beforeReload) { save = Promise.resolve(o.beforeReload()); } } catch (e) { /* reload anyway */ }
      var cap = new Promise(function (r) { setTimeout(r, 5000); });
      return Promise.race([save.catch(function () {}), cap]).then(function () {
        var w = reg && reg.waiting;
        if (w) {
          // the worker takes over, and the page follows it
          return new Promise(function (resolve) {
            navigator.serviceWorker.addEventListener("controllerchange", resolve, { once: true });
            setTimeout(resolve, 3000);
            w.postMessage("activate");
          });
        }
        // no new worker could install: the old one would serve the old build
        // again for ever, so it goes, with its cache; the network answers
        var gone = reg ? reg.unregister() : Promise.resolve();
        return gone.then(function () { return window.caches ? caches.keys() : []; }).then(function (ks) {
          return Promise.all(ks.filter(function (k) { return cname !== "" && k.indexOf(cname + "|") === 0; /* this playground's only */ }).map(function (k) { return caches.delete(k); }));
        });
      }).then(function () { location.reload(); }, function () { location.reload(); });
    },
    start: function (opts) {
      o = opts || {};
      st = "current";
      running = o.version || "";
      var scope = o.scope || new URL(".", location.href).pathname;
      var h = o.home || here;
      here = h;
      if ("serviceWorker" in navigator && window.isSecureContext) {
        cachedP = navigator.serviceWorker.register(scope + "wasmcache-sw.js", { scope: scope }).then(function (r) {
          reg = r;
          return navigator.serviceWorker.ready;
        }).then(function () {
          return ask("info").then(function (i) { if (i && !running) { running = i.version; } return true; });
        }).catch(function () { return false; });
      }
      cachedP.then(function () { return api.check(); });
      document.addEventListener("visibilitychange", function () {
        if (document.visibilityState === "visible") { api.check(); }
      });
      if (o.intervalMs !== 0) { setInterval(api.check, o.intervalMs || 600000); }
      return cachedP;
    }
  };
  window.wasmcache = api;
})();
