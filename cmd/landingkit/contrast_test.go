package main

import (
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
const minContrast = 4.5

// gradientTextBrands are the brands the sheet is checked against: the kit's
// defaults (an empty map), every real organisation in testdata/logos, and
// go-slides' indigo, the one that showed the defect.
func gradientTextBrands(t *testing.T) map[string]map[string]string {
	t.Helper()
	brands := map[string]map[string]string{
		"defaults": {},
		"go-slides": {
			"base": "#1e1b4b", "deep": "#12102e", "bright": "#4f46e5",
			"chipFg": "#12102e", "chipFgDark": "#9e99f1",
		},
	}
	files, err := filepath.Glob("testdata/logos/go-*.toml")
	if err != nil {
		t.Fatal(err)
	}
	kv := regexp.MustCompile(`(?m)^\s*(\w+)\s*=\s*"([^"]*)"`)
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]string{}
		for _, g := range kv.FindAllStringSubmatch(string(b), -1) {
			m[g[1]] = g[2]
		}
		brands[strings.TrimSuffix(filepath.Base(f), ".toml")] = m
	}
	// ⛔ A glob that matched nothing would leave three brands and pass.
	if len(brands) < 10 {
		t.Fatalf("only %d brands to check", len(brands))
	}
	return brands
}

var (
	tmplVar  = regexp.MustCompile(`\$(\w+)\s*:=\s*\$br\.(\w+)\s*\|\s*default\s*"([^"]+)"`)
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
	for _, g := range tmplVar.FindAllStringSubmatch(sheet, -1) {
		v := g[3]
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

// lastDecl is the last value of prop declared in a :root block with that
// selector, in source order: what the cascade keeps among equal
// specificities (the @supports block comes after the fallbacks).
func lastDecl(css, selector, prop string) string {
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
	for name, brand := range gradientTextBrands(t) {
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
