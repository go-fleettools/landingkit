package main

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

// The kit-version line (layouts/partials/kit-version.html) is the one place a
// built page says which landingkit it was built with. check warns when a site
// has none; audit reads it from every published page, so drift across the
// fleet's pins is visible without cloning anything.
//
// The pattern accepts both the quoted and the minified form of the class
// attribute, an optional link around the text, and any label the partial
// prints: "v0.36.0", a pseudo-version, or "(local)".

var kitVersionRE = regexp.MustCompile(`<span\b[^>]*\bclass=["']?kit-version\b[^>]*>(?:\s*<a\b[^>]*>)?\s*landingkit\s+([^<\s][^<]*?)\s*(?:</a>\s*)?</span>`)

// kitVersion is what the page's kit-version line says after "landingkit ", or
// "" when the page has no such line.
func kitVersion(page string) string {
	if m := kitVersionRE.FindStringSubmatch(page); m != nil {
		return m[1]
	}
	return ""
}

// kitTally prints "kit versions: v0.36.0 x2, none x1", most recent label last,
// so a run over many pages ends with the drift in one line.
func kitTally(w io.Writer, counts map[string]int) {
	labels := make([]string, 0, len(counts))
	for l := range counts {
		labels = append(labels, l)
	}
	sort.Strings(labels)
	parts := make([]string, len(labels))
	for i, l := range labels {
		parts[i] = fmt.Sprintf("%s x%d", l, counts[l])
	}
	fmt.Fprintf(w, "kit versions: %s\n", strings.Join(parts, ", "))
}
