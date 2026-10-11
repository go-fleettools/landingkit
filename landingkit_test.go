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
	if seen < 5 {
		t.Fatalf("found %d partials, want the 5 the kit ships", seen)
	}
}
