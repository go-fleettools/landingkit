package main

import (
	"bytes"
	"strings"
	"testing"
)

// The anchors are WCAG's own published examples, so a reader can check the
// command against the specification rather than against this test.
func TestContrastCommandAgreesWithTheWCAGAnchors(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"contrast", "#ffffff", "#000000", "#767676", "#ffffff"}, &out, &errb); code != 1 {
		t.Fatalf("exit %d, want 1 (the third is under the bar): %s%s", code, out.String(), errb.String())
	}
	for _, want := range []string{
		"#000000 on #ffffff  21.00:1  AA",
		"#767676 on #ffffff  4.54:1  AA",
		"#ffffff on #ffffff  1.00:1  under",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q from\n%s", want, out.String())
		}
	}
}

// The two defects this command exists to have caught, as the fleet's own
// colours — and the band between the bars, which is said rather than left to
// be looked up.
func TestContrastCommandOnTheAccentsThatFailed(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"contrast", "#0b0e14", "#4f46e5", "#726bea"}, &out, &errb); code != 1 {
		t.Fatalf("exit %d, want 1: %s%s", code, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), "#4f46e5 on #0b0e14  3.07:1  under (large text only)") {
		t.Errorf("the document family's indigo is not reported as it was measured:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "#726bea on #0b0e14  4.61:1  AA") {
		t.Errorf("the fitted accent is not reported as AA:\n%s", out.String())
	}
	out.Reset()
	if code := run([]string{"contrast", "#ffffff", "#0d9488"}, &out, &errb); code != 1 {
		t.Fatalf("go-widgets' teal should be under the bar on white, exit %d", code)
	}
	if !strings.Contains(out.String(), "3.74:1  under (large text only)") {
		t.Errorf("go-widgets' teal:\n%s", out.String())
	}
}

func TestContrastCommandRefusesWhatItCannotRead(t *testing.T) {
	for _, args := range [][]string{
		{"contrast"},
		{"contrast", "#ffffff"},
		{"contrast", "white", "#000000"},
		{"contrast", "#ffffff", "#fff"},
	} {
		var out, errb bytes.Buffer
		if code := run(args, &out, &errb); code != 2 {
			t.Errorf("%v: exit %d, want 2", args, code)
		}
		if errb.Len() == 0 {
			t.Errorf("%v: refused in silence", args)
		}
	}
	// A bare six hex digits is a colour; the hash is optional on input and
	// always present in the output, so two spellings of one colour read alike.
	var out, errb bytes.Buffer
	if code := run([]string{"contrast", "ffffff", "000000"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s%s", code, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), "#000000 on #ffffff  21.00:1  AA") {
		t.Errorf("without the hash:\n%s", out.String())
	}
}
