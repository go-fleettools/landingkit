package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The `btn-try` variant (layouts/partials/btn-try-style.html): a red button
// with white text, opted into by a site that calls the partial and puts the
// class on a button. WCAG 2.2: text 4.5:1 (SC 1.4.3), a focus indicator 3:1
// against what it touches (SC 1.4.11, 2.4.11): the button and the page.
const minNonText = 3.0

var tryRule = regexp.MustCompile(`\.btn\.btn-try(:hover|:focus-visible)?\{([^}]*)\}`)

func tryColours(t *testing.T) (bg, hoverBg string) {
	t.Helper()
	b, err := os.ReadFile("../../layouts/partials/btn-try-style.html")
	if err != nil {
		t.Fatal(err)
	}
	sheet := string(b)
	for _, m := range tryRule.FindAllStringSubmatch(sheet, -1) {
		bgm := regexp.MustCompile(`(?:^|;)background:(#[0-9a-fA-F]{6})`).FindStringSubmatch(m[2])
		switch m[1] {
		case "":
			if bgm == nil || !strings.Contains(m[2], "color:#fff") {
				t.Fatalf("the base rule has no #rrggbb background and white text: %q", m[2])
			}
			bg = bgm[1]
		case ":hover":
			if bgm == nil {
				t.Fatalf("the hover rule has no #rrggbb background: %q", m[2])
			}
			hoverBg = bgm[1]
		case ":focus-visible":
			if !strings.Contains(m[2], "outline:2px solid var(--ink)") {
				t.Fatalf("the focus ring is not 2px in --ink: %q", m[2])
			}
		}
	}
	if bg == "" || hoverBg == "" {
		t.Fatal("missing the base or the hover rule")
	}
	if !strings.Contains(sheet, ":focus-visible{") {
		t.Fatal("no focus state")
	}
	return bg, hoverBg
}

func TestBtnTryContrast(t *testing.T) {
	bg, hoverBg := tryColours(t)
	styles, err := os.ReadFile("../../layouts/partials/styles.html")
	if err != nil {
		t.Fatal(err)
	}
	// comments out: lastDecl reads a declaration after a ";", and a comment
	// before the first one of a line would hide it
	css := regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(resolve(string(styles), map[string]string{}), "")
	for _, c := range []struct{ name, hex string }{{"rest", bg}, {"hover", hoverBg}} {
		if got := contrast(white, hexRGB(c.hex)); got < minContrast {
			t.Errorf("white on the %s red %s is %.2f:1, under %.1f:1", c.name, c.hex, got, minContrast)
		} else {
			t.Logf("white on the %s red %s: %.2f:1", c.name, c.hex, got)
		}
	}
	for _, theme := range []struct{ name, selector string }{
		{"light", ":root"},
		{"dark (toggle)", `:root[data-theme="dark"]`},
		{"dark (system)", ":root:not([data-theme])"},
	} {
		ink, page := lastDecl(css, theme.selector, "--ink"), lastDecl(css, theme.selector, "--bg")
		if ink == "" {
			ink = lastDecl(css, ":root", "--ink")
		}
		if page == "" {
			page = lastDecl(css, ":root", "--bg")
		}
		if !hexStop.MatchString(ink) || !hexStop.MatchString(page) {
			t.Fatalf("%s: no #rrggbb --ink (%q) and --bg (%q)", theme.name, ink, page)
		}
		// the ring is offset 2px from the button: it touches the page; the
		// button's own edge is the other side of the gap, and a ring that
		// does not read against the button would not read when the offset is
		// lost to a clipped parent.
		for _, against := range []struct{ name, hex string }{{"page", page}, {"button", bg}, {"hovered button", hoverBg}} {
			if got := contrast(hexRGB(ink), hexRGB(against.hex)); got < minNonText {
				t.Errorf("%s: the ring (--ink %s) on the %s %s is %.2f:1, under %.1f:1", theme.name, ink, against.name, against.hex, got, minNonText)
			}
		}
	}
}

// The variant is opt-in: nothing but its own partial (and the README, the
// examples that show it) mentions it, so a site that does not call the
// partial renders as it did.
func TestBtnTryIsOptIn(t *testing.T) {
	err := filepath.Walk("../../layouts", func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Base(p) == "btn-try-style.html" {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if filepath.Base(p) == "cta.html" {
			// The row renders the button's MARKUP when a site lists a "try"
			// item; it carries no rule for it, which stays in btn-try-style.html.
			if strings.Contains(string(b), "<style") {
				t.Errorf("%s carries a stylesheet: the variant's rules must stay in btn-try-style.html", p)
			}
			return nil
		}
		if strings.Contains(string(b), "btn-try") {
			t.Errorf("%s mentions btn-try: the variant must stay in its own partial", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// An icon is decoration: the button's text names it. Every icon partial is
// aria-hidden, not focusable, 16 px and drawn in currentColor, so it takes the
// text's colour, and so its contrast (white on btn-try's red).
func TestIconsAreDecorative(t *testing.T) {
	files, err := filepath.Glob("../../layouts/partials/icons/*.html")
	if err != nil || len(files) < 3 {
		t.Fatalf("icons: %v, %v", files, err)
	}
	var sawBrowser bool
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		for _, want := range []string{`aria-hidden="true"`, `focusable="false"`, `fill="currentColor"`, `width="16"`, `class="icon"`} {
			if !strings.Contains(s, want) {
				t.Errorf("%s lacks %s", f, want)
			}
		}
		if strings.Contains(s, "<title") {
			t.Errorf("%s has a <title>: a decorative icon is not named", f)
		}
		sawBrowser = sawBrowser || filepath.Base(f) == "browser.html"
	}
	if !sawBrowser {
		t.Error("icons/browser.html is missing")
	}
}

// inline-code-style.html is opt-in like btn-try: its rule is in no other
// partial, so a site that does not call it is byte for byte what it was.
func TestInlineCodeStyleIsOptInAndKeepsInlineCodeWhole(t *testing.T) {
	b, err := os.ReadFile("../../layouts/partials/inline-code-style.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{":not(pre)>code{", "display:inline-block", "max-width:100%", "overflow-wrap:anywhere"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("inline-code-style.html lacks %q", want)
		}
	}
	if strings.HasSuffix(string(b), "\n") {
		t.Error("a partial must not end with a newline")
	}
	err = filepath.Walk("../../layouts", func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Base(p) == "inline-code-style.html" {
			return err
		}
		c, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if strings.Contains(string(c), ":not(pre)>code") {
			t.Errorf("%s carries the inline-code rule: it must stay in its own partial", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
