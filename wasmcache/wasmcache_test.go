package wasmcache

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func put(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func site(t *testing.T) string {
	d := t.TempDir()
	put(t, d, "playground/index.html", "<html>")
	put(t, d, "playground/app.wasm", "wasm")
	put(t, d, "playground/fonts/a.ttf", "font")
	put(t, d, "playground/other.txt", "not cached")
	put(t, d, "fr/playground/index.html", "<html fr>")
	put(t, d, "api/x.json", "{}")
	return d
}

func TestGenerate(t *testing.T) {
	d := site(t)
	cfg := Config{Dir: d, Assets: []string{"playground/*.wasm", "playground/index.html", "playground/fonts/", "fr/playground/index.html"},
		Scopes: []string{"/playground/", "/fr/playground/"}}
	m, err := Generate(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range m.Files {
		paths = append(paths, f.Path)
	}
	want := "/fr/playground/index.html /playground/app.wasm /playground/fonts/a.ttf /playground/index.html /playground/wasmcache-client.js"
	if strings.Join(paths, " ") != want {
		t.Fatalf("files %v", paths)
	}
	if m.Version != m.Build || len(m.Build) != 16 || m.Name != "wasmcache" || len(m.Exclude) != 1 || m.Exclude[0] != "/api/" {
		t.Fatalf("manifest %+v", m)
	}
	// every scope's worker holds the manifest, and only that
	re := regexp.MustCompile(`const CONFIG = (\{.*\});`)
	for _, s := range cfg.Scopes {
		b, err := os.ReadFile(filepath.Join(d, s, "wasmcache-sw.js"))
		if err != nil {
			t.Fatal(err)
		}
		var got Manifest
		if err := json.Unmarshal([]byte(re.FindSubmatch(b)[1]), &got); err != nil || got.Build != m.Build || len(got.Files) != 5 {
			t.Fatalf("worker of %s: %v %+v", s, err, got)
		}
	}
	ver, _ := os.ReadFile(filepath.Join(d, "playground/wasmcache.json"))
	if !strings.Contains(string(ver), `"version":"`+m.Version+`"`) {
		t.Fatalf("version file %s", ver)
	}
	if b, _ := os.ReadFile(filepath.Join(d, "playground/wasmcache-client.js")); string(b) != string(client) {
		t.Fatal("the page script was not written")
	}
	// a second run over the same site is the same build: its own files are not assets
	m2, err := Generate(cfg)
	if err != nil || m2.Build != m.Build {
		t.Fatalf("second run: %v %v", err, m2)
	}
	// a content change is a new build, an explicit version is kept
	put(t, d, "playground/app.wasm", "wasm2")
	cfg.Version, cfg.Name, cfg.Home, cfg.Exclude = "abc123", "pg", "/playground/", []string{"/api/", "/private/"}
	m3, err := Generate(cfg)
	if err != nil || m3.Build == m.Build || m3.Version != "abc123" || m3.Name != "pg" {
		t.Fatalf("third run: %v %+v", err, m3)
	}
}

func TestGenerateRefusals(t *testing.T) {
	d := site(t)
	ok := Config{Dir: d, Assets: []string{"playground/*.wasm"}, Scopes: []string{"/playground/"}}
	cases := []struct {
		name string
		edit func(*Config)
		want string
	}{
		{"no scope", func(c *Config) { c.Scopes = nil }, "no scope"},
		{"no asset", func(c *Config) { c.Assets = nil }, "no asset"},
		{"scope without slash", func(c *Config) { c.Scopes = []string{"playground/"} }, "must start and end"},
		{"scope without end slash", func(c *Config) { c.Scopes = []string{"/playground"} }, "must start and end"},
		{"scope with ..", func(c *Config) { c.Scopes = []string{"/a/../b/"} }, "must start and end"},
		{"bad home", func(c *Config) { c.Home = "x/" }, "must start and end"},
		{"bad exclude", func(c *Config) { c.Exclude = []string{"api/"} }, "must start with a slash"},
		{"asset matches nothing", func(c *Config) { c.Assets = []string{"nope/*.wasm"} }, "matches no file"},
		{"asset under an excluded prefix", func(c *Config) { c.Assets = []string{"api/"} }, "excluded"},
		{"no such dir", func(c *Config) { c.Dir = filepath.Join(d, "missing") }, "no such file"},
	}
	for _, c := range cases {
		cfg := ok
		c.edit(&cfg)
		if _, err := Generate(cfg); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", c.name, err, c.want)
		}
	}
}

func TestGenerateWriteErrors(t *testing.T) {
	d := site(t)
	put(t, d, "blocked", "a file where a directory is wanted")
	cfg := Config{Dir: d, Assets: []string{"playground/*.wasm"}, Scopes: []string{"/blocked/"}}
	if _, err := Generate(cfg); err == nil {
		t.Fatal("a worker written beneath a file")
	}
	// the version file fails: the scope's worker is written, its home is not
	cfg = Config{Dir: d, Assets: []string{"playground/*.wasm"}, Scopes: []string{"/playground/"}, Home: "/blocked/"}
	if _, err := Generate(cfg); err == nil {
		t.Fatal("a version file written beneath a file")
	}
	// the page script fails: the version file can be written, the script cannot
	if err := os.MkdirAll(filepath.Join(d, "playground", "wasmcache-client.js"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg = Config{Dir: d, Assets: []string{"playground/*.wasm"}, Scopes: []string{"/playground/"}}
	if _, err := Generate(cfg); err == nil {
		t.Fatal("a page script written over a directory")
	}
}

func TestGenerateSkipsLinksAndReportsUnreadable(t *testing.T) {
	d := site(t)
	if err := os.Symlink(filepath.Join(d, "playground/app.wasm"), filepath.Join(d, "playground/link.wasm")); err != nil {
		t.Skip("no symlinks here")
	}
	m, err := Generate(Config{Dir: d, Assets: []string{"playground/*.wasm"}, Scopes: []string{"/playground/"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range m.Files {
		if strings.HasSuffix(f.Path, "link.wasm") {
			t.Fatal("a symlink was cached")
		}
	}
	if os.Getuid() == 0 || runtime.GOOS == "windows" {
		return
	}
	if err := os.Chmod(filepath.Join(d, "playground/app.wasm"), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(Config{Dir: d, Assets: []string{"playground/*.wasm"}, Scopes: []string{"/playground/"}}); err == nil {
		t.Fatal("an unreadable asset was skipped")
	}
}

func TestTestkitIsACopy(t *testing.T) {
	a := Testkit()
	if !strings.Contains(string(a), "WASMCACHE_OK") {
		t.Fatal("the kit is not the e2e script")
	}
	a[0] = 'X'
	if Testkit()[0] == 'X' {
		t.Fatal("the caller can change the kit")
	}
}

// TestKitInChrome runs the browser tests against a fixture site, when a
// Chrome and node are at hand (CHROME, and NODE_PATH holding puppeteer-core):
// the same kit a playground runs in its own CI.
func TestKitInChrome(t *testing.T) {
	chrome := os.Getenv("CHROME")
	node, err := exec.LookPath("node")
	if (chrome == "" || err != nil) && os.Getenv("WASMCACHE_REQUIRE_BROWSER") != "" {
		t.Fatal("the browser lane has no Chrome or no node: a skipped test is not a pass")
	}
	if chrome == "" || err != nil {
		t.Skip("CHROME and node are needed for the browser tests")
	}
	d := t.TempDir()
	put(t, d, "playground/index.html", `<!doctype html><html lang="en"><meta charset="utf-8"><title>fixture</title>
<script src="/playground/wasmcache-client.js"></script>
<script>
fetch("/playground/app.wasm").then(function (r) { return r.arrayBuffer(); }).then(function () {
  window.appReady = true;
  if (window.wasmcache) wasmcache.start({ intervalMs: 0 });
});
</script>`)
	put(t, d, "playground/app.wasm", strings.Repeat("0123456789abcdef", 1<<16)) // 1 MiB
	put(t, d, "playground/fonts/f.ttf", "font bytes")
	put(t, d, "api/x", "{}")
	if _, err := Generate(Config{Dir: d, Assets: []string{"playground/index.html", "playground/app.wasm", "playground/fonts/"}, Scopes: []string{"/playground/"}}); err != nil {
		t.Fatal(err)
	}
	kit := filepath.Join(t.TempDir(), "wasmcache-e2e.cjs")
	if err := os.WriteFile(kit, Testkit(), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(node, kit, "--root", d, "--page", "/playground/", "--asset", "/playground/app.wasm", "--ready", "window.appReady===true")
	b, err := cmd.CombinedOutput()
	t.Logf("%s", b)
	if err != nil || !strings.Contains(string(b), "WASMCACHE_OK") {
		t.Fatalf("the kit failed: %v", err)
	}
}
