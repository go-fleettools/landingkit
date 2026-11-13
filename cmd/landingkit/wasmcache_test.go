package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runWC(args ...string) (int, string, string) {
	var o, e bytes.Buffer
	code := run(append([]string{"wasmcache"}, args...), &o, &e)
	return code, o.String(), e.String()
}

func TestWasmcacheCommand(t *testing.T) {
	d := t.TempDir()
	if err := os.MkdirAll(filepath.Join(d, "pg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "pg", "a.wasm"), []byte("w"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, _ := runWC("-dir", d, "-asset", "pg/*.wasm", "-scope", "/pg/", "-version", "v1", "-name", "x", "-home", "/pg/", "-exclude", "/api/")
	if code != 0 || !strings.Contains(out, "version v1, 2 files") {
		t.Fatalf("%d %q", code, out)
	}
	for _, f := range []string{"pg/wasmcache-sw.js", "pg/wasmcache.json", "pg/wasmcache-client.js"} {
		if _, err := os.Stat(filepath.Join(d, f)); err != nil {
			t.Fatal(err)
		}
	}
	if code, _, e := runWC(); code != 2 || !strings.Contains(e, "usage") {
		t.Fatalf("no dir: %d %q", code, e)
	}
	if code, _, _ := runWC("-nope"); code != 2 {
		t.Fatalf("bad flag: %d", code)
	}
	if code, _, e := runWC("-dir", d, "-scope", "/pg/"); code != 1 || !strings.Contains(e, "no asset") {
		t.Fatalf("no asset: %d %q", code, e)
	}
	// -h prints the defaults, which asks each repeatable flag for its value
	if code, _, e := runWC("-h"); code != 2 || !strings.Contains(e, "-asset") {
		t.Fatalf("-h: %d %q", code, e)
	}
	var l list
	l.Set("a")
	l.Set("b")
	if l.String() != "a,b" {
		t.Fatal(l.String())
	}
}

func TestWasmcacheTestkit(t *testing.T) {
	d := t.TempDir()
	code, out, _ := runWC("testkit", "-out", filepath.Join(d, "kit"))
	if code != 0 || !strings.HasSuffix(strings.TrimSpace(out), "wasmcache-e2e.cjs") {
		t.Fatalf("%d %q", code, out)
	}
	if b, err := os.ReadFile(filepath.Join(d, "kit", "wasmcache-e2e.cjs")); err != nil || !strings.Contains(string(b), "WASMCACHE_OK") {
		t.Fatalf("%v", err)
	}
	if code, _, e := runWC("testkit"); code != 2 || !strings.Contains(e, "usage") {
		t.Fatalf("no -out: %d %q", code, e)
	}
	if code, _, _ := runWC("testkit", "-nope"); code != 2 {
		t.Fatalf("bad flag: %d", code)
	}
	if err := os.WriteFile(filepath.Join(d, "file"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := runWC("testkit", "-out", filepath.Join(d, "file", "sub")); code != 1 {
		t.Fatalf("dir under a file: %d", code)
	}
	blocked := filepath.Join(d, "blocked")
	if err := os.MkdirAll(filepath.Join(blocked, "wasmcache-e2e.cjs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := runWC("testkit", "-out", blocked); code != 1 {
		t.Fatalf("file over a directory: %d", code)
	}
}
