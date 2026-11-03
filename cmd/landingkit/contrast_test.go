package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The hero's accent line (`h1.head .accent`) is text painted with a
// gradient. Each of its stops must read against the page's background, in
// both themes, for every organisation's brand: WCAG 2.2 SC 1.4.3 asks
// 4.5:1 (3:1 for large text; we hold the hero, which is large, to 4.5).
// v0.5.0 painted it with --grad (base→deep) in dark mode too: go-slides'
// indigo on #0b0e14 was nearly invisible. This test reads the sheet itself,
// resolves its template variables against each brand, and computes the
// contrast of what a browser with color-mix() would paint.
//
// The bar is minContrast, which brand.go holds the generated accents to.

var kv = regexp.MustCompile(`(?m)^\s*(\w+)\s*=\s*"([^"]*)"`)

// brands are the brands the sheet is checked against: the kit's defaults (an
// empty map), every real organisation in testdata/logos, and the document
// family's indigo (go-slides, go-ooxml and go-richdoc share it), which showed
// the gradient-text defect and then the accent one.
//
// ⛔ GENERATED, NOT READ FROM THE .toml GOLDENS. Those goldens are kit.py's
// output, which is the equality oracle for derive() and is frozen there; they
// have no accent keys and never will. What an organisation actually commits is
// what `landingkit brand` prints TODAY, so that is what the sheet is checked
// against — otherwise this test would go on measuring a brand nobody ships.
// derive() is not marking its own homework here: TestBrandIsByteEqualToKitPy
// holds it to kit.py for every key kit.py had, and to an independent integer
// reference (fittedAccent) for the two it did not.
func brands(t *testing.T) map[string]map[string]string {
	t.Helper()
	brands := map[string]map[string]string{"defaults": {}}
	logos, err := filepath.Glob("testdata/logos/go-*.svg")
	if err != nil {
		t.Fatal(err)
	}
	for _, logo := range logos {
		var out, errb bytes.Buffer
		if code := run([]string{"brand", logo}, &out, &errb); code != 0 {
			t.Fatalf("%s: exit %d: %s", logo, code, errb.String())
		}
		m := map[string]string{}
		for _, g := range kv.FindAllStringSubmatch(out.String(), -1) {
			m[g[1]] = g[2]
		}
		brands[strings.TrimSuffix(filepath.Base(logo), ".svg")] = m
	}
	// go-slides is not in testdata (its logo lives in another organisation), so
	// it is written out. The values are the document family's, as the generator
	// prints them for go-ooxml and go-richdoc.
	brands["go-slides"] = map[string]string{
		"base": "#1e1b4b", "deep": "#12102e", "bright": "#4f46e5",
		"chipFg": "#12102e", "chipFgDark": "#9e99f1",
		"accent": "#1e1b4b", "accentDark": "#726bea",
	}
	// ⛔ A glob that matched nothing would leave two brands and pass.
	if len(brands) < 10 {
		t.Fatalf("only %d brands to check", len(brands))
	}
	return brands
}

var (
	// A default is a literal, or ANOTHER variable — which is how the accents
	// fall back to the raw stop they are derived from. A parser that read only
	// the literal form left `{{ $accent | safeCSS }}` in the sheet, and the
	// braces it leaves behind make the :root block unreadable to rootBlk: the
	// test then reports a missing --bg rather than a colour it got wrong, which
	// is the one way round worth having.
	tmplVar  = regexp.MustCompile(`\$(\w+)\s*:=\s*\$br\.(\w+)\s*\|\s*default\s*(?:"([^"]+)"|\$(\w+))`)
	safeCSS  = regexp.MustCompile(`\{\{\s*\$(\w+)\s*\|\s*safeCSS\s*\}\}`)
	rootBlk  = regexp.MustCompile(`(:root(?:\[data-theme="dark"\]|:not\(\[data-theme\]\))?)\{([^{}]*)\}`)
	accentBg = regexp.MustCompile(`h1\.head \.accent\{background:var\((--[\w-]+)\)`)
	mixStop  = regexp.MustCompile(`^color-mix\(in srgb,(#[0-9a-fA-F]{6}) (\d+)%,(#fff|#000)\)$`)
	hexStop  = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

// resolve substitutes the sheet's brand variables with brand's values, or
// the sheet's own defaults.
func resolve(sheet string, brand map[string]string) string {
	vals := map[string]string{}
	// Source order, so a variable used as another's default is already known.
	for _, g := range tmplVar.FindAllStringSubmatch(sheet, -1) {
		v := g[3]
		if g[4] != "" {
			v = vals[g[4]]
		}
		if b, ok := brand[g[2]]; ok {
			v = b
		}
		vals[g[1]] = v
	}
	return safeCSS.ReplaceAllStringFunc(sheet, func(s string) string {
		name := safeCSS.FindStringSubmatch(s)[1]
		if v, ok := vals[name]; ok {
			return v
		}
		return s
	})
}

// comment is a CSS /* … */, which a declaration can sit behind. Splitting a
// block on ';' leaves the comment glued to the front of the declaration after
// it, so a property documented on the line above reads as ABSENT — and absent
// means "inherited from :root" to the callers below, which is a different
// colour checked in silence. Stripping them first is what lets a token be
// explained where it is declared.
var comment = regexp.MustCompile(`(?s)/\*.*?\*/`)

// lastDecl is the last value of prop declared in a :root block with that
// selector, in source order: what the cascade keeps among equal
// specificities (the @supports block comes after the fallbacks).
func lastDecl(css, selector, prop string) string {
	css = comment.ReplaceAllString(css, "")
	var v string
	for _, m := range rootBlk.FindAllStringSubmatch(css, -1) {
		if m[1] != selector {
			continue
		}
		for _, d := range strings.Split(m[2], ";") {
			d = strings.TrimSpace(d)
			if strings.HasPrefix(d, prop+":") {
				v = strings.TrimSpace(strings.TrimPrefix(d, prop+":"))
			}
		}
	}
	return v
}

// gradientStops is the colours of a linear-gradient(angle, stop, ...),
// each a #rrggbb or a color-mix(in srgb, #rrggbb P%, #fff|#000).
func gradientStops(v string) ([]rgb, error) {
	if !strings.HasPrefix(v, "linear-gradient(") || !strings.HasSuffix(v, ")") {
		return nil, fmt.Errorf("not a linear-gradient: %q", v)
	}
	inner := v[len("linear-gradient(") : len(v)-1]
	var args []string
	depth, start := 0, 0
	for i, r := range inner {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				args = append(args, strings.TrimSpace(inner[start:i]))
				start = i + 1
			}
		}
	}
	args = append(args, strings.TrimSpace(inner[start:]))
	if len(args) < 3 {
		return nil, fmt.Errorf("fewer than two stops: %q", v)
	}
	var out []rgb
	for _, a := range args[1:] {
		switch {
		case hexStop.MatchString(a):
			out = append(out, hexRGB(a))
		case mixStop.MatchString(a):
			g := mixStop.FindStringSubmatch(a)
			p, _ := strconv.Atoi(g[2])
			other := rgb{}
			if g[3] == "#fff" {
				other = white
			}
			// CSS Color 5: srgb interpolates the encoded components.
			out = append(out, mix(hexRGB(g[1]), other, 1-float64(p)/100))
		default:
			return nil, fmt.Errorf("a stop this test cannot read: %q in %q", a, v)
		}
	}
	return out, nil
}

func TestGradientTextContrast(t *testing.T) {
	b, err := os.ReadFile("../../layouts/partials/styles.html")
	if err != nil {
		t.Fatal(err)
	}
	sheet := string(b)
	m := accentBg.FindStringSubmatch(sheet)
	if m == nil {
		t.Fatal("no `h1.head .accent{background:var(--…)` rule in the sheet")
	}
	prop := m[1]
	worst, who := map[string]float64{}, map[string]string{}
	defer func() {
		for th, c := range worst {
			t.Logf("%s: lowest stop %.2f:1 (%s)", th, c, who[th])
		}
	}()
	for name, brand := range brands(t) {
		css := resolve(sheet, brand)
		light := lastDecl(css, ":root", prop)
		for _, theme := range []struct{ name, selector string }{
			{"light", ":root"},
			{"dark (toggle)", `:root[data-theme="dark"]`},
			{"dark (system)", ":root:not([data-theme])"},
		} {
			v := lastDecl(css, theme.selector, prop)
			if v == "" {
				v = light // not redefined: inherited from :root
			}
			bg := lastDecl(css, theme.selector, "--bg")
			if bg == "" || !hexStop.MatchString(bg) {
				t.Fatalf("%s: no #rrggbb --bg in %s", name, theme.selector)
			}
			stops, err := gradientStops(v)
			if err != nil {
				t.Fatalf("%s %s: %v", name, theme.name, err)
			}
			for i, s := range stops {
				c := contrast(s, hexRGB(bg))
				if c < worst[theme.name] || worst[theme.name] == 0 {
					worst[theme.name] = c
					who[theme.name] = name
				}
				if c < minContrast {
					t.Errorf("%s, %s: %s stop %d (%s) on %s is %.2f:1, under %.1f:1",
						name, theme.name, prop, i+1, rgbHex(s), bg, c, minContrast)
				}
			}
		}
	}
}

// ⛔ THE SHEET HAD A CONTRAST TEST FOR ONE TOKEN AND NOT FOR THE ONE THAT
// CARRIES TEXT. TestGradientTextContrast has held the hero's accent line since
// v0.5.0 — and --accent, a flat colour on flat text, went unchecked. It paints
//
//	.eyebrow   13px, weight 800
//	.backlink  14px, weight 700
//	.xm        (table.bench tr.lose) through --accent-deep
//
// none of which is "large scale" under WCAG 2.2 (18pt, or 14pt bold = 18.66px),
// so the exception does not apply and the bar is the full 4.5:1. In dark mode
// both tokens were the logo's light stop as read, and the document family's
// indigo #4f46e5 on #0b0e14 is 3.07:1 — printed in the generated brand's own
// comment, by a generator that went on to print the brand.
//
// Measured, not reasoned: this test reads the sheet, resolves it against every
// brand, and asks what a browser would paint.
func TestAccentContrast(t *testing.T) {
	b, err := os.ReadFile("../../layouts/partials/styles.html")
	if err != nil {
		t.Fatal(err)
	}
	sheet := string(b)
	worst, who := map[string]float64{}, map[string]string{}
	defer func() {
		for th, c := range worst {
			t.Logf("%s: lowest accent %.2f:1 (%s)", th, c, who[th])
		}
	}()
	for name, brand := range brands(t) {
		css := resolve(sheet, brand)
		for _, prop := range []string{"--accent", "--accent-deep"} {
			light := lastDecl(css, ":root", prop)
			for _, theme := range []struct{ name, selector string }{
				{"light", ":root"},
				{"dark (toggle)", `:root[data-theme="dark"]`},
				{"dark (system)", ":root:not([data-theme])"},
			} {
				v := lastDecl(css, theme.selector, prop)
				if v == "" {
					v = light // not redefined: inherited from :root
				}
				bg := lastDecl(css, theme.selector, "--bg")
				if bg == "" || !hexStop.MatchString(bg) {
					t.Fatalf("%s: no #rrggbb --bg in %s", name, theme.selector)
				}
				if !hexStop.MatchString(v) {
					t.Fatalf("%s %s: %s is %q, which this test cannot read", name, theme.name, prop, v)
				}
				key := theme.name + " " + prop
				c := contrast(hexRGB(v), hexRGB(bg))
				if c < worst[key] || worst[key] == 0 {
					worst[key], who[key] = c, name
				}
				if c < minContrast {
					t.Errorf("%s, %s: %s (%s) on %s is %.2f:1, under %.1f:1",
						name, theme.name, prop, v, bg, c, minContrast)
				}
			}
		}
	}
}

// The parser must read what it is handed, and refuse what it cannot: a
// stop it skipped would pass unseen.
func TestGradientStops(t *testing.T) {
	s, err := gradientStops("linear-gradient(135deg,#000000,color-mix(in srgb,#ffffff 50%,#000))")
	if err != nil || len(s) != 2 || rgbHex(s[0]) != "#000000" || rgbHex(s[1]) != "#808080" {
		t.Fatalf("got %v, %v", s, err)
	}
	for _, bad := range []string{"#fff", "linear-gradient(135deg,#000000)", "linear-gradient(135deg,red,#000000)"} {
		if _, err := gradientStops(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	// the WCAG anchors: black on white 21:1, #767676 on white 4.54:1
	if c := contrast(rgb{}, white); c < 20.99 || c > 21.01 {
		t.Errorf("black/white %.2f", c)
	}
	if c := contrast(hexRGB("#767676"), white); c < 4.53 || c > 4.55 {
		t.Errorf("#767676/white %.2f", c)
	}
}
