package main

import (
	"strings"
	"testing"
)

// ⛔⛔ THE GATE FOR THE DEFECT v0.28.0 SHIPPED TO THE FLEET.
//
// That release turned the Digital Commons band on "by default" and it reached
// only the layouts that call commons-ribbon.html — the kit's own example, and
// almost nobody else. ~320 landings wrote their own layouts/index.html before
// the band existed and call fork-ribbon.html alone, so the pin bump gave them
// the band's stylesheet rule and no band. Measured on the first one bumped:
// go-numerics.github.io served `commons-ribbon` once (the CSS) and
// "Digital Commons" not at all.
//
// Every CI case built the kit's example — the one layout that already did the
// right thing — so the suite could not see it. This gate reads the BUILT page,
// so it answers for whatever layout produced it.
//
// ⚠ THE RULE IS THE PAIR, NOT "CSS WITHOUT ELEMENT". The first shape of it
// flagged five of the kit's own pages (the carousel and hero demos), which have
// the site-wide stylesheet and no fork ribbon either — correct about the facts
// and wrong about the defect. styles.html emits ONE sheet for the whole site
// (`landingkit verify` compares it as text, so it cannot vary per page) while
// the band belongs to the pages carrying the fork ribbon, which it is a pair
// with.
func TestBandStray(t *testing.T) {
	const rule = `<style>.commons-ribbon{position:absolute;top:0;right:0}</style>`
	for _, c := range []struct {
		name, page, want string
	}{
		{
			// The shipped defect, in the spelling a reader actually gets.
			name: "fork ribbon, no band, minified",
			page: `<html><head>` + rule + `</head><body><div class=fork-ribbon><a href=x>Fork me</a></div></body></html>`,
			want: "the fork ribbon is here and the Digital Commons band is not",
		},
		{
			// The same, with the quotes the repository's own build keeps.
			name: "fork ribbon, no band, quoted",
			page: `<html><head>` + rule + `</head><body><div class="fork-ribbon"><a href="x">Fork me</a></div></body></html>`,
			want: "the fork ribbon is here and the Digital Commons band is not",
		},
		{
			name: "the pair, which is what the kit emits",
			page: `<html><head>` + rule + `</head><body><div class=fork-ribbon><a href=x>Fork me</a></div>` +
				`<div class=commons-ribbon><a href=y>Digital Commons</a></div></body></html>`,
			want: "",
		},
		{
			// What fork-ribbon.html emitting the band makes possible.
			name: "two bands",
			page: `<html><head>` + rule + `</head><body><div class=fork-ribbon><a href=x>F</a></div>` +
				`<div class=commons-ribbon><a href=y>D</a></div>` +
				`<div class=commons-ribbon><a href=y>D</a></div></body></html>`,
			want: "2 Digital Commons bands on one page",
		},
		{
			// A page of the site with no fork ribbon: the sheet is site-wide and
			// says nothing about this page. Not a defect — this is the case that
			// made the first shape of the rule unusable.
			name: "no fork ribbon, no band, sheet present",
			page: `<html><head>` + rule + `</head><body><main>docs</main></body></html>`,
			want: "",
		},
		{
			// [params.commons] url = "": no rule anywhere, no band anywhere.
			name: "opted out",
			page: `<html><head><style>.nav{padding-bottom:34px}</style></head><body><div class=fork-ribbon><a href=x>F</a></div></body></html>`,
			want: "",
		},
		{
			// The sheet and the markup disagreeing the other way round, which
			// means one of the two partials stopped asking commons-url.html.
			name: "a band with no rule",
			page: `<html><head><style>.nav{padding-bottom:34px}</style></head><body><div class=commons-ribbon><a href=y>D</a></div></body></html>`,
			want: "band(s) on a page whose stylesheet has no .commons-ribbon rule",
		},
	} {
		got := bandStray(c.page)
		switch {
		case c.want == "" && len(got) > 0:
			t.Errorf("%s: reported %q, want nothing", c.name, got)
		case c.want != "" && len(got) == 0:
			t.Errorf("%s: reported nothing, want %q", c.name, c.want)
		case c.want != "" && !strings.Contains(got[0], c.want):
			t.Errorf("%s: reported %q, want it to contain %q", c.name, got[0], c.want)
		}
	}
}

// ⛔ AND IT REACHES THE COMMAND, not only the function: a rule nothing calls is
// a rule that does not run. `check` must exit 1 and name the page.
func TestCheckRefusesTheBandlessPair(t *testing.T) {
	const rule = `<style>.commons-ribbon{position:absolute;top:0;right:0}</style>`
	d := checkSite(t, `<html><head>`+rule+`</head><body><div class=fork-ribbon><a href=x>Fork me</a></div></body></html>`)
	code, msg := runCheck(t, d)
	if code != 1 {
		t.Errorf("exit %d, want 1 — the page carries the sheet and no band: %s", code, msg)
	}
	if !strings.Contains(msg, "the fork ribbon is here and the Digital Commons band is not") {
		t.Errorf("message %q does not name the defect", msg)
	}
}
