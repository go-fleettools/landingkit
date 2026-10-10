package landingkit

import (
	"os"
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
