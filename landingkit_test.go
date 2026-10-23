package landingkit

import (
	"io/fs"
	"os"
	"path/filepath"
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
		`role="region" aria-roledescription="carousel"`, // the region
		`aria-roledescription="slide"`,                  // labelled slides
		`aria-label="Stop slide rotation"`,              // the rotation button, first in the region
		`aria-live="off"`, `"polite"`,                   // live only when stopped
		`aria-label="Choose slide to display"`, // the dots' group
		`aria-disabled`,                        // the current dot stays focusable
		`prefers-reduced-motion: reduce`,       // no rotation …
		`prefers-reduced-motion:reduce`,        // … and no animation
		`"focusin"`, `"mouseenter"`,            // focus stops, hover pauses
		`if(st.done||st.downloaded)return;`, // the cached-load skip
		`window.landingkitCarousel=api`,     // the page's API
		`downloaded:function`, `api.done=function(reveal,onWait)`,
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
		`aria-label="Stop slide rotation"`,
		`aria-live="off"`, `"polite"`,
		`aria-label="Choose slide to display"`,
		`aria-disabled`,
		`"ArrowLeft"`, `"ArrowRight"`, // keyboard
		`"focusin"`, `"mouseenter"`,
		`prefers-reduced-motion: reduce`, `prefers-reduced-motion:reduce`,
		`grid-area:1/1`,                    // every card in one cell: the hero's size from the first paint
		`data-src`, `fetchpriority="high"`, // later pictures wait; the first may be the LCP
		`addEventListener("load"`, // the next one after the page has loaded
		`aria-hidden="true"`,      // hidden cards are hidden from assistive technology
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

// The mantra: on by default with the fleet's two values, off by
// [params.mantra] show = false, by a page's `mantra: false`, and on the
// full-viewport playground layout; at most four items (a data licence such
// as `fonts: OFL-1.1` beside the code's), of a plain charset (they become
// CSS strings). CI checks the chips reach the example's page, that four
// reach a page built with four, and that, turned off, the page is byte for
// byte the previous release's.
func TestMantra(t *testing.T) {
	contract(t, "layouts/partials/styles.html", []string{
		`slice "LICENSE=BSD-3-Clause" "CGO_ENABLED=0"`,
		`isset $mt "show"`,
		`if eq .Params.mantra false`,
		`if eq .Layout "playground"`,
		`errorf "landingkit: [params.mantra] items holds at most four entries`,
		"findRE `^[A-Za-z0-9_.=:+/, -]{1,40}$`",
		`jsonify | safeCSS`,
		`.nav{padding-bottom:34px}`,
		`.nav>.wrap::before{content:`, `.nav>.wrap::after{content:`, // the third and fourth chips
		`.nav{padding-bottom:{{ add 34 (mul 29 $extra) }}px}`, // a row per data chip on a phone
	})
}
