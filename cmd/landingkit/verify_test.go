package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// fake stands in for hugo and Chrome. No process is started: the tests of a
// command that runs other commands must never be able to run themselves.
type fake struct {
	html     map[string]string     // site dir -> the index.html "hugo" writes
	bg       map[string]color.RGBA // "site-light" etc. -> the colour "Chrome" paints
	bands    map[string][]band     // "site" etc. -> full-width bands painted over bg
	hugoErr  error
	noIndex  bool
	chromErr error
	noShot   bool
	badShot  bool
	calls    []string

	// The probe. "Chrome" answers --dump-dom by keeping the candidate points
	// the injected script asks about that no band covers -- what probe.js
	// does in a real browser -- unless one of these says otherwise.
	chromeRoom int    // window height minus viewport height, as Chrome's --dump-dom has
	leaky      string // the page whose probe does NOT exclude its bands
	silent     bool   // the probe reports nothing
	stray      bool   // the probe reports a point nobody asked about
	narrow     bool   // the probe's viewport is not shotW wide
	stuck      bool   // the probe's viewport ignores the window size
	keepOnly   int    // >0: the probe keeps only this many points
	shotErr    bool   // the screenshot fails, the probe does not
	lockRef    bool   // the ref's directory turns read-only at the first Chrome run
	locked     string // the directory lockRef made read-only, for the cleanup
	probeHTML  map[string]string
}

// band is a full-width stripe from y0 (included) to y1 (excluded).
type band struct {
	y0, y1 int
	c      color.RGBA
}

func (f *fake) run(name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	if name == "fake-chrome" {
		return f.chrome(args)
	}
	if f.hugoErr != nil {
		return []byte("hugo said no"), f.hugoErr
	}
	var src, dst string
	for i, a := range args {
		switch a {
		case "--source":
			src = args[i+1]
		case "--destination":
			dst = args[i+1]
		}
	}
	if f.noIndex {
		return nil, nil
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return nil, err
	}
	return nil, os.WriteFile(filepath.Join(dst, "index.html"), []byte(f.html[src]), 0o644)
}

func (f *fake) chrome(args []string) ([]byte, error) {
	if f.chromErr != nil {
		return []byte("chrome said no"), f.chromErr
	}
	var shot string
	theme, height, dump := "light", 0, false
	for _, a := range args {
		if s, ok := strings.CutPrefix(a, "--screenshot="); ok {
			shot = s
		}
		if s, ok := strings.CutPrefix(a, "--window-size=1280,"); ok {
			height, _ = strconv.Atoi(s)
		}
		if a == "--blink-settings=preferredColorScheme=0" {
			theme = "dark"
		}
		if a == "--dump-dom" {
			dump = true
		}
	}
	page, err := url.Parse(args[len(args)-1])
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(filepath.FromSlash(page.Path))
	name := filepath.Base(dir)
	if f.lockRef {
		f.lockRef = false
		f.locked = filepath.Join(filepath.Dir(dir), "ref")
		if err := os.Chmod(f.locked, 0o555); err != nil {
			return nil, err
		}
	}
	if dump {
		return f.dump(filepath.FromSlash(page.Path), name, height)
	}
	if f.shotErr {
		return []byte("no screenshot today"), errors.New("exit 1")
	}
	if f.noShot {
		return nil, nil
	}
	if f.badShot {
		return nil, os.WriteFile(shot, []byte("not a png"), 0o644)
	}
	img := image.NewRGBA(image.Rect(0, 0, 1280, 2400))
	c := f.bg[name+"-"+theme]
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, 255
	}
	for _, b := range f.bands[name] {
		for y := b.y0; y < b.y1; y++ {
			for x := range 1280 {
				img.SetRGBA(x, y, b.c)
			}
		}
	}
	out, err := os.Create(shot)
	if err != nil {
		return nil, err
	}
	defer out.Close()
	return nil, png.Encode(out, img)
}

var askedPoints = regexp.MustCompile(`window\.__landingkitPoints=\[(.*?)\];`)

// dump plays probe.js: it reads the points the injected script asks about
// and answers, on <html>, with the ones no band covers.
func (f *fake) dump(probePage, name string, height int) ([]byte, error) {
	b, err := os.ReadFile(probePage)
	if err != nil {
		return nil, err
	}
	if f.probeHTML == nil {
		f.probeHTML = map[string]string{}
	}
	f.probeHTML[name] = string(b)
	if f.silent {
		return []byte("<html><body></body></html>"), nil
	}
	w, h := 1280, height-f.chromeRoom
	if f.narrow {
		w = 800
	}
	if f.stuck {
		h = 2313
	}
	m := askedPoints.FindSubmatch(b)
	var keep []string
	for _, p := range strings.Split(strings.Trim(string(m[1]), "[]"), "],[") {
		var x, y int
		fmt.Sscanf(p, "%d,%d", &x, &y)
		covered := false
		for _, bd := range f.bands[name] {
			covered = covered || (y >= bd.y0 && y < bd.y1 && name != f.leaky)
		}
		if !covered && y < h {
			keep = append(keep, fmt.Sprintf("%d,%d", x, y))
		}
	}
	if f.keepOnly > 0 {
		keep = keep[:f.keepOnly]
	}
	if f.stray {
		keep = append(keep, "7,7")
	}
	return fmt.Appendf(nil, "noise on stderr\n<html data-landingkit-viewport=\"%dx%d\" data-landingkit-background=\"%s\"><body></body></html>", w, h, strings.Join(keep, " ")), nil
}

var (
	paper  = color.RGBA{255, 255, 255, 255}
	night  = color.RGBA{0x0b, 0x0e, 0x14, 255}
	mint   = color.RGBA{0xf0, 0xfd, 0xf9, 255}
	sheetA = "<style>\n  :root{ --cyan:#b45309; --grad:linear-gradient(135deg,#b45309,#6e3305); --bg:#ffffff; }\n</style>"
	sheetB = "<style>\n  :root{ --cyan:#0079A8; --grad:linear-gradient(135deg,#0079A8,#054A66); --bg:#ffffff; }\n</style>"
)

// sites makes a branded site and an unbranded reference, as on disk.
func sites(t *testing.T) (site, ref string) {
	t.Helper()
	dir := t.TempDir()
	site, ref = filepath.Join(dir, "site"), filepath.Join(dir, "ref")
	for p, toml := range map[string]string{
		site: "title = 'x'\n[params.brand]\n  base = \"#b45309\"\n  deep = \"#6e3305\"\n[[params.repos]]\n  name = \"#ffffff\"\n",
		ref:  "title = 'ref'\n",
	} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "hugo.toml"), []byte(toml), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return site, ref
}

func newFake(site, ref string) *fake {
	return &fake{
		html: map[string]string{site: "<html>" + sheetA + "</html>", ref: "<html>" + sheetB + "</html>"},
		bg: map[string]color.RGBA{
			"site-light": paper, "ref-light": paper,
			"site-dark": night, "ref-dark": night,
		},
		chromeRoom: 87, // measured: 2313 px of viewport for a 2400 px window
	}
}

func verify(t *testing.T, f *fake, args ...string) (int, string, string) {
	t.Helper()
	old := runCmd
	runCmd = f.run
	defer func() { runCmd = old }()
	var out, errb bytes.Buffer
	code := run(append([]string{"verify", "-chrome", "fake-chrome", "-out", filepath.Join(t.TempDir(), "renders")}, args...), &out, &errb)
	return code, out.String(), errb.String()
}

func TestVerifyPasses(t *testing.T) {
	site, ref := sites(t)
	f := newFake(site, ref)
	code, out, errs := verify(t, f, site, ref)
	if code != 0 {
		t.Fatalf("exit %d\n%s%s", code, out, errs)
	}
	for _, want := range []string{
		"PASS the <style> block equals",
		"PASS light: 92/92 background pixels identical (#ffffff)",
		"PASS dark: 92/92 background pixels identical (#0b0e14)",
		"(builds and renders in ",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	// Two builds, four probes, four renders, the theme forced both ways, and
	// ONE probe again: the first one learns that the --dump-dom window keeps
	// 87 px for browser chrome, and every later one asks for 2487.
	if len(f.calls) != 11 || strings.Count(strings.Join(f.calls, "\n"), "--window-size=1280,2487") != 4 {
		t.Fatalf("%d calls: %q", len(f.calls), f.calls)
	}
	for _, want := range []string{"preferredColorScheme=1", "preferredColorScheme=0 --force-dark-mode"} {
		if !strings.Contains(strings.Join(f.calls, "\n"), want) {
			t.Errorf("no render with %q", want)
		}
	}
}

// The defect the kit exists for: go-fft's emerald surfaces. Text equal,
// background not.
func TestVerifyCatchesATintedBackground(t *testing.T) {
	site, ref := sites(t)
	f := newFake(site, ref)
	f.bg["site-light"] = mint
	code, out, _ := verify(t, f, site, ref)
	if code != 1 || !strings.Contains(out, "FAIL light: 92/92 background pixels differ, e.g. (4, 100) site #f0fdf9 ref #ffffff") ||
		!strings.Contains(out, "PASS dark") {
		t.Fatalf("exit %d\n%s", code, out)
	}
}

// A capture that did not get the theme it asked for is called wrong, not
// compared.
func TestVerifyRefusesAWrongCapture(t *testing.T) {
	site, ref := sites(t)
	f := newFake(site, ref)
	f.bg["ref-dark"], f.bg["site-dark"] = paper, paper
	code, out, _ := verify(t, f, site, ref)
	if code != 1 || !strings.Contains(out, "FAIL dark: the REFERENCE rendered #ffffff, not a dark page; the capture is wrong") {
		t.Fatalf("exit %d\n%s", code, out)
	}
}

func TestVerifyCatchesAStyleDifference(t *testing.T) {
	site, ref := sites(t)
	f := newFake(site, ref)
	long := strings.Repeat("x", 200)
	f.html[site] = "<style>\n  --bg:#f0fdf9;\n  " + long + "\n  same\n  extra\n</style>"
	f.html[ref] = "<style>\n  --bg:#ffffff;\n  " + long + "y\n  same\n</style>"
	code, out, _ := verify(t, f, "-pixels=false", site, ref)
	if code != 1 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	for _, want := range []string{
		"FAIL the <style> blocks differ beyond brand values:",
		"  site: --bg:#f0fdf9;\n  ref:  --bg:#ffffff;",
		"  site: " + long[:120] + "\n",
		"  site has 6 lines, ref 5",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "background pixels") {
		t.Error("-pixels=false rendered anyway")
	}
}

// Masking replaces the longest value first and every value of the site's
// [params.brand], but nothing from another table.
func TestBrandValues(t *testing.T) {
	site, _ := sites(t)
	vals, err := brandValues(site)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(vals, " ")
	for _, want := range []string{"#b45309", "#6e3305", "#0079A8", "rgba(34,204,226,.14)"} {
		if !strings.Contains(joined, want) {
			t.Errorf("%q not masked: %v", want, vals)
		}
	}
	if strings.Contains(joined, "#ffffff") {
		t.Errorf("a value outside [params.brand] is masked: %v", vals)
	}
	for i := 1; i < len(vals); i++ {
		if len(vals[i]) > len(vals[i-1]) {
			t.Fatalf("not longest first: %v", vals)
		}
	}
}

func TestVerifyFailures(t *testing.T) {
	site, ref := sites(t)
	empty := t.TempDir()
	for _, c := range []struct {
		name string
		set  func(*fake)
		args []string
		code int
		says string
	}{
		{"hugo fails", func(f *fake) { f.hugoErr = errors.New("exit 1") }, nil, 1, "hugo said no"},
		{"no index.html", func(f *fake) { f.noIndex = true }, nil, 1, "index.html"},
		{"no style", func(f *fake) { f.html[site] = "<html></html>" }, nil, 1, "has no <style> block"},
		{"no hugo.toml", func(f *fake) { f.html[empty] = sheetA }, []string{empty, ref}, 1, "hugo.toml"},
		{"chrome fails", func(f *fake) { f.chromErr = errors.New("exit 1") }, nil, 1, "chrome said no"},
		{"no screenshot", func(f *fake) { f.noShot = true }, nil, 1, "site-light.png"},
		{"not a png", func(f *fake) { f.badShot = true }, nil, 1, "png"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFake(site, ref)
			c.set(f)
			args := c.args
			if args == nil {
				args = []string{site, ref}
			}
			code, out, errs := verify(t, f, args...)
			if code != c.code || !strings.Contains(errs, c.says) {
				t.Fatalf("exit %d, want %d and %q\nstdout:\n%s\nstderr:\n%s", code, c.code, c.says, out, errs)
			}
		})
	}
}

func TestVerifyUsage(t *testing.T) {
	for _, c := range []struct {
		args []string
		code int
	}{
		{[]string{"verify", "-h"}, 0},
		{[]string{"verify", "-nope"}, 2},
		{[]string{"verify", "one"}, 2},
		{[]string{"verify", "-hugo", " ", "a", "b"}, 2},
	} {
		var out, errb bytes.Buffer
		if code := run(c.args, &out, &errb); code != c.code || !strings.Contains(errb.String(), "landingkit verify") {
			t.Errorf("%v: exit %d, stderr %q", c.args, code, errb.String())
		}
	}
}

func TestVerifyOutputDirectory(t *testing.T) {
	site, ref := sites(t)

	t.Run("refused inside a work tree", func(t *testing.T) {
		tree := t.TempDir()
		if err := os.Mkdir(filepath.Join(tree, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		f := newFake(site, ref)
		code, _, errs := verify(t, f, "-out", filepath.Join(tree, "a", "b"), site, ref)
		if code != 1 || !strings.Contains(errs, "is inside the git work tree "+tree) {
			t.Fatalf("exit %d: %s", code, errs)
		}
		if len(f.calls) != 0 {
			t.Fatal("built before refusing")
		}
	})

	t.Run("cannot be created", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(file, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		code, _, errs := verify(t, newFake(site, ref), "-out", filepath.Join(file, "x"), site, ref)
		if code != 1 || !strings.Contains(errs, "not a directory") {
			t.Fatalf("exit %d: %s", code, errs)
		}
	})

	t.Run("a fresh temporary directory by default", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("TMPDIR", tmp)
		old := runCmd
		runCmd = newFake(site, ref).run
		defer func() { runCmd = old }()
		var out, errb bytes.Buffer
		if code := run([]string{"verify", "-pixels=false", site, ref}, &out, &errb); code != 0 {
			t.Fatalf("exit %d: %s", code, errb.String())
		}
		if !strings.Contains(out.String(), "(builds and renders in "+filepath.Join(tmp, "landingkit-")) {
			t.Fatalf("not under TMPDIR %s:\n%s", tmp, out.String())
		}
	})

	t.Run("temporary directory fails", func(t *testing.T) {
		old := mkdirTemp
		mkdirTemp = func(string, string) (string, error) { return "", errors.New("disk full") }
		defer func() { mkdirTemp = old }()
		var out, errb bytes.Buffer
		if code := run([]string{"verify", "-pixels=false", site, ref}, &out, &errb); code != 1 || !strings.Contains(errb.String(), "disk full") {
			t.Fatalf("exit %d: %s", code, errb.String())
		}
	})

	t.Run("no absolute path", func(t *testing.T) {
		old := absPath
		absPath = func(string) (string, error) { return "", errors.New("getwd: gone") }
		defer func() { absPath = old }()
		var out, errb bytes.Buffer
		if code := run([]string{"verify", "-pixels=false", "-out", "rel", site, ref}, &out, &errb); code != 1 || !strings.Contains(errb.String(), "getwd: gone") {
			t.Fatalf("exit %d: %s", code, errb.String())
		}
	})
}

func TestFindChrome(t *testing.T) {
	old := chromeCandidates
	defer func() { chromeCandidates = old }()

	chromeCandidates = []string{"landingkit-no-such-browser"}
	if got := findChrome(); got != "" {
		t.Fatalf("found %q", got)
	}
	var out, errb bytes.Buffer
	if code := run([]string{"verify", "a", "b"}, &out, &errb); code != 1 || !strings.Contains(errb.String(), "no Chrome found") {
		t.Fatalf("exit %d: %s", code, errb.String())
	}

	// An executable that exists stands for a browser; it is looked up, never run.
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	chromeCandidates = []string{"landingkit-no-such-browser", exe}
	if got := findChrome(); got != exe {
		t.Fatalf("found %q, want %q", got, exe)
	}
}

// The real runner, on a program that does not exist: it must report the
// failure rather than start anything.
func TestRunCmd(t *testing.T) {
	if _, err := runCmd("landingkit-no-such-program"); err == nil {
		t.Fatal("ran a program that does not exist")
	}
}

var bgSoft = color.RGBA{0xf7, 0xf8, 0xfb, 255}

// The false FAIL this probe exists for: go-embedded-ruby's .strip, a tall
// full-width band on --bg-soft, sat where the fixed points used to sample.
// The site has the band, the reference does not, and the backgrounds are the
// same: PASS. The witness shows the old points WOULD have read the band.
func TestVerifyIgnoresATallBandAtTheOldSampleHeight(t *testing.T) {
	site, ref := sites(t)
	f := newFake(site, ref)
	f.html[site] = "<html><body>" + sheetA + "</body></html>"
	f.bands = map[string][]band{"site": {{850, 1350, bgSoft}}}
	code, out, errs := verify(t, f, site, ref)
	if code != 0 || !strings.Contains(out, "PASS light: 72/72 background pixels identical (#ffffff)") {
		t.Fatalf("exit %d\n%s%s", code, out, errs)
	}
	// The probe went in before </body>, and asked about the gutters.
	if p := f.probeHTML["site"]; !strings.Contains(p, "window.__landingkitPoints=[[4,100],[1275,100],") || !strings.HasSuffix(p, "</script></body></html>") {
		t.Fatalf("probe page:\n%.300s", p)
	}
	old := [][2]int{{4, 900}, {4, 1200}, {1275, 1000}}
	dir := regexp.MustCompile(`\(builds and renders in (.*)\)`).FindStringSubmatch(out)[1]
	px, err := sample(filepath.Join(dir, "site-light.png"), old)
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range px {
		if c != (rgb{0xf7, 0xf8, 0xfb}) {
			t.Fatalf("witness: the old point %v reads %s, not the band", old[i], rgbHex(c))
		}
	}
}

// The band is excluded, the background is not: a tinted page with a band
// still FAILS on every point outside the band.
func TestVerifyBandDoesNotHideATint(t *testing.T) {
	site, ref := sites(t)
	f := newFake(site, ref)
	f.bands = map[string][]band{"site": {{850, 1350, bgSoft}}}
	f.bg["site-light"] = mint
	code, out, _ := verify(t, f, site, ref)
	if code != 1 || !strings.Contains(out, "FAIL light: 72/72 background pixels differ, e.g. (4, 100) site #f0fdf9 ref #ffffff") {
		t.Fatalf("exit %d\n%s", code, out)
	}
}

func TestVerifyProbeVerdicts(t *testing.T) {
	site, ref := sites(t)
	for _, c := range []struct {
		name string
		set  func(*fake)
		says string
	}{
		// A page whose gutters are all covered is not passed for lack of
		// evidence.
		{"too little background", func(f *fake) { f.keepOnly = minPoints - 1 },
			"FAIL light: too little page background to judge: 7 points on the site, 7 on the reference, want 8 each"},
		// A probe that let a band through on the REFERENCE is caught by the
		// reference not being one colour.
		{"leaky reference", func(f *fake) {
			f.bands = map[string][]band{"ref": {{850, 1350, bgSoft}}}
			f.leaky = "ref"
		}, "FAIL light: the REFERENCE's background is not one colour: #ffffff at (4, 100), #f7f8fb at (4, 850)"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFake(site, ref)
			c.set(f)
			code, out, _ := verify(t, f, site, ref)
			if code != 1 || !strings.Contains(out, c.says) {
				t.Fatalf("exit %d, want 1 and %q\n%s", code, c.says, out)
			}
		})
	}
}

func TestVerifyProbeFailures(t *testing.T) {
	site, ref := sites(t)
	for _, c := range []struct {
		name string
		set  func(*fake)
		says string
	}{
		{"silent", func(f *fake) { f.silent = true }, "probing site light: the probe did not report"},
		{"stray point", func(f *fake) { f.stray = true }, `probing site light: "7,7" is not a point that was asked about`},
		{"narrow", func(f *fake) { f.narrow = true }, "the probe's viewport is 800x2313, the screenshot's 1280x2400"},
		{"stuck", func(f *fake) { f.stuck = true }, "the probe's viewport is 1280x2313, the screenshot's 1280x2400"},
		{"screenshot fails", func(f *fake) { f.shotErr = true }, "rendering site light: exit 1\nno screenshot today"},
		{"probe page cannot be written", func(f *fake) { f.lockRef = true }, "landingkit-probe.html"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFake(site, ref)
			c.set(f)
			code, out, errs := verify(t, f, site, ref)
			if f.locked != "" {
				defer os.Chmod(f.locked, 0o755)
			}
			if code != 1 || !strings.Contains(errs, c.says) {
				t.Fatalf("exit %d, want 1 and %q\nstdout:\n%s\nstderr:\n%s", code, c.says, out, errs)
			}
		})
	}
}

// The probe reads the built page; a page gone between the build and the
// probe is an error, not an empty probe.
func TestProbeWithoutAPage(t *testing.T) {
	v := verifier{out: t.TempDir(), chrome: "fake-chrome"}
	if _, err := v.probe("site", false); err == nil || !strings.Contains(err.Error(), "index.html") {
		t.Fatalf("err = %v", err)
	}
}
