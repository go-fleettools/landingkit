// wasmcache's service worker: the playground's wasm and assets, kept in Cache
// Storage under the build that wrote them, so that a returning visitor starts
// without the network. `landingkit wasmcache` writes one copy of this file per
// scope, with CONFIG filled in (the files, their SHA-256, the build).
//
// The rules, each of them a refusal:
//  - an ALLOWLIST: only a GET for a path the build listed is ever answered
//    here. Everything else (an /api/ call, a POST, another origin, a request
//    with an Authorization header, a Range request) is not touched: no
//    respondWith, so it is the browser's own request and nothing is stored.
//  - what is stored is what the build wrote: each file is fetched without
//    credentials, must be a plain 200 that was not redirected, and must hash to
//    the manifest's SHA-256, or the install fails and the old build keeps
//    serving. The hash is checked again at every read: an entry that no longer
//    matches (truncated, evicted in part, tampered with) is deleted and the
//    network answers instead.
//  - a file whose SHA-256 and size are the same in the new build is copied from
//    the old build's cache (hash checked again), not downloaded again; lazy
//    files the old build had stored are copied too, never fetched at install.
//  - a new build installs beside the old and WAITS: the page says so and
//    switches when asked ("activate"), so a page never runs with files of two
//    builds. The very first worker takes over at once.
//  - the old build's cache is deleted when the new one activates.
"use strict";

const CONFIG = /*WASMCACHE_CONFIG*/{};
// "|" cannot be in a name (Generate refuses it), so one playground's prefix is
// never the start of another's, and this worker only ever opens, reads or
// deletes caches that begin with ITS name: two playgrounds on one origin
// share the code, never an entry.
const CACHE = CONFIG.name + "|" + CONFIG.build;
const FILES = new Map(CONFIG.files.map((f) => [f.path, f]));
const HEX = (buf) => Array.from(new Uint8Array(buf), (b) => b.toString(16).padStart(2, "0")).join("");
const sha256 = async (buf) => HEX(await crypto.subtle.digest("SHA-256", buf));
// Headers that describe the bytes on the wire, not the ones stored.
const DROP = ["content-encoding", "content-length", "transfer-encoding"];

function stored(buf, headers) {
  const h = new Headers();
  headers.forEach((v, k) => { if (!DROP.includes(k.toLowerCase())) h.set(k, v); });
  return new Response(buf, { status: 200, headers: h });
}

// fetchVerified: the file as the build wrote it, or null. The HTTP cache may
// hold an older copy (the page has just downloaded this one), so it is asked
// first, and the network only when the bytes do not match.
async function fetchVerified(path) {
  const want = FILES.get(path);
  for (const cache of ["default", "reload"]) {
    let res;
    try {
      res = await fetch(path, { cache, credentials: "omit", redirect: "error" });
    } catch (e) {
      return null;
    }
    if (!res.ok || res.status !== 200 || res.redirected || res.type !== "basic") {
      return null;
    }
    const buf = await res.arrayBuffer();
    if (buf.byteLength === want.size && (await sha256(buf)) === want.sha256) {
      return stored(buf, res.headers);
    }
  }
  return null;
}

// reuse: this file as an EARLIER build of this playground stored it, when its
// bytes are exactly the ones this build lists (same size and SHA-256), or null.
// A deploy that changed one file then downloads that file, not the wasm again.
// The hash is checked here, on every candidate: an entry that does not match
// (another version, truncated, tampered with) is not reused, and neither is
// one that cannot be read; the caller fetches it instead. Only caches that
// begin with this worker's name are opened.
async function reuse(path) {
  const want = FILES.get(path);
  try {
    for (const k of await caches.keys()) {
      if (!k.startsWith(CONFIG.name + "|") || k === CACHE) {
        continue;
      }
      const hit = await (await caches.open(k)).match(path);
      if (!hit) {
        continue;
      }
      const buf = await hit.arrayBuffer();
      if (buf.byteLength === want.size && (await sha256(buf)) === want.sha256) {
        return stored(buf, hit.headers);
      }
    }
  } catch (e) {
    // a cache deleted or unreadable meanwhile: the network answers instead
  }
  return null;
}

self.addEventListener("install", (e) => {
  e.waitUntil((async () => {
    const cache = await caches.open(CACHE);
    try {
      // lazy files are never fetched here (they wait for their first request,
      // see the fetch handler); one an earlier build already stored is copied
      const todo = CONFIG.files.map((f) => f.path);
      // two at a time: a wasm is tens of megabytes in memory while it is hashed
      await Promise.all([0, 1].map(async () => {
        for (let p; (p = todo.pop()) !== undefined;) {
          if (await cache.match(p)) {
            continue; // an earlier, interrupted install of this build
          }
          let res = await reuse(p);
          if (!res && !FILES.get(p).lazy) {
            res = await fetchVerified(p);
            if (!res) {
              throw new Error(p + ": not the file the build wrote");
            }
          }
          if (res) {
            await cache.put(p, res);
          }
        }
      }));
    } catch (err) {
      await caches.delete(CACHE); // never a half-written build
      throw err;
    }
    if (!self.registration.active) {
      await self.skipWaiting();
    }
  })());
});

self.addEventListener("activate", (e) => {
  e.waitUntil((async () => {
    for (const k of await caches.keys()) {
      if (k.startsWith(CONFIG.name + "|") && k !== CACHE) {
        await caches.delete(k);
      }
    }
    await self.clients.claim();
  })());
});

self.addEventListener("message", (e) => {
  if (e.data === "activate") {
    self.skipWaiting();
  } else if (e.data === "info" && e.source) {
    e.source.postMessage({ wasmcache: { name: CONFIG.name, build: CONFIG.build, version: CONFIG.version } });
  }
});

// pathOf: the manifest path this request is for, or "" (not ours).
function pathOf(req) {
  if (req.method !== "GET" || req.headers.has("authorization") || req.headers.has("range")) {
    return "";
  }
  const url = new URL(req.url);
  if (url.origin !== self.location.origin) {
    return "";
  }
  let p = url.pathname;
  if (CONFIG.exclude.some((x) => p.startsWith(x))) {
    return "";
  }
  if (p.endsWith("/")) {
    p += "index.html";
  }
  if (!FILES.has(p)) {
    return "";
  }
  // A page of another deploy asks for its own files (?v=<version>); only this
  // build's are here.
  const v = url.searchParams.get("v");
  if (v !== null && v !== CONFIG.version) {
    return "";
  }
  return p;
}

// clientSkew: true when the page or worker making this request is itself of
// another deploy (its URL carries ?v=<other>). A file it asks for WITHOUT ?v
// (a worker's importScripts("wasm_exec.js")) is that deploy's, not this
// build's: an older /playground/ worker must not hand its copy to a newer
// /fr/ page, nor the reverse.
async function clientSkew(e) {
  // a navigation has no client yet (and asking for the one it will create
  // never answers): only requests made BY a page or worker are judged
  const id = e.request.mode === "navigate" ? "" : e.clientId;
  if (!id) {
    return false;
  }
  const c = await self.clients.get(id);
  if (!c) {
    return false;
  }
  const v = new URL(c.url).searchParams.get("v");
  return v !== null && v !== CONFIG.version;
}

// verifiedNetwork: the network's answer for a listed file, checked against the
// manifest like a cache read. A file the server changed since this build was
// written is refused, not run.
async function verifiedNetwork(req, p) {
  const res = await fetch(req);
  if (!res.ok || res.status !== 200 || res.type !== "basic") {
    return res; // an error, a redirect: not bytes to run; passed on as it is
  }
  const want = FILES.get(p);
  const buf = await res.clone().arrayBuffer();
  if (buf.byteLength === want.size && (await sha256(buf)) === want.sha256) {
    if (new URL(req.url).searchParams.get("v") === null || new URL(req.url).searchParams.get("v") === CONFIG.version) {
      await (await caches.open(CACHE)).put(p, stored(buf, res.headers)); // a missing entry heals
    }
    return res;
  }
  if (req.mode === "navigate") {
    // The page itself no longer matches this build: a newer deploy is out and
    // the cached copy is gone or broken. A 502 here would leave the playground
    // dead until someone accepted an update. Instead the page answered is a
    // constant of this worker (it executes none of the server's bytes): it
    // removes this worker and this playground's caches, and reloads, so the
    // network's page, and its new worker, take over.
    // It reloads at most once: the reload carries ?wasmcache-reset=1 (a query
    // parameter, not sessionStorage, which can throw or be blocked and would
    // then loop; it is stateless and per navigation). A page that comes back
    // still failing, with the marker, shows a static message and stops.
    // "<" is escaped so no name can close the script element.
    const name = JSON.stringify(CONFIG.name + "|").replace(/</g, "\\u003c");
    return new Response(
      "<!doctype html><meta charset=utf-8><title>Updating</title><p id=m>Updating…</p><script>" +
      "(async()=>{const u=new URL(location.href);if(u.searchParams.get('wasmcache-reset')==='1'){" +
      "document.getElementById('m').textContent='This page could not be updated automatically. Please clear this site\\'s data (cookies and site data in the browser settings) and reload.';return}" +
      "try{const r=await navigator.serviceWorker.getRegistration(location.href);if(r)await r.unregister();" +
      "for(const k of await caches.keys())if(k.startsWith(" + name + "))await caches.delete(k)}catch(e){}" +
      "u.searchParams.set('wasmcache-reset','1');location.replace(u.href)})()" +
      "</script>",
      { status: 200, headers: { "content-type": "text/html; charset=utf-8", "cache-control": "no-store" } });
  }
  return new Response("wasmcache: " + p + " from the network is not the file this build was written with (hash mismatch); refused", {
    status: 502, statusText: "wasmcache hash mismatch", headers: { "content-type": "text/plain" },
  });
}

self.addEventListener("fetch", (e) => {
  const p = pathOf(e.request);
  if (p === "") {
    return;
  }
  e.respondWith((async () => {
    if (new URL(e.request.url).searchParams.get("v") === null && (await clientSkew(e))) {
      return fetch(e.request); // another deploy's request: not ours to answer
    }
    const cache = await caches.open(CACHE);
    const hit = await cache.match(p);
    if (hit) {
      const buf = await hit.arrayBuffer();
      const want = FILES.get(p);
      if (buf.byteLength === want.size && (await sha256(buf)) === want.sha256) {
        return stored(buf, hit.headers);
      }
      await cache.delete(p); // broken: the network answers, and the entry is rewritten
      e.waitUntil(fetchVerified(p).then((r) => r && cache.put(p, r)));
    }
    if (FILES.get(p).lazy) {
      // first use of a lazy file: verified, stored for the next visit, served
      const res = await fetchVerified(p);
      if (res) {
        await cache.put(p, res.clone());
        return res;
      }
    }
    return verifiedNetwork(e.request, p);
  })());
});
