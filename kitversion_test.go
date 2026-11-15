package landingkit

import (
	"strings"
	"testing"
)

// The kit-version partial reads the module graph Hugo resolved and must never
// trust the Version of a REPLACED module (the example's go.mod says v0.14.0
// while building the checkout). The built pages are checked by CI's hugo lane
// (a tagged landing and the example, which is a replace); this pins the
// template's decisions, and its colour, which is the sheet's --slate and not
// the footer's --muted-2.
func TestKitVersionPartial(t *testing.T) {
	p := contract(t, "layouts/partials/kit-version.html", []string{
		`range hugo.Deps`,
		`eq .Path "github.com/go-fleettools/landingkit"`,
		`(not .Replace)`, // a replace: the Version is the replaced one's
		`(ne .Version "")`,
		`"landingkit (local)"`,
		`releases/tag/%s`,
		`landingkit/commit/%s`, // a pseudo-version has no tag
		"[-.][0-9]{14}-[0-9a-f]{12}$",
		`class="kit-version"`,
		`color:var(--slate)`,
	})
	// Opt-in and inline: styles.html is compared as text by `landingkit verify`,
	// so the line must not add a rule to it.
	s := contract(t, "layouts/partials/styles.html", nil)
	if strings.Contains(s, "kit-version") {
		t.Error("styles.html mentions kit-version: a landing that does not call the partial would get a different <style> block")
	}
	if strings.Contains(p, "muted-2") && strings.Contains(p, "color:var(--muted-2)") {
		t.Error("kit-version.html uses --muted-2, 3.11:1 on the light background")
	}
}
