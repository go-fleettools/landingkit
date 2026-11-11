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
	// A feature card as the fleet writes it: <div class="card">…<h3>…</h3><p>…</p></div>
	// (v0.22.0: the "Pure Go, CGO=0" and "BSD-3-Clause" cards said the strip again).
	cardRE  = regexp.MustCompile(`(?s)<div\b[^>]*\bclass="?card\b[^>]*>(.*?</p>)\s*</div>`)
	tagRE   = regexp.MustCompile(`(?s)<[^>]*>`)
	spaceRE = regexp.MustCompile(`\s+`)
	valueRE = regexp.MustCompile(`[-.0-9]`)
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
		if strings.Contains(m[0], "mantra-goarch") {
			continue // an architecture, judged by archRepeats
		}
		out = append(out, html.UnescapeString(strings.TrimSpace(m[1])))
	}
	return out
}

// The strip's line of architectures (v0.22.0, [params.mantra] goarch). A
// region naming two or more of them says the line again; ONE name is not a
// repeat ("measured on an arm64 laptop" is a fact about a measurement).
var archChipRE = regexp.MustCompile(`<code\b[^>]*\bclass="?[^">]*\bmantra-goarch\b[^>]*>(.*?)</code>`)

func archs(page string) []string {
	var out []string
	for _, m := range archChipRE.FindAllStringSubmatch(page, -1) {
		out = append(out, strings.TrimSpace(m[1]))
	}
	return out
}

// archRepeats lists the architectures of the strip that a text names, when
// it names at least two of them, as whole words.
func archRepeats(t string, arch []string) []string {
	var found []string
	for _, a := range arch {
		if regexp.MustCompile(`(^|[^a-z0-9])` + regexp.QuoteMeta(a) + `($|[^a-z0-9])`).MatchString(t) {
			found = append(found, a)
		}
	}
	if len(found) < 2 {
		return nil
	}
	return found
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
	arch := archs(page)
	if len(f) == 0 && len(arch) == 0 {
		return nil
	}
	where := []string{"footer", "hero sub-text", "a feature card"}
	var found []string
	for i, m := range [][]string{
		firstOf(footerRE.FindAllStringSubmatch(page, -1)),
		firstOf(subRE.FindAllStringSubmatch(page, -1)),
		firstOf(cardRE.FindAllStringSubmatch(page, -1)),
	} {
		for _, body := range m {
			t := strings.ToLower(text(body))
			for _, fact := range f {
				if strings.Contains(t, strings.ToLower(fact)) {
					found = append(found, fmt.Sprintf("%s repeats %q", where[i], fact))
				}
			}
			if a := archRepeats(t, arch); a != nil {
				found = append(found, fmt.Sprintf("%s repeats the tested architectures %s", where[i], strings.Join(a, ", ")))
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

// `check` also fails a hero whose call-to-action row is out of the kit's order.
//
// The row is the `.cta` block under the title. The kit's order is the docs
// button first (`btn-primary`, the blue one), then the red `btn-try` ("Try it
// in your browser"), then the plain `btn-ghost` ones (README, "The hero's
// call-to-action row"; cta.html renders it in that order whatever order the
// site lists its buttons in). A landing that writes the row by hand can put
// them anywhere, and did: go-wasmcontainers' had the red button first. The
// check reads the BUILT page, so it judges a hand-written row and the
// partial's output alike.
var (
	ctaRowRE = regexp.MustCompile(`(?s)(<div\b[^>]*\bclass=(?:"[^"]*\bcta\b[^"]*"|cta\b)[^>]*>)(.*?)</div>`)
	ctaBtnRE = regexp.MustCompile(`<a\b[^>]*\bclass=(?:"([^"]*)"|([^\s>"]+))[^>]*>`)
)

// `check` also fails a hero row the kit did not render (v0.21.0).
//
// The order alone did not keep the landings alike. go-numerics and go-ndarray
// wrote their rows by hand, in the right order, and both put "Try it in the
// browser" in a ghost button where the kit has the red btn-try, one of them
// without the icon. Only cta.html renders the row the same way everywhere, so
// a .cta row must carry the attribute cta.html writes, data-kit-cta. In any
// row, kit's or not, a link to a playground must be the red btn-try, and a
// btn-primary or btn-try button must carry its icon (an <svg>).
var (
	ctaKitRE  = regexp.MustCompile(`\sdata-kit-cta\b`)
	ctaLinkRE = regexp.MustCompile(`(?s)<a\b([^>]*)>(.*?)</a>`)
	hrefRE    = regexp.MustCompile(`\bhref=(?:"([^"]*)"|([^\s>"]+))`)
	classRE   = regexp.MustCompile(`\bclass=(?:"([^"]*)"|([^\s>"]+))`)
)

// ctaStray lists what makes a page's hero rows differ from the kit's: a row
// written by hand, a playground link that is not the red button, a kit button
// without its icon.
func ctaStray(page string) []string {
	var out []string
	for _, row := range ctaRowRE.FindAllStringSubmatch(page, -1) {
		if !ctaKitRE.MatchString(row[1]) {
			out = append(out, "a hero call-to-action row written by hand (no data-kit-cta)")
		}
		for _, a := range ctaLinkRE.FindAllStringSubmatch(row[2], -1) {
			href, classes := attr(hrefRE, a[1]), " "+attr(classRE, a[1])+" "
			label := text(a[2])
			if strings.Contains(href, "playground") && !strings.Contains(classes, " btn-try ") {
				out = append(out, fmt.Sprintf("the playground link %q is not the red btn-try button", label))
			}
			for _, kind := range []string{"btn-primary", "btn-try"} {
				if strings.Contains(classes, " "+kind+" ") && !strings.Contains(a[2], "<svg") {
					out = append(out, fmt.Sprintf("the %s button %q has no icon", kind, label))
				}
			}
		}
	}
	return out
}

func attr(re *regexp.Regexp, tag string) string {
	m := re.FindStringSubmatch(tag)
	if m == nil {
		return ""
	}
	return m[1] + m[2]
}

// ctaRank is a button's place in the row, from its classes; -1 for a link the
// order says nothing about.
func ctaRank(classes string) (int, string) {
	for _, f := range strings.Fields(classes) {
		switch f {
		case "btn-primary":
			return 0, f
		case "btn-try":
			return 1, f
		case "btn-ghost":
			return 2, f
		}
	}
	return -1, ""
}

// ctaDisorder lists the buttons of a page's hero rows that come after one the
// kit puts later, and counts the rows it read.
func ctaDisorder(page string) (disorder []string, rows int) {
	for _, row := range ctaRowRE.FindAllStringSubmatch(page, -1) {
		rows++
		last, lastName := -1, ""
		for _, b := range ctaBtnRE.FindAllStringSubmatch(row[2], -1) {
			rank, name := ctaRank(b[1] + " " + b[2])
			switch {
			case rank < 0:
			case rank < last:
				disorder = append(disorder, fmt.Sprintf("hero call-to-action %q comes after %q", name, lastName))
			default:
				last, lastName = rank, name
			}
		}
	}
	return disorder, rows
}

func cmdCheck(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprint(stderr, "usage: landingkit check <built-site-dir>\n")
		return 2
	}
	bad, seen, rows := 0, 0, 0
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
		order, n := ctaDisorder(page)
		rows += n
		for _, o := range order {
			fmt.Fprintf(stderr, "%s: %s; the kit's order is docs (btn-primary), try (btn-try), then the rest: list them in [[params.cta]] and render the row with {{ partial \"cta.html\" . }}\n", p, o)
			bad++
		}
		for _, s := range ctaStray(page) {
			fmt.Fprintf(stderr, "%s: %s; list the buttons in [[params.cta]] (kind = docs, try, github or link) and render the row with {{ partial \"cta.html\" . }}, plus {{ partial \"btn-try-style.html\" . }} after styles.html for a try button\n", p, s)
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
	fmt.Fprintf(stdout, "check: %d page(s) with the mantra strip, nothing said twice; %d hero call-to-action row(s), rendered by the kit and in its order\n", seen, rows)
	return 0
}
