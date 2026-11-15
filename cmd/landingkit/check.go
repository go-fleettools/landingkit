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
			// A repeat is the strip's fact STANDING ALONE: a card's heading or a
			// footer's segment. A phrase inside a sentence that says something
			// else ("MFEM (BSD-3-Clause) is the design reference", "BSD-3-Clause
			// wrapper · GPL-2.0 engine") is a mention, not the strip said twice
			// (landingkit#46). The hero's sub-text keeps the substring rule.
			switch i {
			case 0:
				found = append(found, standaloneFacts(where[i], footerSegments(body), f)...)
			case 2:
				found = append(found, standaloneFacts(where[i], []string{text(headingOf(body))}, f)...)
			default:
				for _, fact := range f {
					if strings.Contains(t, strings.ToLower(fact)) {
						found = append(found, fmt.Sprintf("%s repeats %q", where[i], fact))
					}
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

// bandRuleRE matches the Digital Commons band's own rule in a page's <style>
// block, and bandElemRE the band itself. Both are the kit's exact output.
var (
	bandRuleRE = regexp.MustCompile(`\.commons-ribbon\{`)
	bandElemRE = regexp.MustCompile(classLiteral("commons-ribbon"))
	forkElemRE = regexp.MustCompile(classLiteral("fork-ribbon"))
)

// classLiteral matches `class="name"` whether or not the quotes survived.
//
// ⛔⛔ A PUBLISHED PAGE IS MINIFIED, AND THE FIRST VERSION OF THIS GATE PASSED ON
// THE VERY PAGE THAT SHIPPED THE DEFECT. Hugo's pipeline drops the quotes around
// an attribute value, so `class="fork-ribbon"` reaches a reader as
// `class=fork-ribbon`; the gate was written against the BUILT page in the
// repository, where the quotes are still there, and reported go-numerics.github.io
// clean while that page carried the band's stylesheet and no band.
//
// It is the second time today the same assumption cost a measurement: a chip
// check read `<code class="mantra-chip">` and found all four chips absent from a
// page that held them all. A check on published HTML anchors on what minification
// cannot touch.
func classLiteral(name string) string {
	return `class=(?:"` + name + `"|'` + name + `'|` + name + `(?:[\s>]|$))`
}

// bandStray reports the two ways a page's Digital Commons band can be wrong,
// and both have been real.
//
// ⛔⛔ (1) THE STYLESHEET WITHOUT THE BAND, which is what v0.28.0 shipped to the
// fleet. That release turned the band on "by default", and it reached only the
// layouts that call commons-ribbon.html — the kit's own example and almost
// nobody else. Most landings wrote their own layouts/index.html before the band
// existed and call fork-ribbon.html alone, so the pin bump gave them the rule
// and nothing to style: go-numerics.github.io served `commons-ribbon` once (the
// CSS) and "Digital Commons" not at all.
//
// Every CI case built the kit's example, the one layout that already did the
// right thing, so the whole suite was blind to it. This is the gate that is not
// blind: it reads the BUILT page, so it answers for whatever layout produced it.
//
// ⛔ (2) TWO BANDS, which the fix for (1) makes possible: fork-ribbon.html emits
// the band since v0.29.0, so a layout that also calls commons-ribbon.html gets
// it twice. A count is the check — presence cannot tell one from two.
//
// ⛔⛔ AND THE SUBJECT IS THE PAIR, NOT THE STYLESHEET. The first shape of this
// rule flagged "the rule is here and the band is not", and it immediately
// reported five of the kit's own pages — the carousel and hero demos, which have
// no fork ribbon either. It was right that they have the rule and no band, and
// wrong that this is a defect: styles.html emits ONE stylesheet for the whole
// site (`landingkit verify` compares it as text, so it cannot vary per page),
// while the band belongs to the pages that carry the fork ribbon. The two bands
// are a PAIR — same corner, same angle, one outside the other — so the invariant
// is the pairing:
//
//	a page with the fork ribbon has exactly one Commons band
//	no page has two
//
// That still catches what shipped: go-numerics.github.io has `class="fork-ribbon"`
// and no band. A rule shaped as "CSS without element" would have been noise on
// five pages and would have had to be switched off.
func bandStray(page string) []string {
	if !bandRuleRE.MatchString(page) {
		// The site wrote [params.commons] url = "": there is no band anywhere, and
		// the stylesheet says so. Nothing to pair.
		if n := len(bandElemRE.FindAllString(page, -1)); n > 0 {
			return []string{fmt.Sprintf("%d Digital Commons band(s) on a page whose stylesheet "+
				"has no .commons-ribbon rule: the band and the sheet disagree", n)}
		}
		return nil
	}
	n := len(bandElemRE.FindAllString(page, -1))
	switch {
	case n > 1:
		return []string{fmt.Sprintf("%d Digital Commons bands on one page: fork-ribbon.html emits "+
			"the band, so a layout that also calls commons-ribbon.html doubles it — drop the "+
			"explicit call", n)}
	case n == 0 && forkElemRE.MatchString(page):
		return []string{"the fork ribbon is here and the Digital Commons band is not: the two are " +
			"a pair, and fork-ribbon.html emits both since v0.29.0 — a layout holding a " +
			"hand-written fork ribbon should call the partial instead"}
	}
	return nil
}

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
	bad, seen, rows, footers, versioned := 0, 0, 0, 0, 0
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
		if footerRE.MatchString(page) {
			footers++
		}
		if kitVersion(page) != "" {
			versioned++
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
		for _, s := range bandStray(page) {
			fmt.Fprintf(stderr, "%s: %s\n", p, s)
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
	// A WARNING, never a failure: the version line is opt-in, so every landing
	// that bumps its pin starts without one, and a gate that failed them would
	// block the very bump that makes the line available. `audit` reports "none"
	// for the same pages. One warning per site, not per page: a site with a
	// footer on some pages and the line on one of them is adopting it.
	if footers > 0 && versioned == 0 {
		fmt.Fprint(stderr, "check: warning: no page carries the kit-version line; call {{ partial \"kit-version.html\" . }} in the footer to show which landingkit built it\n")
	}
	if bad > 0 {
		return 1
	}
	fmt.Fprintf(stdout, "check: %d page(s) with the mantra strip, nothing said twice; %d hero call-to-action row(s), rendered by the kit and in its order\n", seen, rows)
	return 0
}

var h3RE = regexp.MustCompile(`(?s)<h3\b[^>]*>(.*?)</h3>`)

// headingOf is a card's <h3> markup, or "" when it has none.
func headingOf(card string) string {
	if m := h3RE.FindStringSubmatch(card); m != nil {
		return m[1]
	}
	return ""
}

// footerSegments are the pieces of a footer set apart by an element boundary or a
// middle dot: "BSD-3-Clause · Pure Go" and `<span>BSD-3-Clause</span>` are two
// and one segment, "BSD-3-Clause wrapper · GPL-2.0 engine" is two.
func footerSegments(markup string) []string {
	s := html.UnescapeString(tagRE.ReplaceAllString(markup, "\x00"))
	s = strings.NewReplacer("\x00", "\x01", "·", "\x01", "|", "\x01").Replace(s)
	var out []string
	for _, seg := range strings.Split(s, "\x01") {
		if seg = strings.TrimSpace(spaceRE.ReplaceAllString(seg, " ")); seg != "" {
			out = append(out, seg)
		}
	}
	return out
}

// standaloneFacts reports a segment that is nothing but strip facts (and
// punctuation, "and", "&"): "BSD-3-Clause", "Pure Go", "Pure Go, CGO_ENABLED=0".
func standaloneFacts(where string, segs []string, facts []string) []string {
	var out []string
	var cw []string
	for _, fact := range facts {
		for _, w := range strings.FieldsFunc(strings.ToLower(fact), func(r rune) bool { return r == ':' || r == '=' || r == ' ' || r == ',' }) {
			cw = append(cw, w)
		}
	}
	chipWords := strings.Join(cw, " ")
	for _, seg := range segs {
		rest := strings.ToLower(seg)
		var hit []string
		for _, fact := range facts {
			if lf := strings.ToLower(fact); strings.Contains(rest, lf) {
				hit = append(hit, fact)
				rest = strings.ReplaceAll(rest, lf, " ")
			}
		}
		rest = strings.NewReplacer("cgo=0", " ", ",", " ", ".", " ", ";", " ", "&", " ", "-", " ", "—", " ", "–", " ", "/", " ").Replace(rest)
		// words a heading made of the fact is padded with ("No cgo", "CGO free")
		left := strings.Fields(rest)
		kept := left[:0]
		for _, w := range left {
			switch w {
			case "and", "no", "zero", "free", "cgo":
			default:
				// a word of the chip's own label ("fonts" of "fonts: OFL-1.1")
				if !strings.Contains(" "+chipWords+" ", " "+w+" ") {
					kept = append(kept, w)
				}
			}
		}
		if len(hit) == 0 || len(kept) != 0 {
			continue
		}
		for _, h := range hit {
			out = append(out, fmt.Sprintf("%s repeats %q", where, h))
		}
	}
	return out
}
