package main

import (
	"fmt"
	"io"
	"regexp"
	"strings"
)

// ⛔ A LANDING THAT IS NOT ON THE KIT STILL HAS TEXT ON A BACKGROUND. `brand`
// measures the two pairs the kit itself uses — the stops against white and
// against #0b0e14 — and can say nothing about a page that chose its own
// background, which is every hand-made landing in the fleet. Those are the
// pages nobody can check, so those are the pages that drift.
//
// The maths is the one `brand` and the sheet's tests already use, exported as a
// command rather than copied: a second implementation is a second answer, and
// the fleet has been bitten by recomputing a contrast by hand and getting 3.61
// where the number was 3.77.

var hexColor = regexp.MustCompile(`^#?[0-9A-Fa-f]{6}$`)

// cmdContrast prints the contrast ratio of each foreground against one
// background, and says which clear the bar.
func cmdContrast(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprint(stderr, "usage: landingkit contrast <background> <foreground>...\n")
		return 2
	}
	for _, a := range args {
		if !hexColor.MatchString(a) {
			fmt.Fprintf(stderr, "landingkit: %q is not a #rrggbb colour\n", a)
			return 2
		}
	}
	// The hash is optional on input and always printed, so that two spellings
	// of one colour read alike in a table somebody is comparing by eye.
	norm := func(s string) string { return "#" + strings.ToLower(strings.TrimPrefix(s, "#")) }
	bg := hexRGB(norm(args[0]))
	worst := 0
	for _, a := range args[1:] {
		c := contrast(hexRGB(norm(a)), bg)
		verdict := "under"
		if c >= minContrast {
			verdict = "AA"
		} else {
			worst = 1
		}
		// 3:1 is the bar for large-scale text and for non-text, so a value
		// between the two is said rather than left to be looked up.
		if c < minContrast && c >= largeContrast {
			verdict = "under (large text only)"
		}
		fmt.Fprintf(stdout, "%s on %s  %.2f:1  %s\n", norm(a), norm(args[0]), c, verdict)
	}
	return worst
}

// largeContrast is WCAG 2.2 SC 1.4.3's other bar: 3:1 for text at 18pt, or
// 14pt bold (18.66px). Nothing the kit paints with an accent is that large,
// which is why minContrast is what brand.go fits to — but a hand-made landing
// may well be asking about a heading.
const largeContrast = 3.0
