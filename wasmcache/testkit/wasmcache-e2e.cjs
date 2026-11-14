// wasmcache's browser tests: what a playground checks in its own CI that its
// offline cache does what it says, in headless Chrome (puppeteer-core).
//
//   landingkit wasmcache testkit -out kit
//   CHROME=/path/to/chrome NODE_PATH=.../node_modules \
//     node kit/wasmcache-e2e.cjs --root public --page /playground/ --scope /playground/ \
//       --asset /playground/playground.wasm [--asset ...] [--api /api/git/] \
//       [--ready "globalThis.appReady"] [--prepare "<js>"] [--verify "<js>"]
//
// It serves --root itself (a loopback server that counts what it is asked
// for, answers /api/* with a cacheable response, and can announce a newer
// deploy), and checks, one after the other:
//   1. the first visit caches the build; the worker's scope is the playground's
//   2. the second load asks the server for none of the cached files
//   3. a corrupt or truncated entry is not served: the network answers, and the
//      entry is repaired
//   4. /api/* (with and without an Authorization header) is never cached
//   5. a newer deploy shows a non-blocking prompt, in 12 languages, and does not
//      reload by itself
//   6. accepting it reloads into the new build, work kept (--prepare runs
//      before the update, --verify afterwards; default: a localStorage key),
//      and the old build's cache is gone
// --ready is a JS expression true when the app is up (default: the page's
// `wasmcache.cached()` has settled). The page must call wasmcache.start().
// Prints WASMCACHE_OK {...}; exits 1 at the first failure.
"use strict";
const http = require("http");
const fs = require("fs");
const path = require("path");
const crypto = require("crypto");
const puppeteer = require("puppeteer-core");

const args = { asset: [] };
for (let i = 2; i < process.argv.length; i += 2) {
  const k = process.argv[i].replace(/^--/, "");
  if (k === "asset") args.asset.push(process.argv[i + 1]);
  else args[k] = process.argv[i + 1];
}
const root = path.resolve(args.root || "."), pagePath = args.page || "/playground/", scope = args.scope || pagePath;
const apiPrefix = args.api || "/api/", ready = args.ready || "true";
const prepare = args.prepare || "localStorage.setItem('wasmcache-kit','work')";
const verify = args.verify || "localStorage.getItem('wasmcache-kit')==='work'";
// --hooks <file.cjs>: an app's own way to make work and to find it again,
// exporting { viewport, async prepare(page), async verify(page) -> bool };
// without it --prepare and --verify are JS expressions.
const hooks = args.hooks ? require(path.resolve(args.hooks)) : {};
const fail = (m) => { console.error("FAIL " + m); process.exit(1); };
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const canon = (o) => JSON.stringify(Object.keys(o).sort().map((k) => [k, o[k]]));
const sha = (b) => crypto.createHash("sha256").update(b).digest("hex");

// the manifest, as the generated worker holds it
const swFile = path.join(root, scope, "wasmcache-sw.js");
const swText = fs.readFileSync(swFile, "utf8");
const CONF = /const CONFIG = (\{.*\});/.exec(swText);
if (!CONF) fail(swFile + " holds no manifest: was `landingkit wasmcache` run on " + root + "?");
const manifest = JSON.parse(CONF[1]);
const assets = args.asset.length ? args.asset : [manifest.files[0].path];
for (const a of assets) if (!manifest.files.some((f) => f.path === a)) fail(a + " is not in the manifest");

// the server
let hits = {}, bumped = 0, apiN = 0;
const server = http.createServer((req, res) => {
  const u = new URL(req.url, "http://x");
  hits[u.pathname] = (hits[u.pathname] || 0) + 1;
  if (u.pathname.startsWith(apiPrefix)) {
    apiN++;
    res.writeHead(200, { "content-type": "application/json", "cache-control": "private, no-store" }); // the browser's own cache must not answer: only a worker could
    return res.end(JSON.stringify({ n: apiN, auth: req.headers.authorization || null }));
  }
  let p = u.pathname.endsWith("/") ? u.pathname + "index.html" : u.pathname;
  let body;
  try { body = fs.readFileSync(path.join(root, path.normalize(p))); } catch (e) { res.writeHead(404); return res.end("no"); }
  if (bumped && p.endsWith("/wasmcache.json")) {
    body = Buffer.from(JSON.stringify({ name: manifest.name, build: "bumped" + bumped, version: "bumped" + bumped }));
  } else if (bumped && p === scope + "wasmcache-sw.js") {
    const m = Object.assign({}, manifest, { build: "bumped" + bumped, version: "bumped" + bumped });
    body = Buffer.from(swText.replace(CONF[1], JSON.stringify(m)));
  }
  const types = { ".html": "text/html", ".js": "text/javascript", ".json": "application/json", ".wasm": "application/wasm", ".css": "text/css" };
  res.writeHead(200, { "content-type": types[path.extname(p)] || "application/octet-stream", "cache-control": "no-cache" });
  res.end(body);
});

const out = {};
(async () => {
  await new Promise((r) => server.listen(0, "127.0.0.1", r));
  const base = "http://127.0.0.1:" + server.address().port;
  const browser = await puppeteer.launch({ executablePath: process.env.CHROME, headless: true, args: ["--no-sandbox"] });
  const ctx = await browser.createBrowserContext();
  const open = async () => {
    const page = await ctx.newPage();
    if (hooks.viewport) await page.setViewport(hooks.viewport);
    page.on("pageerror", (e) => fail("page error: " + e));
    page.on("console", (m) => { if (/^wasmcache:/.test(m.text())) console.error(new Date().toISOString() + " " + m.text()); });
    await page.goto(base + pagePath, { waitUntil: "load" });
    await page.waitForFunction(ready, { timeout: 90000 });
    await page.waitForFunction("globalThis.wasmcache && wasmcache.state() !== 'off'", { timeout: 90000 }); // the page called start()
    return page;
  };
  const cacheName = () => manifest.name + "|" + manifest.build;
  const cacheHash = (page, p) => page.evaluate(async (n, p) => {
    const r = await (await caches.open(n)).match(p);
    if (!r) return null;
    const b = await r.arrayBuffer();
    return Array.from(new Uint8Array(await crypto.subtle.digest("SHA-256", b)), (x) => x.toString(16).padStart(2, "0")).join("");
  }, cacheName(), p);

  // 1. the first visit
  let page = await open();
  const cached = await page.evaluate(() => wasmcache.cached());
  if (cached !== true) fail("the first visit did not cache the build (wasmcache.cached() = " + cached + ")");
  const keys = await page.evaluate(async (n) => { const c = await caches.open(n); return (await c.keys()).map((r) => new URL(r.url).pathname); }, cacheName());
  const eager = manifest.files.filter((f) => !f.lazy), lazy = manifest.files.filter((f) => f.lazy);
  for (const f of eager) if (!keys.includes(f.path)) fail(f.path + " is not in the cache after the first visit");
  for (const f of lazy) if (keys.includes(f.path)) fail(f.path + " is lazy and was cached before anyone asked for it");
  const scopeSeen = await page.evaluate(async () => (await navigator.serviceWorker.getRegistration()).scope);
  if (scopeSeen !== base + scope) fail("the worker's scope is " + scopeSeen + ", not " + base + scope);
  out.firstVisit = keys.length + " files cached, scope " + scope;
  // --neighbour <page>: another playground on this origin, with its own worker,
  // name and files; it installs its cache now and must be left alone by this one
  let neighbour = null;
  if (args.neighbour) {
    const nScope = args.neighbour, nm = JSON.parse(/const CONFIG = (\{.*\});/.exec(fs.readFileSync(path.join(root, nScope, "wasmcache-sw.js"), "utf8"))[1]);
    if (nm.name === manifest.name) fail("the neighbour has the same cache name: " + nm.name);
    const np = await ctx.newPage();
    await np.goto(base + nScope, { waitUntil: "load" });
    await np.waitForFunction("globalThis.wasmcache && wasmcache.state() !== 'off'", { timeout: 90000 });
    if ((await np.evaluate(() => wasmcache.cached())) !== true) fail("the neighbour did not cache");
    const nCache = nm.name + "|" + nm.build;
    const entries = {};
    for (const f of nm.files.filter((f) => !f.lazy)) entries[f.path] = f.sha256;
    neighbour = { cache: nCache, entries, files: nm.files };
    const own = await np.evaluate(async (n) => { const c = await caches.open(n); const o = {}; for (const r of await c.keys()) { const b = await (await c.match(r)).arrayBuffer(); o[new URL(r.url).pathname] = Array.from(new Uint8Array(await crypto.subtle.digest("SHA-256", b)), (x) => x.toString(16).padStart(2, "0")).join(""); } return o; }, nCache);
    if (canon(own) !== canon(entries)) fail("the neighbour's cache is not what its manifest lists");
    await np.close();
  }
  await page.close();

  // 2. the second load asks the server for none of them
  hits = {};
  page = await open();
  // --network <paths>: files this page's own worker is not the one that
  // fetches (a Web Worker script is controlled by the worker whose scope holds
  // the SCRIPT's URL: /fr/playground/ loads /playground/worker.js, which the
  // /playground/ worker controls, not this one)
  const viaOther = (args.network || "").split(",").filter(Boolean);
  const asked = eager.filter((f) => hits[f.path] && !viaOther.includes(f.path));
  if (asked.length) fail("the second load asked the server for " + asked.map((f) => f.path).join(", "));
  const viaSW = await page.evaluate((a) => fetch(a).then((r) => r.status), assets[0]);
  if (viaSW !== 200 || hits[assets[0]]) fail("a cached asset reached the network");
  out.secondLoad = "0 requests for the " + eager.length + " cached files";
  // a lazy file: fetched (and stored) on first use, from the cache after
  for (const f of lazy) {
    const body = await page.evaluate((a) => fetch(a).then((r) => r.arrayBuffer()).then((b) => b.byteLength), f.path);
    if (body !== f.size) fail(f.path + " came back with " + body + " bytes");
    for (let i = 0; i < 100 && (await cacheHash(page, f.path)) !== f.sha256; i++) await sleep(100);
    if ((await cacheHash(page, f.path)) !== f.sha256) fail(f.path + " was not stored on first use");
    hits = {};
    await page.evaluate((a) => fetch(a), f.path);
    if (hits[f.path]) fail(f.path + " reached the network after first use");
  }
  if (lazy.length) out.lazyFiles = lazy.length + " lazy file(s) stored on first use, then served without the network";
  await page.close();

  // 3. a corrupt entry, then a truncated one: the network answers, the entry is repaired
  const a0 = assets[0], want = manifest.files.find((f) => f.path === a0);
  for (const [label, make] of [["corrupt", () => "garbage"], ["truncated", (b) => b.slice(0, b.length >> 1)]]) {
    page = await open();
    await page.evaluate(async (n, p, mode) => {
      const c = await caches.open(n);
      const r = await c.match(p), b = new Uint8Array(await r.arrayBuffer());
      await c.put(p, new Response(mode === "corrupt" ? "garbage" : b.slice(0, b.length >> 1), { headers: r.headers }));
    }, cacheName(), a0, label);
    await page.close();
    hits = {};
    page = await open();
    const got = await page.evaluate((a) => fetch(a).then((r) => r.arrayBuffer()).then((b) => b.byteLength), a0);
    if (got !== want.size) fail("a " + label + " entry was served: " + got + " bytes, not " + want.size);
    if (!hits[a0]) fail("a " + label + " entry did not fall back to the network");
    const got2 = await page.evaluate(async (a) => { const b = await (await fetch(a)).arrayBuffer(); return Array.from(new Uint8Array(await crypto.subtle.digest("SHA-256", b)), (x) => x.toString(16).padStart(2, "0")).join(""); }, a0);
    if (got2 !== want.sha256) fail("the bytes after a " + label + " entry are not the build's");
    for (let i = 0; i < 100 && (await cacheHash(page, a0)) !== want.sha256; i++) await sleep(100);
    if ((await cacheHash(page, a0)) !== want.sha256) fail("the " + label + " entry was not repaired");
    await page.close();
  }
  out.corruptEntries = "corrupt and truncated entries fall back to the network and are repaired";

  // 4. /api/* is never cached
  hits = {};
  page = await open();
  const probe = apiPrefix + "probe";
  const r1 = await page.evaluate((u) => fetch(u).then((r) => r.json()), probe);
  const r2 = await page.evaluate((u) => fetch(u).then((r) => r.json()), probe);
  const r3 = await page.evaluate((u) => fetch(u, { headers: { Authorization: "Bearer secret" } }).then((r) => r.json()), probe);
  const r4 = await page.evaluate((u) => fetch(u, { method: "POST", body: "x" }).then((r) => r.status), probe);
  if (hits[probe] !== 4) fail(apiPrefix + " reached the server " + hits[probe] + " times for 4 requests: something answered one");
  if (r1.n === r2.n || r3.auth !== "Bearer secret" || r4 !== 200) fail(apiPrefix + " answers were not the server's own");
  const stored = await page.evaluate(async () => { const o = []; for (const k of await caches.keys()) for (const r of await (await caches.open(k)).keys()) o.push(new URL(r.url).pathname); return o; });
  const leaked = stored.filter((p) => p.startsWith(apiPrefix));
  if (leaked.length) fail("cached: " + leaked.join(", "));
  // a file of the neighbouring playground is not this one's to serve: it goes to the network
  if (neighbour) {
    const np = Object.keys(neighbour.entries).find((p) => !manifest.files.some((f) => f.path === p));
    hits = {};
    await page.evaluate((u) => fetch(u), np);
    if (!hits[np]) fail("this playground served the neighbour's " + np + " from a cache");
  }
  const unlisted = stored.filter((p) => !manifest.files.some((f) => f.path === p) && !(neighbour && p in neighbour.entries));
  if (unlisted.length) fail("the cache holds files the manifest does not list: " + unlisted.join(", "));
  out.api = apiPrefix + " reached the server every time, with and without Authorization; the cache holds only the manifest's files";

  // 5. a newer deploy: a prompt, no reload
  if (hooks.prepare) await hooks.prepare(page); else await page.evaluate(prepare);
  await page.evaluate(() => { window.__kitMarker = "still here"; });
  const langs = await page.evaluate(() => Object.keys(wasmcache.words()));
  if (langs.length !== 12) fail("the prompt speaks " + langs.length + " languages, not 12");
  for (const l of langs) {
    const w = await page.evaluate((l) => wasmcache.words(l), l);
    if (!w.update || !w.reload || !w.later) fail("the prompt lacks words in " + l);
  }
  bumped = 1;
  const newer = await page.evaluate(() => wasmcache.check());
  if (newer !== true) fail("a newer deploy was not detected");
  await page.waitForSelector("#wasmcache-update", { timeout: 20000 });
  const box = await page.evaluate(() => {
    const e = document.getElementById("wasmcache-update"), r = e.getBoundingClientRect(), s = getComputedStyle(e);
    const mid = document.elementFromPoint(innerWidth / 2, innerHeight / 2);
    return { role: e.getAttribute("role"), pos: s.position, share: (r.width * r.height) / (innerWidth * innerHeight), midCovered: e.contains(mid), marker: window.__kitMarker, text: e.textContent };
  });
  if (box.role !== "status" || box.pos !== "fixed" || box.share > 0.15 || box.midCovered) fail("the prompt is blocking: " + JSON.stringify(box));
  if (box.marker !== "still here") fail("the page reloaded by itself");
  const regState = () => page.evaluate(async () => {
    const r = await navigator.serviceWorker.getRegistration();
    const s = (w) => (w ? w.state : null);
    return r ? { active: s(r.active), waiting: s(r.waiting), installing: s(r.installing), scope: r.scope } : null;
  });
  console.error("before accepting: " + JSON.stringify(await regState()));
  out.prompt = "non-blocking prompt shown for a newer deploy: " + JSON.stringify(box.text.slice(0, 60)) + "; page not reloaded";

  // 6. accept: reload into the new build, work kept, old cache gone
  await Promise.all([page.waitForNavigation({ waitUntil: "load", timeout: 240000 }), page.click("#wasmcache-reload")]); // Reload waits for the new build to install
  await page.waitForFunction(ready, { timeout: 90000 });
  await page.waitForFunction("globalThis.wasmcache && wasmcache.state() !== 'off'", { timeout: 90000 });
  if (!(hooks.verify ? await hooks.verify(page) : await page.evaluate(verify))) fail("the work was not kept over the update");
  // asked until the new worker answers: a slow machine may still be handing
  // the page over (the reload waits for it, but the kit does not assume)
  let info = null;
  for (let i = 0; i < 60; i++) {
    info = await page.evaluate(() => new Promise((res) => {
      const c = navigator.serviceWorker.controller;
      if (!c) return res(null);
      navigator.serviceWorker.addEventListener("message", (e) => res(e.data.wasmcache), { once: true });
      c.postMessage("info");
    }));
    if (info && info.version === "bumped1") break;
    await sleep(500);
  }
  if (!info) fail("no worker controls the page after accepting");
  if (info.version !== "bumped1") fail("the page runs " + info.version + " after accepting, not the new build; registration " + JSON.stringify(await regState()) + ", caches " + JSON.stringify(await page.evaluate(() => caches.keys())));
  const names = await page.evaluate(() => caches.keys());
  const mine = names.filter((n) => n.startsWith(manifest.name + "|"));
  if (mine.length !== 1 || mine[0] !== manifest.name + "|bumped1") fail("old caches remain: " + names.join(", "));
  if (neighbour) {
    // the other playground's cache is exactly as it was: not read, not deleted, not rewritten
    const theirs = await page.evaluate(async (n) => {
      const c = await caches.open(n), o = {};
      for (const r of await c.keys()) {
        const b = await (await c.match(r)).arrayBuffer();
        o[new URL(r.url).pathname] = Array.from(new Uint8Array(await crypto.subtle.digest("SHA-256", b)), (x) => x.toString(16).padStart(2, "0")).join("");
      }
      return o;
    }, neighbour.cache);
    if (canon(theirs) !== canon(neighbour.entries)) fail("the neighbouring playground's cache changed: " + JSON.stringify(theirs));
    if (!names.includes(neighbour.cache)) fail("the neighbouring playground's cache is gone");
    if (names.length !== mine.length + 1) fail("unexpected caches: " + names.join(", "));
    out.neighbour = "a second playground on this origin (" + neighbour.cache + ") kept every entry untouched through this one's corruption, API traffic and update";
  }
  names.splice(0, names.length, ...mine);
  out.update = "accepted: reloaded into " + info.version + ", work kept, old cache deleted (" + names[0] + ")";
  await browser.close();
  server.close();
  console.log("WASMCACHE_OK " + JSON.stringify(out));
})().catch((e) => fail(e && e.stack || String(e)));
