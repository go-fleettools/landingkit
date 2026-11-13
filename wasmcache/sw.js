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
//  - a new build installs beside the old and WAITS: the page says so and
//    switches when asked ("activate"), so a page never runs with files of two
//    builds. The very first worker takes over at once.
//  - the old build's cache is deleted when the new one activates.
"use strict";

const CONFIG = /*WASMCACHE_CONFIG*/{};
const CACHE = CONFIG.name + "-" + CONFIG.build;
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

self.addEventListener("install", (e) => {
  e.waitUntil((async () => {
    const cache = await caches.open(CACHE);
    try {
      // lazy files wait for their first request (see the fetch handler)
      const todo = CONFIG.files.filter((f) => !f.lazy).map((f) => f.path);
      // two at a time: a wasm is tens of megabytes in memory while it is hashed
      await Promise.all([0, 1].map(async () => {
        for (let p; (p = todo.pop()) !== undefined;) {
          if (await cache.match(p)) {
            continue; // an earlier, interrupted install of this build
          }
          const res = await fetchVerified(p);
          if (!res) {
            throw new Error(p + ": not the file the build wrote");
          }
          await cache.put(p, res);
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
      if (k.startsWith(CONFIG.name + "-") && k !== CACHE) {
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

self.addEventListener("fetch", (e) => {
  const p = pathOf(e.request);
  if (p === "") {
    return;
  }
  e.respondWith((async () => {
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
    return fetch(e.request);
  })());
});
