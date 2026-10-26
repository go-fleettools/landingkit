package main

import (
	"fmt"
	"html"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// `check` fails a landing that says again, in its footer or its hero
// sub-text, what the mantra strip already says.
//
// The strip is CSS generated content on the nav bar: in the built page it is
// `.nav::before{content:"LICENSE=BSD-3-Clause"...}`. A landing whose layout
// hard-codes "BSD-3-Clause" and "CGO_ENABLED=0" in its footer, or "Pure Go
// ... CGO_ENABLED=0" in its hero, shows every fact twice; mantra-on.html
// exists so that the own mention appears only without the strip. The check
// reads the BUILT page, as `links` does: the layouts hold template
// expressions, and `{{ if not (partial "mantra-on.html" .) }}` around a
// mention is exactly what must pass.
//
// With the strip off the page has no chip, and the check has nothing to say.

var (
	chipRE    = regexp.MustCompile(`(?s)\.nav(?:>\.wrap)?::(?:before|after)\{[^}]*?content:("(?:[^"\\]|\\.)*")`)
	chipTagRE = regexp.MustCompile(`<code\b[^>]*\bclass="?mantra-chip\b[^>]*>(.*?)</code>`)
	footerRE  = regexp.MustCompile(`(?s)<footer\b[^>]*>(.*?)</footer>`)
	subRE     = regexp.MustCompile(`(?s)<p\b[^>]*\bclass="?[^">]*\bsub\b[^>]*>(.*?)</p>`)
	tagRE     = regexp.MustCompile(`(?s)<[^>]*>`)
	spaceRE   = regexp.MustCompile(`\s+`)
	valueRE   = regexp.MustCompile(`[-.0-9]`)
)

// chips are the strings of the mantra strip in a built page.
func chips(page string) []string {
	var out []string
	for _, m := range chipRE.FindAllStringSubmatch(page, -1) {
		s := m[1]
		s = strings.TrimSuffix(strings.TrimPrefix(s, `"`), `"`)
		s = strings.ReplaceAll(s, `\"`, `"`)
		out = append(out, s)
	}
	// Kit versions that wrote the chips as markup (v0.7.0, which the
	// go-llmflow landing pins, does) rather than as generated content.
	for _, m := range chipTagRE.FindAllStringSubmatch(page, -1) {
		out = append(out, html.UnescapeString(strings.TrimSpace(m[1])))
	}
	return out
}

// facts are the phrases a chip already says: the whole chip, its value when
// the value looks like a licence name (a dash, a dot or a digit, at least
// three characters: so "0" of CGO_ENABLED=0 is not one, the whole chip is
// the fact), and "Pure Go" when the strip carries CGO_ENABLED=0.
func facts(chips []string) []string {
	var out []string
	for _, c := range chips {
		out = append(out, c)
		if i := strings.LastIndexAny(c, "=:"); i >= 0 {
			v := strings.TrimSpace(c[i+1:])
			if len(v) >= 3 && valueRE.MatchString(v) {
				out = append(out, v)
			}
		}
		if strings.Contains(c, "CGO_ENABLED=0") {
			out = append(out, "Pure Go")
		}
	}
	return out
}

func text(markup string) string {
	t := html.UnescapeString(tagRE.ReplaceAllString(markup, " "))
	return strings.TrimSpace(spaceRE.ReplaceAllString(t, " "))
}

// redundant lists what a page's footer and hero sub-text repeat.
func redundant(page string) []string {
	f := facts(chips(page))
	if len(f) == 0 {
		return nil
	}
	where := []string{"footer", "hero sub-text"}
	var found []string
	for i, m := range [][]string{
		firstOf(footerRE.FindAllStringSubmatch(page, -1)),
		firstOf(subRE.FindAllStringSubmatch(page, -1)),
	} {
		for _, body := range m {
			t := strings.ToLower(text(body))
			for _, fact := range f {
				if strings.Contains(t, strings.ToLower(fact)) {
					found = append(found, fmt.Sprintf("%s repeats %q", where[i], fact))
				}
			}
		}
	}
	return found
}

func firstOf(ms [][]string) []string {
	var out []string
	for _, m := range ms {
		out = append(out, m[1])
	}
	return out
}

func cmdCheck(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprint(stderr, "usage: landingkit check <built-site-dir>\n")
		return 2
	}
	bad, seen := 0, 0
	err := filepath.WalkDir(args[0], func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".html" {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		page := string(b)
		if len(chips(page)) > 0 {
			seen++
		}
		for _, r := range redundant(page) {
			fmt.Fprintf(stderr, "%s: %s, which the mantra strip already says; wrap it in {{ if not (partial \"mantra-on.html\" .) }}\n", p, r)
			bad++
		}
		return nil
	})
	if err != nil {
		fmt.Fprintf(stderr, "landingkit check: %v\n", err)
		return 1
	}
	if bad > 0 {
		return 1
	}
	fmt.Fprintf(stdout, "check: %d page(s) with the mantra strip, nothing said twice\n", seen)
	return 0
}
