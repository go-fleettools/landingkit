package main

import (
	"net/http"
	"strings"
	"testing"
)

const kitLine = `<span class="kit-version" style="color:var(--slate)"><a href="https://github.com/go-fleettools/landingkit/releases/tag/v0.36.0" style="color:inherit">landingkit v0.36.0</a></span>`

func TestKitVersionReadsEveryFormThePartialCanPrint(t *testing.T) {
	for name, tc := range map[string]struct{ html, want string }{
		"tagged and linked":  {kitLine, "v0.36.0"},
		"minified":           {`<span class=kit-version style=color:var(--slate)><a href=https://x/releases/tag/v0.9.1 style=color:inherit>landingkit v0.9.1</a></span>`, "v0.9.1"},
		"local, no link":     {`<span class="kit-version" style="color:var(--slate)">landingkit (local)</span>`, "(local)"},
		"pseudo-version":     {`<span class="kit-version"><a href="https://x/commit/abcdef123456">landingkit v0.36.1-0.20261115120000-abcdef123456</a></span>`, "v0.36.1-0.20261115120000-abcdef123456"},
		"none":               {`<footer>© the authors</footer>`, ""},
		"another span":       {`<span class="kit-versions-note">landingkit v1.0.0</span>`, ""},
		"the words in prose": {`<footer>built with landingkit v0.36.0</footer>`, ""},
	} {
		if got := kitVersion(tc.html); got != tc.want {
			t.Errorf("%s: kitVersion = %q, want %q", name, got, tc.want)
		}
	}
}

// check WARNS about a site with a footer and no version line, and still
// passes: failing would block the pin bump that makes the line available.
func TestCheckWarnsWithoutTheVersionLine(t *testing.T) {
	bare := checkSite(t, page(`A gateway.`, `<footer><div class=wrap><span>© the authors</span></div></footer>`))
	code, msg := runCheck(t, bare)
	if code != 0 || !strings.Contains(msg, "check: warning: no page carries the kit-version line") {
		t.Fatalf("bare site: exit %d:\n%s", code, msg)
	}
	with := checkSite(t, page(`A gateway.`, `<footer><div class=wrap><span>© the authors</span>`+kitLine+`</div></footer>`))
	code, msg = runCheck(t, with)
	if code != 0 || strings.Contains(msg, "warning") {
		t.Fatalf("site with the line: exit %d:\n%s", code, msg)
	}
	// A problem elsewhere is still a failure, and the warning does not hide it.
	both := checkSite(t, page(`x`, `<footer>BSD-3-Clause</footer>`))
	if code, msg = runCheck(t, both); code != 1 || !strings.Contains(msg, `footer repeats "BSD-3-Clause"`) {
		t.Fatalf("exit %d:\n%s", code, msg)
	}
}

// audit prints the version each published page reads, "none" for a page
// without the line, and a tally so drift across many landings is one line.
func TestAuditPrintsTheKitVersion(t *testing.T) {
	srv := auditServer(t)
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/new/":
			w.Write([]byte(page(`x`, `<footer>`+kitLine+`</footer>`)))
		case "/old/":
			w.Write([]byte(page(`x`, `<footer><span class="kit-version">landingkit v0.28.0</span></footer>`)))
		case "/local/":
			w.Write([]byte(page(`x`, `<footer><span class="kit-version">landingkit (local)</span></footer>`)))
		case "/bare/":
			w.Write([]byte(page(`x`, `<footer>© the authors</footer>`)))
		case "/drift/":
			w.Write([]byte(page(`x`, `<footer>BSD-3-Clause `+kitLine+`</footer>`)))
		default:
			http.NotFound(w, r)
		}
	})
	code, msg := runAudit(t, srv.URL+"/new/", srv.URL+"/old/", srv.URL+"/local/", srv.URL+"/bare/", srv.URL+"/drift/", srv.URL+"/absent/")
	if code != 1 {
		t.Fatalf("exit %d:\n%s", code, msg)
	}
	for _, want := range []string{
		"ok     " + srv.URL + "/new/ (0 hero row(s)) kit v0.36.0",
		"ok     " + srv.URL + "/old/ (0 hero row(s)) kit v0.28.0",
		"ok     " + srv.URL + "/local/ (0 hero row(s)) kit (local)",
		"ok     " + srv.URL + "/bare/ (0 hero row(s)) kit none",
		"kit    " + srv.URL + "/drift/: v0.36.0", // a drifted page still reports its version
		"kit versions: (local) x1, none x1, v0.28.0 x1, v0.36.0 x2",
		"UNREAD " + srv.URL + "/absent/: HTTP 404", // an unread page has no version and is not tallied
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q in:\n%s", want, msg)
		}
	}
}
