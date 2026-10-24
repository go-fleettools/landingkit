package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestProbe(t *testing.T) {
	old := rawBase
	rawBase = "https://raw.githubusercontent.com"
	defer func() { rawBase = old }()
	for in, want := range map[string]string{
		// A blob link is asked of the raw host, anchor and query stripped.
		"https://github.com/o/r/blob/main/README.md":          "https://raw.githubusercontent.com/o/r/main/README.md",
		"https://github.com/o/r/blob/v1.2.0/a/b.go#L10-L20":   "https://raw.githubusercontent.com/o/r/v1.2.0/a/b.go",
		"https://github.com/o/r/blob/main/x.md?plain=1#intro": "https://raw.githubusercontent.com/o/r/main/x.md",
		// Everything else is fetched as it is.
		"https://github.com/o/r":                    "https://github.com/o/r",
		"https://github.com/o/r/tree/main/docs":     "https://github.com/o/r/tree/main/docs",
		"https://github.com/blob/x":                 "https://github.com/blob/x",
		"http://github.com/o/r/blob/main/README.md": "http://github.com/o/r/blob/main/README.md",
		"https://example.org/o/r/blob/main/x":       "https://example.org/o/r/blob/main/x",
	} {
		if got := probe(in); got != want {
			t.Errorf("probe(%q) = %q, want %q", in, got, want)
		}
	}
}

// raw stands for raw.githubusercontent.com and for any other host: it serves
// the files it has, 404 for the rest, and counts what it was asked.
func raw(t *testing.T, files ...string) (*httptest.Server, *[]string) {
	t.Helper()
	var asked []string
	have := map[string]bool{}
	for _, f := range files {
		have[f] = true
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Path)
		if r.URL.Path == "/moved" {
			http.Redirect(w, r, "/o/r/main/README.md", http.StatusFound)
			return
		}
		if !have[r.URL.Path] {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)
	old := rawBase
	rawBase = srv.URL
	t.Cleanup(func() { rawBase = old })
	oldSleep := sleep
	sleep = func(time.Duration) {}
	t.Cleanup(func() { sleep = oldSleep })
	return srv, &asked
}

func site(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func links(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(append([]string{"links"}, args...), &out, &errb)
	return code, out.String(), errb.String()
}

// The point of the rewrite: a blob link to a file that exists passes, and
// one to a file that does not FAILS -- github.com itself is never asked.
func TestBlobLinks(t *testing.T) {
	_, asked := raw(t, "/o/r/main/README.md")

	ok := site(t, map[string]string{"index.html": `<a href="https://github.com/o/r/blob/main/README.md#usage">readme</a>`})
	code, out, errs := links("-attempts", "1", ok)
	if code != 0 || !strings.Contains(out, "  ok   200  https://github.com/o/r/blob/main/README.md#usage  (via ") {
		t.Fatalf("an existing blob path: exit %d\n%s%s", code, out, errs)
	}

	missing := site(t, map[string]string{"index.html": `<a href="https://github.com/o/r/blob/main/GONE.md">gone</a>`})
	code, out, errs = links(missing)
	if code != 1 || !strings.Contains(out, "  FAIL 404  https://github.com/o/r/blob/main/GONE.md") ||
		!strings.Contains(errs, "1 link(s) or image(s) do not resolve") {
		t.Fatalf("a missing blob path: exit %d\n%s%s", code, out, errs)
	}
	// Three attempts at the missing file, one at the existing one.
	if got := strings.Join(*asked, " "); got != "/o/r/main/README.md /o/r/main/GONE.md /o/r/main/GONE.md /o/r/main/GONE.md" {
		t.Fatalf("asked %s", got)
	}
}

func TestLinksOtherURLs(t *testing.T) {
	srv, asked := raw(t, "/o/r/main/README.md", "/page")
	dir := site(t, map[string]string{
		"index.html":     `see ` + srv.URL + `/page. And <img src="https://img.shields.io/badge/x-y-green">, ` + srv.URL + `/moved,`,
		"sub/other.html": `"` + srv.URL + `/page"`,
		"app.wasm":       "\x00" + srv.URL + "/never-read",
	})
	code, out, errs := links(dir)
	if code != 0 {
		t.Fatalf("exit %d\n%s%s", code, out, errs)
	}
	for _, want := range []string{"checking 3 distinct URLs", "  ok   200  " + srv.URL + "/page\n", "  ok   200  " + srv.URL + "/moved\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(strings.Join(*asked, " "), "never-read") || strings.Contains(out, "shields") {
		t.Fatalf("asked %v\n%s", *asked, out)
	}
}

// "Not now" is not "not found". A 429 (and a 503 that says when to come
// back) is retried with backoff, honouring Retry-After within a bound, and
// then reported as unknown with a warning -- go-llmflow's link check failed
// on docs.vllm.ai answering 429. A 404, a 410 and a 503 with no Retry-After
// still fail.
func TestLinksRateLimited(t *testing.T) {
	var waits []time.Duration
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/busy":
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusTooManyRequests)
		case "/busy-forever":
			w.Header().Set("Retry-After", "86400")
			w.WriteHeader(http.StatusTooManyRequests)
		case "/maint":
			w.Header().Set("Retry-After", "Sat, 24 Oct 2026 06:00:30 GMT")
			w.WriteHeader(http.StatusServiceUnavailable)
		case "/down":
			w.WriteHeader(http.StatusServiceUnavailable) // no Retry-After: broken
		case "/gone":
			w.WriteHeader(http.StatusGone)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	oldSleep, oldNow := sleep, now
	sleep = func(d time.Duration) { waits = append(waits, d) }
	now = func() time.Time { return time.Date(2026, 10, 24, 6, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { sleep, now = oldSleep, oldNow })

	// rate limited only: a warning, exit 0
	code, out, errs := links("-wait", "1s", "-max-retry-after", "30s",
		site(t, map[string]string{"a.html": srv.URL + "/busy " + srv.URL + "/busy-forever " + srv.URL + "/maint"}))
	if code != 0 || strings.Count(out, "  WARN ") != 3 || !strings.Contains(out, "  WARN 429  "+srv.URL+"/busy  (rate limited") ||
		!strings.Contains(out, "  WARN 503  "+srv.URL+"/maint") || !strings.Contains(errs, "warning: 3 link(s) could not be checked") {
		t.Fatalf("exit %d\n%s%s", code, out, errs)
	}
	// Retry-After honoured (7s over a 1s, 2s backoff), bounded (a day is
	// 30s), and an HTTP-date read against the clock (30s); two waits each
	want := []time.Duration{7 * time.Second, 7 * time.Second, 30 * time.Second, 30 * time.Second, 30 * time.Second, 30 * time.Second}
	if fmt.Sprint(waits) != fmt.Sprint(want) {
		t.Errorf("waited %v, want %v", waits, want)
	}

	// broken links still fail, and are not hidden among the warnings
	waits = nil
	code, out, errs = links("-attempts", "2", site(t, map[string]string{"b.html": srv.URL + "/gone " + srv.URL + "/missing " + srv.URL + "/down " + srv.URL + "/busy"}))
	if code != 1 || !strings.Contains(out, "  FAIL 410  ") || !strings.Contains(out, "  FAIL 404  ") || !strings.Contains(out, "  FAIL 503  "+srv.URL+"/down") ||
		!strings.Contains(out, "  WARN 429  ") || !strings.Contains(errs, "3 link(s) or image(s) do not resolve") {
		t.Fatalf("exit %d\n%s%s", code, out, errs)
	}
}

func TestRetryAfter(t *testing.T) {
	at := time.Date(2026, 10, 24, 6, 0, 0, 0, time.UTC)
	for v, want := range map[string]time.Duration{
		"":                              0,
		"120":                           2 * time.Minute,
		"-3":                            0,
		"soon":                          0,
		"Sat, 24 Oct 2026 06:01:00 GMT": time.Minute,
		"Sat, 24 Oct 2026 05:59:00 GMT": 0, // the past
	} {
		if got := retryAfter(v, at); got != want {
			t.Errorf("retryAfter(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestLinksNoAnswer(t *testing.T) {
	srv, _ := raw(t)
	dead := srv.URL + "/x"
	srv.Close()
	code, out, _ := links("-attempts", "2", "-timeout", "2s", site(t, map[string]string{"a.html": dead}))
	if code != 1 || !strings.Contains(out, "  FAIL 000  "+dead) {
		t.Fatalf("exit %d\n%s", code, out)
	}
}

func TestLinksErrors(t *testing.T) {
	for _, c := range []struct {
		args []string
		code int
		says string
	}{
		{[]string{"-h"}, 0, "usage: landingkit links"},
		{[]string{"-nope"}, 2, "usage: landingkit links"},
		{nil, 2, "usage: landingkit links"},
		{[]string{"-attempts", "0", "d"}, 2, "usage: landingkit links"},
		{[]string{filepath.Join(t.TempDir(), "absent")}, 1, "absent"},
		{[]string{site(t, map[string]string{"index.html": "no links"})}, 1, "no links found"},
	} {
		code, _, errs := links(c.args...)
		if code != c.code || !strings.Contains(errs, c.says) {
			t.Errorf("%v: exit %d, stderr %q", c.args, code, errs)
		}
	}
}

func TestLinksUnreadableFile(t *testing.T) {
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Fatal("this test needs a Unix account that file modes apply to")
	}
	dir := site(t, map[string]string{"index.html": "https://example.invalid/"})
	p := filepath.Join(dir, "index.html")
	if err := os.Chmod(p, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(p, 0o644)
	if code, _, errs := links(dir); code != 1 || !strings.Contains(errs, "permission denied") {
		t.Fatalf("exit %d: %s", code, errs)
	}
}
