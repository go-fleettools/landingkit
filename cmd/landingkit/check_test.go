package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// What Hugo emits for the default strip, minified (attribute quotes dropped).
const strip = `<style>.nav{padding-bottom:34px}.nav::before,.nav::after{position:absolute}.nav::before{content:"LICENSE=BSD-3-Clause";left:max(24px,calc(50% - 536px))}.nav::after{content:"CGO_ENABLED=0";left:calc(1ch + 28px)}</style>`

func page(sub, footer string) string {
	return `<!doctype html><html><head>` + strip + `</head><body><main><section class=hero><h1 class=head>Fast, <span>in pure Go.</span></h1><p class=sub>` + sub + `</p></section></main>` + footer + `</body></html>`
}

func checkSite(t *testing.T, html string) string {
	t.Helper()
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "index.html"), []byte(html), 0o644); err != nil {
		t.Fatal(err)
	}
	return d
}

func runCheck(t *testing.T, dir string) (int, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run([]string{"check", dir}, &out, &errb)
	return code, out.String() + errb.String()
}

// The planted positive: the footer and the hero say again what the strip says.
func TestCheckFailsRedundantFooterAndHero(t *testing.T) {
	d := checkSite(t, page(`A gateway. <code>CGO_ENABLED=0</code>, BSD-3-Clause.`,
		`<footer><div class=wrap><span>© the authors · BSD-3-Clause</span><span>Pure Go · CGO_ENABLED=0</span></div></footer>`))
	code, msg := runCheck(t, d)
	if code != 1 {
		t.Fatalf("exit %d, want 1: %s", code, msg)
	}
	for _, want := range []string{`footer repeats "BSD-3-Clause"`, `footer repeats "Pure Go"`, `footer repeats "CGO_ENABLED=0"`, `hero sub-text repeats "CGO_ENABLED=0"`, `hero sub-text repeats "BSD-3-Clause"`} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q in:\n%s", want, msg)
		}
	}
}

// The clean landing: the copyright line only, and a hero that says what the
// project is. The heading's "pure Go" is the identity, not a footer fact.
func TestCheckPassesCleanLanding(t *testing.T) {
	d := checkSite(t, page(`A gateway in front of vLLM.`, `<footer><div class=wrap><span>© the authors</span></div></footer>`))
	if code, msg := runCheck(t, d); code != 0 {
		t.Fatalf("exit %d: %s", code, msg)
	}
}

// With the strip off there is nothing to repeat: the own mention is wanted.
func TestCheckIgnoresPageWithoutStrip(t *testing.T) {
	html := strings.Replace(page(`CGO_ENABLED=0`, `<footer>BSD-3-Clause · Pure Go · CGO_ENABLED=0</footer>`), strip, "", 1)
	d := checkSite(t, html)
	code, msg := runCheck(t, d)
	if code != 0 || !strings.Contains(msg, "0 page(s)") {
		t.Fatalf("exit %d: %s", code, msg)
	}
}

// A data licence chip (the third and fourth chips) is a fact too.
func TestCheckSeesAThirdChip(t *testing.T) {
	html := strings.Replace(page(`x`, `<footer>fonts OFL-1.1</footer>`), `</style>`, `.nav>.wrap::before{content:"fonts: OFL-1.1";left:1px}</style>`, 1)
	if code, msg := runCheck(t, checkSite(t, html)); code != 1 || !strings.Contains(msg, `"OFL-1.1"`) {
		t.Fatalf("exit %d: %s", code, msg)
	}
}

// The strip as markup, as the kit's earlier versions wrote it.
func TestCheckSeesMarkupChips(t *testing.T) {
	html := strings.Replace(page(`x`, `<footer>Pure Go</footer>`), strip,
		`<span class=mantra-row><code class=mantra-chip>CGO_ENABLED=0</code></span>`, 1)
	if code, msg := runCheck(t, checkSite(t, html)); code != 1 || !strings.Contains(msg, `"Pure Go"`) {
		t.Fatalf("exit %d: %s", code, msg)
	}
}

func TestCheckUsage(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"check"}, &out, &errb); code != 2 {
		t.Fatalf("exit %d", code)
	}
	if code := run([]string{"check", filepath.Join(t.TempDir(), "absent")}, &out, &errb); code != 1 {
		t.Fatalf("missing dir: exit %d", code)
	}
}
