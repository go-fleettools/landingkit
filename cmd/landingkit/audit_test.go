package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// auditServer serves the fleet's shapes over TLS and points audit's client
// at it for the test.
func auditServer(t *testing.T) *httptest.Server {
	t.Helper()
	kitRow := `<div class=cta data-kit-cta><a class="btn btn-primary" href=/docs/>` + icon + `Docs</a><a class="btn btn-try" href=/playground/>` + icon + `Try</a></div>`
	ghostTry := `<div class=cta><a class="btn btn-primary" href=/docs/>` + icon + `Docs</a><a class="btn btn-ghost" href=/playground/>Try it in the browser →</a></div>`
	mux := http.NewServeMux()
	mux.HandleFunc("/ok/", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(ctaPage(kitRow))) })
	mux.HandleFunc("/drift/", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(ctaPage(ghostTry))) })
	mux.HandleFunc("/said-twice/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(page(`x`, `<footer>BSD-3-Clause</footer>`)))
	})
	mux.HandleFunc("/huge/", func(w http.ResponseWriter, r *http.Request) {
		w.Write(bytes.Repeat([]byte("a"), auditMaxBody+1))
	})
	mux.HandleFunc("/truncated/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		w.Write([]byte("<html>cut"))
	})
	mux.HandleFunc("/loop/", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/loop/", http.StatusFound) })
	mux.HandleFunc("/to-http/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://example.invalid/", http.StatusFound)
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	saved := auditClient
	c := srv.Client()
	c.CheckRedirect = saved.CheckRedirect
	auditClient = c
	t.Cleanup(func() { auditClient = saved })
	return srv
}

func runAudit(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(append([]string{"audit"}, args...), &out, &errb)
	return code, out.String() + errb.String()
}

func TestAuditPassesAKitLanding(t *testing.T) {
	srv := auditServer(t)
	code, msg := runAudit(t, srv.URL+"/ok/")
	if code != 0 || !strings.Contains(msg, "ok     "+srv.URL+"/ok/ (1 hero row(s))") || !strings.Contains(msg, "1 ok, 0 drifted, 0 unread") {
		t.Fatalf("exit %d:\n%s", code, msg)
	}
}

// The landing that prompted the tool, among good ones: the verdict names it
// and its findings, and the others still pass.
func TestAuditNamesTheDriftedLanding(t *testing.T) {
	srv := auditServer(t)
	code, msg := runAudit(t, srv.URL+"/ok/", srv.URL+"/drift/", srv.URL+"/said-twice/")
	if code != 1 {
		t.Fatalf("exit %d, want 1:\n%s", code, msg)
	}
	for _, want := range []string{
		"DRIFT  " + srv.URL + "/drift/: a hero call-to-action row written by hand",
		"DRIFT  " + srv.URL + `/drift/: the playground link "Try it in the browser →" is not the red btn-try button`,
		"DRIFT  " + srv.URL + `/said-twice/: footer repeats "BSD-3-Clause", which the mantra strip already says`,
		"audit: 3 page(s), 1 ok, 2 drifted, 0 unread",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q in:\n%s", want, msg)
		}
	}
}

// An out-of-order row is drift too.
func TestAuditSeesTheOrder(t *testing.T) {
	srv := auditServer(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(ctaPage(`<div class=cta data-kit-cta><a class="btn btn-try" href=/playground/>` + icon + `Try</a><a class="btn btn-primary" href=/docs/>` + icon + `Docs</a></div>`)))
	})
	srv.Config.Handler = mux
	if code, msg := runAudit(t, srv.URL+"/"); code != 1 || !strings.Contains(msg, `"btn-primary" comes after "btn-try"`) {
		t.Fatalf("exit %d:\n%s", code, msg)
	}
}

// What could not be read is UNREAD and fails the run: never a pass.
func TestAuditNeverPassesWhatItCouldNotRead(t *testing.T) {
	srv := auditServer(t)
	for name, tc := range map[string]struct{ url, want string }{
		"404":              {srv.URL + "/absent", "HTTP 404"},
		"too large":        {srv.URL + "/huge/", "page larger than"},
		"truncated body":   {srv.URL + "/truncated/", "unexpected EOF"},
		"redirect loop":    {srv.URL + "/loop/", "more than 5 redirects"},
		"redirect to http": {srv.URL + "/to-http/", "not https"},
		"plain http":       {"http://example.invalid/", "not an https URL"},
		"not a URL":        {"::", "not an https URL"},
		"no host":          {"https:///x", "not an https URL"},
		"unreachable":      {"https://127.0.0.1:1/", "UNREAD"},
	} {
		t.Run(name, func(t *testing.T) {
			code, msg := runAudit(t, tc.url)
			if code != 1 || !strings.Contains(msg, "UNREAD "+tc.url) || !strings.Contains(msg, tc.want) || !strings.Contains(msg, "1 unread") {
				t.Fatalf("exit %d, want 1 and %q:\n%s", code, tc.want, msg)
			}
		})
	}
}

func TestAuditUsage(t *testing.T) {
	if code, _ := runAudit(t); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if code, _ := runAudit(t, "-f"); code != 2 {
		t.Fatalf("-f without a file: exit %d, want 2", code)
	}
}

// -f reads the URLs from a file, skipping blanks and comments; a missing or
// empty list is a usage error, never an empty pass.
func TestAuditReadsAList(t *testing.T) {
	srv := auditServer(t)
	dir := t.TempDir()
	list := filepath.Join(dir, "urls.txt")
	if err := os.WriteFile(list, []byte("# the fleet\n\n"+srv.URL+"/ok/\n  "+srv.URL+"/drift/  \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, msg := runAudit(t, "-f", list)
	if code != 1 || !strings.Contains(msg, "audit: 2 page(s), 1 ok, 1 drifted, 0 unread") {
		t.Fatalf("exit %d:\n%s", code, msg)
	}
	empty := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(empty, []byte("# nothing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{empty, filepath.Join(dir, "absent.txt")} {
		if code, msg := runAudit(t, "-f", f); code != 2 || !strings.Contains(msg, "landingkit audit:") {
			t.Fatalf("%s: exit %d:\n%s", f, code, msg)
		}
	}
}

// The redirect rule on its own: the client the tool ships with.
func TestAuditClientRefusesRedirects(t *testing.T) {
	req := httptest.NewRequest("GET", "https://a.example/", nil)
	if err := auditClient.CheckRedirect(req, make([]*http.Request, 1)); err != nil {
		t.Fatalf("one https redirect refused: %v", err)
	}
}
