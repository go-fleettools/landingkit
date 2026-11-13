// Package wasmcache is the playgrounds' shared offline cache: a service worker
// that keeps a playground's wasm, glue, fonts and page in Cache Storage under
// the build that wrote them, a page script that notices a newer deploy and
// offers a reload, and the browser tests a playground runs in its own CI.
//
// A playground calls Generate on its BUILT site (after Hugo, or whatever
// writes its pages). Generate hashes the files the configuration names and
// writes, beside them:
//
//	<scope>wasmcache-sw.js      one worker per scope, the manifest inside it
//	<home>wasmcache-client.js   the page side (wasmcache.start(...))
//	<home>wasmcache.json        the version file the page asks for, no-store
//
// The worker only ever answers a GET for a path the manifest lists, so
// anything else (an /api/ call, a POST, an authenticated request) never
// reaches it, and nothing authenticated is ever stored. Every file is checked
// against its SHA-256 when it is stored and again when it is read.
package wasmcache

import (
	"crypto/sha256"
	_ "embed" // the worker, the page script and the test kit are files of this package
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed sw.js
var worker string

//go:embed client.js
var client []byte

//go:embed testkit/wasmcache-e2e.cjs
var testkit []byte

// Config says what to cache.
type Config struct {
	// Dir is the built site's root.
	Dir string
	// Assets name the files to keep, as paths relative to Dir: a glob
	// ("playground/*.wasm", path.Match syntax) or a directory with a
	// trailing slash ("playground/fonts/", everything beneath it).
	Assets []string
	// Lazy name files the same way, kept like the others but stored when
	// first asked for (verified against the manifest) rather than at
	// install: an export engine, fonts for one language. Everything in
	// Assets is downloaded in the background after the first visit.
	Lazy []string
	// Scopes are the paths a worker controls, each starting and ending with
	// a slash ("/playground/", "/fr/playground/"). A worker can only
	// control the directory it is served from, so there is one per scope.
	Scopes []string
	// Exclude are path prefixes that are never cached, whatever Assets say
	// ("/api/" by default: a server mode's calls, authenticated or not).
	Exclude []string
	// Version is the deploy's identity (a commit); the playground's pages
	// carry it as ?v=. Empty: the build's content hash.
	Version string
	// Name prefixes the cache's name ("wasmcache").
	Name string
	// Home is the directory the page script and the version file are
	// written to, "/playground/" by default (the first scope).
	Home string
}

// File is one cached file of the manifest.
type File struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	Lazy   bool   `json:"lazy,omitempty"`
}

// Manifest is what Generate decided; the worker holds it.
type Manifest struct {
	Name    string   `json:"name"`
	Build   string   `json:"build"`
	Version string   `json:"version"`
	Exclude []string `json:"exclude"`
	Files   []File   `json:"files"`
}

// Generate writes the worker, the page script and the version file into
// cfg.Dir and returns the manifest.
func Generate(cfg Config) (*Manifest, error) {
	if cfg.Name == "" {
		cfg.Name = "wasmcache"
	}
	if len(cfg.Exclude) == 0 {
		cfg.Exclude = []string{"/api/"}
	}
	if len(cfg.Scopes) == 0 {
		return nil, errors.New("wasmcache: no scope")
	}
	if len(cfg.Assets)+len(cfg.Lazy) == 0 {
		return nil, errors.New("wasmcache: no asset")
	}
	if cfg.Home == "" {
		cfg.Home = cfg.Scopes[0]
	}
	for _, s := range append([]string{cfg.Home}, cfg.Scopes...) {
		if !strings.HasPrefix(s, "/") || !strings.HasSuffix(s, "/") || strings.Contains(s, "..") {
			return nil, fmt.Errorf("wasmcache: %q must start and end with a slash and hold no ..", s)
		}
	}
	for _, x := range cfg.Exclude {
		if !strings.HasPrefix(x, "/") {
			return nil, fmt.Errorf("wasmcache: excluded prefix %q must start with a slash", x)
		}
	}
	files, err := collect(cfg)
	if err != nil {
		return nil, err
	}
	files = append(files, File{Path: cfg.Home + "wasmcache-client.js", SHA256: sum(client), Size: int64(len(client))})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	h := sha256.New()
	for _, f := range files {
		fmt.Fprintf(h, "%s %s\n", f.Path, f.SHA256)
	}
	m := &Manifest{Name: cfg.Name, Build: hex.EncodeToString(h.Sum(nil))[:16], Version: cfg.Version, Exclude: cfg.Exclude, Files: files}
	if m.Version == "" {
		m.Version = m.Build
	}
	conf, _ := json.Marshal(m) // strings, numbers and slices of them: it cannot fail
	sw := strings.Replace(worker, "/*WASMCACHE_CONFIG*/{}", string(conf), 1)
	for _, s := range cfg.Scopes {
		if err := write(cfg.Dir, s+"wasmcache-sw.js", []byte(sw)); err != nil {
			return nil, err
		}
	}
	ver, _ := json.Marshal(map[string]string{"name": m.Name, "build": m.Build, "version": m.Version})
	if err := write(cfg.Dir, cfg.Home+"wasmcache.json", append(ver, '\n')); err != nil {
		return nil, err
	}
	return m, write(cfg.Dir, cfg.Home+"wasmcache-client.js", client)
}

// Testkit returns the browser tests (a node script driving headless Chrome
// through puppeteer-core) a playground runs against its built site.
func Testkit() []byte { return append([]byte(nil), testkit...) }

func sum(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func write(dir, url string, b []byte) error {
	p := filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(url, "/")))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

// collect hashes every file of the site an asset pattern names.
func collect(cfg Config) ([]File, error) {
	var out []File
	pats := append(append([]string(nil), cfg.Assets...), cfg.Lazy...)
	used := make([]bool, len(pats))
	err := filepath.WalkDir(cfg.Dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(cfg.Dir, p)
		rel = filepath.ToSlash(rel)
		if !d.Type().IsRegular() {
			return nil // a symlink could point anywhere
		}
		if base := path.Base(rel); base == "wasmcache-sw.js" || base == "wasmcache.json" || base == "wasmcache-client.js" {
			return nil // ours, from an earlier run
		}
		hit, lazy := false, false
		for i, pat := range pats {
			if matches(pat, rel) {
				used[i], hit = true, true
				lazy = lazy || i >= len(cfg.Assets)
			}
		}
		if !hit {
			return nil
		}
		for _, x := range cfg.Exclude {
			if strings.HasPrefix("/"+rel, x) {
				return fmt.Errorf("wasmcache: /%s is under the excluded %s", rel, x)
			}
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out = append(out, File{Path: "/" + rel, SHA256: sum(b), Size: int64(len(b)), Lazy: lazy})
		return nil
	})
	if err != nil {
		return nil, err
	}
	for i, u := range used {
		if !u {
			return nil, fmt.Errorf("wasmcache: asset %q matches no file under %s", pats[i], cfg.Dir)
		}
	}
	return out, nil
}

func matches(pat, rel string) bool {
	if strings.HasSuffix(pat, "/") {
		return strings.HasPrefix(rel, pat)
	}
	ok, _ := path.Match(pat, rel)
	return ok
}
