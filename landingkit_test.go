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
	if seen < 6 {
		t.Fatalf("found %d partials, want the 6 the kit ships", seen)
	}
}

// The loading carousel keeps its contract: the WAI-ARIA APG carousel
// pattern, no rotation under reduced motion, shown only while the large
// download is still going (a cached load never sees it), and the page's
// API. Its behaviour is driven in Chrome by the playgrounds that use it
// (go-slides.github.io's browsertest/tour.cjs).
func TestLoadingCarousel(t *testing.T) {
	b, err := os.ReadFile("layouts/partials/loading-carousel.html")
	if err != nil {
		t.Fatal(err)
	}
	p := string(b)
	for _, want := range []string{
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
	} {
		if !strings.Contains(p, want) {
			t.Errorf("loading-carousel.html lacks %q", want)
		}
	}
	// The rotation button is the region's first control.
	if strings.Index(p, `class="lk-car-rot"`) > strings.Index(p, `class="lk-car-slides"`) {
		t.Error("the rotation button is not the first element of the region")
	}
}
