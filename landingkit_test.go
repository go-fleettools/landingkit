package landingkit

import (
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The embedded sheet is the file Hugo mounts, not a copy of it.
func TestStylesIsThePartial(t *testing.T) {
	b, err := os.ReadFile("layouts/partials/styles.html")
	if err != nil {
		t.Fatal(err)
	}
	if Styles != string(b) {
		t.Fatal("Styles differs from layouts/partials/styles.html")
	}
	if len(Styles) == 0 {
		t.Fatal("Styles is empty")
	}
}

// No partial ends with a newline. A partial is pasted where it is called, and
// its final newline comes with it: icons/github.html had one and put a
// whitespace text node between the <svg> and the button text, so a landing's
// built page differed by a byte from the copy it replaced, which had none
// (icons/book.html never had one). Harmless inside .btn, an inline-flex with a
// gap, but a byte-for-byte comparison of two builds is how a migration is
// proved, so the rule is stated once, for every partial, and checked here.
func TestNoPartialEndsWithANewline(t *testing.T) {
	var seen int
	err := filepath.WalkDir("layouts/partials", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		seen++
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.HasSuffix(string(b), "\n") {
			t.Errorf("%s ends with a newline", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// ⛔ A walk that found nothing would pass every assertion above.
	if seen < 7 {
		t.Fatalf("found %d partials, want the 7 the kit ships", seen)
	}
}

// ⛔ THE TWO OPT-IN ADDITIONS MUST BE SILENT FOR EVERY SITE THAT DID NOT ASK.
// Twelve organisations build this kit; a landing that sets neither
// [params.commons] nor [[params.siblings]] has to render the page and the
// stylesheet it rendered before they existed. CI proves it by building the
// example against the base branch and comparing bytes; this says the same
// thing one layer down, where it is cheap and runs on every change.
func TestTheNewPartialsAreOptIn(t *testing.T) {
	// The ribbon writes nothing without the param, and the menu falls back to
	// the exact anchor the layouts used to write by hand.
	contract(t, "layouts/partials/commons-ribbon.html", []string{
		`{{- with $url }}`, // nothing at all when it is empty
		`class="commons-ribbon"`,
		`{{ i18n "kit_digitalCommons" | safeHTML }}`,
	})
	contract(t, "layouts/partials/brand-menu.html", []string{
		`{{- if $sibs -}}`,
		`<a class="brand" href="/">{{ $mark }}{{ $ctx.Site.Title }}</a>`,
		`<details class="brand-menu">`,
		`{{ i18n "kit_siblingSites" | safeHTML }}`,
	})

	// ⛔⛔ AND IT MUST BE ABLE TO CARRY THE BAR'S LOGO. v0.23.0 shipped this
	// partial writing the title and nothing else, because the example's bar is
	// a bare title and that looked finished. The first real landing to adopt it
	// has an <img class="glyph"> in front of the name, and adopting the menu
	// would have dropped the logo off its top bar.
	//
	// The sheet has styled `.glyph` since the beginning while every landing
	// wrote the <img> by hand: the gap was between what the kit styles and what
	// any partial of it could emit.
	contract(t, "layouts/partials/brand-menu.html", []string{
		`{{- if reflect.IsMap . }}`, // a page context still works: v0.23.0's callers
		`class=\"glyph\" src=\"%s\" width=\"32\" height=\"32\"`,
		`($glyphAlt | htmlEscape)`, // the alt text is a site's, so it is escaped
	})
	g := contract(t, "layouts/partials/styles.html", []string{`.glyph{width:32px;height:32px;`})
	// ⚠ THE PARTIAL'S 32 IS THE SHEET'S 32. Written in two places, they can
	// drift; the attributes exist so the bar does not reflow while the logo
	// loads, and a width that disagreed with the rule would do exactly that.
	if !strings.Contains(g, "width:32px;height:32px") {
		t.Error("the sheet no longer sizes .glyph at 32px: brand-menu.html still writes 32")
	}

	// ⛔ AND THE SHEET'S NEW RULES ARE GUARDED BY THE SAME PARAMS. A rule
	// emitted unconditionally would move every organisation's stylesheet, and
	// `landingkit verify` compares the <style> block as text: every landing
	// would fail against the reference until it rebuilt.
	s := contract(t, "layouts/partials/styles.html", []string{
		`{{- with (.Site.Params.commons | default dict).url }}`,
		`.commons-ribbon{position:absolute;top:0;right:0;`,
		`{{- with .Site.Params.siblings }}`,
		`.brand-menu{position:relative}`,
	})
	// Each guarded block must CLOSE before the mantra's, or the mantra would
	// be emitted only for a site that set these params.
	for _, rule := range []string{".commons-ribbon{", ".brand-menu{", ".brand-caret{"} {
		i := strings.Index(s, rule)
		if i < 0 {
			t.Fatalf("%s is not in the sheet", rule)
		}
		if !strings.Contains(s[:i], "{{- with ") {
			t.Errorf("%s is emitted with no `with` guard before it", rule)
		}
	}
	// ⛔ The second band is PARALLEL to the first: same corner, same rotation.
	// A different angle is the one thing that would make them read as two
	// unrelated decorations, and it is a single character to get wrong.
	for _, want := range []string{
		".fork-ribbon a{position:absolute;top:48px;right:-36px;",
		".commons-ribbon a{position:absolute;top:108px;right:-36px;",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("the sheet lacks %q", want)
		}
	}
	if strings.Count(s, "transform:rotate(45deg)") != 2 {
		t.Errorf("the two bands do not both rotate 45deg: %d occurrence(s)",
			strings.Count(s, "transform:rotate(45deg)"))
	}
	assertTheBandIsClipped(t, s)
}

// ⛔⛔ A CORNER RIBBON IS ONLY A RIBBON BECAUSE THE BOX CLIPS IT, and v0.23.0's
// second band was too short to reach the edge: BOTH ITS ENDS STOPPED IN
// MID-AIR, which reads as a floating label rather than a band. It was 260px.
//
// `transform:rotate` turns the element about its CENTRE, so a horizontal band
// of length L reaches only L/2·cos45 ≈ 0.354·L each way — measured before it
// turns, which is the trap. With the centre 136px from the box's left edge,
// 260px reached 92px and fell 44px short.
//
// This checks the LEFT end, which is the one that must leave the box
// horizontally. The right end leaves through the BOTTOM edge, which depends on
// the band's rendered height and so is not computed here; the screenshot is
// what confirms that one.
//
// ⚠ IT IS COMPUTED FROM THE SHEET'S OWN NUMBERS, not compared with 400. A test
// that asserted the literal would pass for a box that had been resized under
// it, which is exactly the drift this guards.
func assertTheBandIsClipped(t *testing.T, sheet string) {
	t.Helper()
	num := func(re, what string) float64 {
		m := regexp.MustCompile(re).FindStringSubmatch(sheet)
		if m == nil {
			t.Fatalf("the sheet does not state the band's %s (%s)", what, re)
		}
		v, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		return v
	}
	boxW := num(`\.commons-ribbon\{position:absolute;top:0;right:0;width:(\d+)px`, "box width")
	bandW := num(`\.commons-ribbon a\{position:absolute;top:\d+px;right:-\d+px;width:(\d+)px`, "band width")
	rightOff := num(`\.commons-ribbon a\{position:absolute;top:\d+px;right:-(\d+)px`, "right offset")

	// the band's centre, in box coordinates: its right edge sits rightOff past
	// the box's right edge
	centreX := boxW + rightOff - bandW/2
	reach := bandW / 2 * math.Cos(math.Pi/4)
	if leftEnd := centreX - reach; leftEnd >= 0 {
		t.Errorf("the band's left end stops %0.f px INSIDE the box (box %0.f, band %0.f, "+
			"right -%0.f): it must be clipped by the edge, not float. A band of %0.f px "+
			"would reach it.",
			leftEnd, boxW, bandW, rightOff, (boxW+rightOff)/(0.5+math.Cos(math.Pi/4)/2))
	}
}

// contract checks that a partial still says what its README promises.
func contract(t *testing.T, file string, wants []string) string {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	p := string(b)
	for _, want := range wants {
		if !strings.Contains(p, want) {
			t.Errorf("%s lacks %q", file, want)
		}
	}
	return p
}

// The loading carousel keeps its contract: the WAI-ARIA APG carousel
// pattern, no rotation under reduced motion, shown only while the large
// download is still going (a cached load never sees it), and the page's
// API. Its behaviour is driven in Chrome by the playgrounds that use it
// (go-slides.github.io's browsertest/tour.cjs).
func TestLoadingCarousel(t *testing.T) {
	p := contract(t, "layouts/partials/loading-carousel.html", []string{
		`role="region" aria-roledescription="carousel"`,         // the region
		`aria-roledescription="slide"`,                          // labelled slides
		`aria-label="{{ i18n "kit_stopRotation" | safeHTML }}"`, // the rotation button, first in the region
		`aria-live="off"`, `"polite"`, // live only when stopped
		`aria-label="{{ i18n "kit_chooseSlide" | safeHTML }}"`, // the dots' group
		`aria-disabled`,                  // the current dot stays focusable
		`prefers-reduced-motion: reduce`, // no rotation …
		`prefers-reduced-motion:reduce`,  // … and no animation
		`"focusin"`, `"mouseenter"`,      // focus stops, hover pauses
		`if(st.done||st.downloaded)return;`, // the cached-load skip
		`window.landingkitCarousel=api`,     // the page's API
		`downloaded:function`, `api.done=function(reveal,onWait)`,
		// signal mode: the delay counts from downloading(), never from the
		// script, so a cached module or a slow runtime cannot flash the carousel
		`api.downloading=function(){st.downloading=true;if(SIGNAL)arm();};`, `if(!SIGNAL)arm();`,
		`{{ if $.signal }} data-signal="1"{{ end }}`,
		`loading="lazy"`, `im.loading="eager"`, // nothing fetched unless shown; the next one preloaded
	})
	// The rotation button is the region's first control.
	if strings.Index(p, `class="lk-car-rot"`) > strings.Index(p, `class="lk-car-slides"`) {
		t.Error("the rotation button is not the first element of the region")
	}
}

// The hero carousel keeps its contract. With no cards it is the landing's
// hero-terminal.html and nothing else (CI compares two built pages byte for
// byte); with cards it is an APG carousel whose cards share one grid cell
// (no layout shift), whose later pictures wait in data-src, and which
// neither rotates nor animates under reduced motion. Its behaviour is driven
// in Chrome on go-slides.github.io (browsertest/hero.cjs).
func TestHeroCarousel(t *testing.T) {
	p := contract(t, "layouts/partials/hero-carousel.html", []string{
		"{{- if not $cards -}}\n{{ partial \"hero-terminal.html\" . }}\n{{- else -}}", // no cards: the terminal alone
		`role="region" aria-roledescription="carousel"`,
		`aria-roledescription="slide"`,
		`aria-label="{{ i18n "kit_stopRotation" | safeHTML }}"`,
		`aria-live="off"`, `"polite"`,
		`aria-label="{{ i18n "kit_chooseSlide" | safeHTML }}"`,
		`aria-disabled`,
		`"ArrowLeft"`, `"ArrowRight"`, // keyboard
		`"focusin"`, `"mouseenter"`,
		`prefers-reduced-motion: reduce`, `prefers-reduced-motion:reduce`,
		`grid-area:1/1`,                    // every card in one cell: the hero's size from the first paint
		`data-src`, `fetchpriority="high"`, // later pictures wait; the first may be the LCP
		`addEventListener("load"`, // the next one after the page has loaded
		`aria-hidden="true"`,      // hidden cards are hidden from assistive technology
		// terminal = "name" is hero-terminal-name.html, the name a plain word
		"{{ $term = printf \"hero-terminal-%s.html\" $c.terminal }}",
		"findRE `^[a-z0-9]+(-[a-z0-9]+)*$` $c.terminal",
	})
	if strings.Index(p, `class="hero-car-rot"`) > strings.Index(p, `class="hero-car-slides"`) {
		t.Error("the rotation button is not the first element of the region")
	}
	// The sheet precedes the markup: a page that arrives in pieces must not
	// paint the cards unstyled, stacked, and then collapse them (a shift).
	if st, sec := strings.Index(p, "<style>"), strings.Index(p, "<section"); st < 0 || sec < 0 || st > sec {
		t.Error("the carousel's <style> does not come before its <section>")
	}
}

// The hero's row keeps its contract: the kit's order is written ONCE, in the
// range, and docs comes before try, which comes before the rest; a bad kind or
// a button without words stops the build instead of rendering a wrong row.
func TestCtaRowOrder(t *testing.T) {
	p := contract(t, "layouts/partials/cta.html", []string{
		`range $kind := slice "docs" "try" "github" "link"`, // the order
		`errorf "cta: unknown kind`, `errorf "cta %q: href and label are both required"`,
		`btn btn-primary`, `btn btn-try`, `btn btn-ghost`,
	})
	if strings.Index(p, `btn btn-primary" href`) > strings.Index(p, `btn btn-try" href`) {
		t.Error("cta.html renders the red button before the docs button")
	}
}

// The strip says the licence and CGO_ENABLED=0; nothing else the kit renders
// says them again. Only styles.html (the strip) and mantra-on.html (the
// question it asks) may hold them: a partial that printed them would repeat
// the strip on every page, and one that needs to must ask mantra-on.html.
func TestNothingElseRepeatsTheMantra(t *testing.T) {
	var seen int
	err := filepath.WalkDir("layouts/partials", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		seen++
		base := filepath.Base(path)
		if base == "styles.html" || base == "mantra-on.html" || base == "mantra-items.html" || base == "mantra.html" {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, w := range []string{"BSD-3", "BSD 3", "CGO_ENABLED", "cgo", "no cgo"} {
			if strings.Contains(strings.ToLower(string(b)), strings.ToLower(w)) && !strings.Contains(string(b), `partial "mantra-on.html"`) {
				t.Errorf("%s says %q without asking mantra-on.html", path, w)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen < 8 {
		t.Fatalf("walked %d partials", seen)
	}
}

// The mantra: on by default with the fleet's two values, off by
// [params.mantra] show = false, by a page's `mantra: false`, and on the
// full-viewport playground layout; at most four items (a data licence such
// as `fonts: OFL-1.1` beside the code's), of a plain charset. Since v0.7.0
// the chips are TEXT: mantra.html renders them from theme-toggle.html, which
// every landing calls in its nav bar, and styles.html only styles them. No
// `content:` may carry them again: CSS generated content looked right in a
// screenshot and was not text (wave 3 shipped 28 pages where a text check
// found neither value). CI checks the chips are text nodes of the example's
// page, that four are with four, and that, turned off, nothing is rendered.
func TestMantra(t *testing.T) {
	contract(t, "layouts/partials/mantra-items.html", []string{
		`slice "LICENSE=BSD-3-Clause" "CGO_ENABLED=0"`,
		`errorf "landingkit: [params.mantra] items holds at most four entries`,
		"findRE `^[A-Za-z0-9_.=:+/, -]{1,40}$`",
		`return $items`,
	})
	contract(t, "layouts/partials/mantra-on.html", []string{
		`isset $mt "show"`,
		`if eq .Params.mantra false`,
		`if eq .Layout "playground"`,
		`partial "mantra-items.html" .`,
		`return (and $show (gt (len $items) 0))`,
	})
	contract(t, "layouts/partials/mantra.html", []string{
		`if partial "mantra-on.html" .`,
		`<div class="mantra" role="note"`,
		`<code class="mantra-chip">{{ . }}</code>`, // text, escaped by Hugo
		`range first 2 $items`, `range after 2 $items`,
	})
	contract(t, "layouts/partials/mantra-goarch.html", []string{
		`isset $mt "goarch"`,
		`errorf "landingkit: [params.mantra] goarch %q is not a GOARCH Go knows`,
		`errorf "landingkit: [params.mantra] goarch %q is listed twice"`,
		`return $arch`,
	})
	contract(t, "layouts/partials/mantra.html", []string{
		`$arch := partial "mantra-goarch.html" .`,
		`{{ if $arch }}<span class="mantra-arch">`, // nothing at all without goarch
		`{{ i18n "kit_goarch" }}`,
		`<code class="mantra-chip mantra-goarch">{{ . }}</code>`,
	})
	contract(t, "layouts/partials/theme-toggle.html", []string{`{{ partial "mantra.html" . }}`})
	s := contract(t, "layouts/partials/styles.html", []string{
		`$show := partial "mantra-on.html" .`, // one answer, shared with the landings
		`.nav{padding-bottom:34px}`,
		`.mantra{position:absolute;top:69px;`,
		`.nav{padding-bottom:{{ add 34 (mul 29 $extra) }}px}`, // a row per data chip on a phone
	})
	if strings.Contains(s, "::before{content:") || strings.Contains(s, "::after{content:") || strings.Contains(s, "jsonify | safeCSS") {
		t.Error("the strip is CSS generated content again: it must be text")
	}
}
