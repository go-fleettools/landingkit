package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"image/png"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/go-fleettools/landingkit"
)

// `verify` fails unless both hold:
//
//  1. Text: the built <style> block equals the reference's once each site's
//     brand values (and the sheet's cyan defaults) are masked. Any other
//     difference -- a tinted surface, an extra rule -- fails.
//  2. Pixels: headless Chrome renders both pages, light AND dark, and the page
//     background is sampled in the gutters: every sample must equal the
//     reference's exactly. The theme is forced explicitly in both directions:
//     headless Chrome otherwise inherits the host's appearance, and on a Mac in
//     dark mode the first version of this check compared dark against dark and
//     called it light. The reference itself must look like the requested theme,
//     or the capture is declared wrong.
//
// The pixel check is what found the defect the text check had been built
// around: a colour-normalised CSS diff hid go-fft's emerald backgrounds
// (#f0fdf9 light, #04130d dark, where the reference is #ffffff and #0b0e14).

// runCmd runs a program and returns what it printed. A variable so the tests
// can stand in for hugo and Chrome without starting either.
var runCmd = func(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

var (
	mkdirTemp = os.MkdirTemp
	absPath   = filepath.Abs
)

// chromeCandidates are tried in order when -chrome is not given.
var chromeCandidates = []string{
	"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	"google-chrome", "google-chrome-stable", "chromium", "chromium-browser",
}

func findChrome() string {
	for _, c := range chromeCandidates {
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}
	return ""
}

type verifier struct {
	hugo   []string
	chrome string
	out    string
	stdout io.Writer
}

func cmdVerify(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	hugo := fs.String("hugo", "hugo", "the Hugo command, words split on spaces (e.g. \"pkgx hugo@0.167.0\")")
	chrome := fs.String("chrome", "", "the Chrome binary for the pixel check (default: the first one found)")
	pixels := fs.Bool("pixels", true, "also render both pages in headless Chrome and compare the background")
	out := fs.String("out", "", "where builds and renders go (default: a new temporary directory); refused inside a git work tree")
	fs.Usage = func() {
		fmt.Fprint(stderr, "usage: landingkit verify [flags] <site> <ref-site>\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 2 || len(strings.Fields(*hugo)) == 0 {
		fs.Usage()
		return 2
	}
	v := verifier{hugo: strings.Fields(*hugo), chrome: *chrome, out: *out, stdout: stdout}
	if err := v.prepare(*pixels); err != nil {
		fmt.Fprintln(stderr, "landingkit:", err)
		return 1
	}
	ok, err := v.text(fs.Arg(0), fs.Arg(1))
	if err == nil && *pixels {
		var okp bool
		okp, err = v.pixels()
		ok = ok && okp
	}
	fmt.Fprintf(stdout, "(builds and renders in %s)\n", v.out)
	if err != nil {
		fmt.Fprintln(stderr, "landingkit:", err)
		return 1
	}
	if !ok {
		return 1
	}
	return 0
}

// prepare settles the output directory and the Chrome binary before anything
// is built, so a missing browser is said at once and not after two builds.
func (v *verifier) prepare(pixels bool) error {
	if pixels && v.chrome == "" {
		if v.chrome = findChrome(); v.chrome == "" {
			return errors.New("no Chrome found for the pixel check: pass -chrome, or -pixels=false to run the text check alone")
		}
	}
	if v.out == "" {
		d, err := mkdirTemp("", "landingkit-")
		if err != nil {
			return err
		}
		v.out = d
	}
	abs, err := absPath(v.out)
	if err != nil {
		return err
	}
	v.out = abs
	if tree := workTree(abs); tree != "" {
		return fmt.Errorf("%s is inside the git work tree %s: renders go outside every repository", abs, tree)
	}
	return os.MkdirAll(abs, 0o755)
}

// workTree is the nearest directory at or above dir holding a .git, or "".
// A .gitignore entry is not a barrier -- `git add -f` or a fresh clone
// publishes the file anyway -- so renders are refused inside a work tree.
func workTree(dir string) string {
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func (v *verifier) build(site, name string) (string, error) {
	dst := filepath.Join(v.out, name)
	args := append(append([]string{}, v.hugo[1:]...), "--source", site, "--destination", dst, "--quiet")
	if out, err := runCmd(v.hugo[0], args...); err != nil {
		return "", fmt.Errorf("building %s: %v\n%s", site, err, out)
	}
	b, err := os.ReadFile(filepath.Join(dst, "index.html"))
	return string(b), err
}

var styleRE = regexp.MustCompile(`(?s)<style>(.*?)</style>`)

func styleBlock(html, site string) (string, error) {
	m := styleRE.FindStringSubmatch(html)
	if m == nil {
		return "", fmt.Errorf("%s: the built index.html has no <style> block", site)
	}
	return m[1], nil
}

var (
	brandTable  = regexp.MustCompile(`(?s)\[params\.brand\](.*?)(?:\n\[|\z)`)
	brandValue  = regexp.MustCompile(`=\s*"([^"]+)"`)
	sheetDefaul = regexp.MustCompile(`\$br\.\w+\s*\|\s*default\s*"([^"]+)"`)
)

// brandValues are the literal values a site's [params.brand] emits, plus the
// sheet's own defaults, longest first so that no value is masked inside a
// longer one before the longer one is.
func brandValues(site string) ([]string, error) {
	toml, err := os.ReadFile(filepath.Join(site, "hugo.toml"))
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	if m := brandTable.FindSubmatch(toml); m != nil {
		for _, v := range brandValue.FindAllSubmatch(m[1], -1) {
			set[string(v[1])] = true
		}
	}
	for _, v := range sheetDefaul.FindAllStringSubmatch(landingkit.Styles, -1) {
		set[v[1]] = true
	}
	vals := make([]string, 0, len(set))
	for v := range set {
		vals = append(vals, v)
	}
	sort.Slice(vals, func(i, j int) bool {
		if len(vals[i]) != len(vals[j]) {
			return len(vals[i]) > len(vals[j])
		}
		return vals[i] < vals[j]
	})
	return vals, nil
}

func masked(style string, vals []string) string {
	for _, v := range vals {
		style = strings.ReplaceAll(style, v, "BRAND")
	}
	return style
}

func (v *verifier) text(site, ref string) (bool, error) {
	var styles [2]string
	for i, s := range [2][2]string{{site, "site"}, {ref, "ref"}} {
		html, err := v.build(s[0], s[1])
		if err != nil {
			return false, err
		}
		st, err := styleBlock(html, s[0])
		if err != nil {
			return false, err
		}
		vals, err := brandValues(s[0])
		if err != nil {
			return false, err
		}
		styles[i] = masked(st, vals)
	}
	if styles[0] == styles[1] {
		fmt.Fprintln(v.stdout, "PASS the <style> block equals the reference's once brand values are masked")
		return true, nil
	}
	fmt.Fprintln(v.stdout, "FAIL the <style> blocks differ beyond brand values:")
	a, b := strings.Split(styles[0], "\n"), strings.Split(styles[1], "\n")
	shown := 0
	for i := 0; i < len(a) && i < len(b) && shown < 6; i++ {
		if a[i] != b[i] {
			fmt.Fprintf(v.stdout, "  site: %s\n  ref:  %s\n", clip(a[i]), clip(b[i]))
			shown++
		}
	}
	if len(a) != len(b) {
		fmt.Fprintf(v.stdout, "  site has %d lines, ref %d\n", len(a), len(b))
	}
	return false, nil
}

func clip(s string) string {
	r := []rune(strings.TrimSpace(s))
	return string(r[:min(len(r), 120)])
}

// neutral are points on the page background, away from cards, text and the
// hero washes: the left gutter at several heights, and the right gutter.
var neutral = [][2]int{
	{4, 900}, {4, 1200}, {4, 1500}, {4, 1800}, {4, 2100}, {4, 2350},
	{1275, 1000}, {1275, 1600}, {1275, 2200},
}

func (v *verifier) shoot(name string, dark bool) (string, error) {
	theme, scheme := "light", 1 // Blink's PreferredColorScheme: 0 = dark, 1 = light
	if dark {
		theme, scheme = "dark", 0
	}
	shot := filepath.Join(v.out, name+"-"+theme+".png")
	args := []string{"--headless=new", "--disable-gpu", "--hide-scrollbars",
		"--window-size=1280,2400", "--screenshot=" + shot,
		fmt.Sprintf("--blink-settings=preferredColorScheme=%d", scheme)}
	if dark {
		args = append(args, "--force-dark-mode")
	}
	page := url.URL{Scheme: "file", Path: filepath.ToSlash(filepath.Join(v.out, name, "index.html"))}
	args = append(args, page.String())
	if out, err := runCmd(v.chrome, args...); err != nil {
		return "", fmt.Errorf("rendering %s %s: %v\n%s", name, theme, err, out)
	}
	return shot, nil
}

func sample(shot string, points [][2]int) ([]rgb, error) {
	f, err := os.Open(shot)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", shot, err)
	}
	res := make([]rgb, 0, len(points))
	for _, p := range points {
		r, g, b, _ := img.At(p[0], p[1]).RGBA()
		res = append(res, rgb{float64(r >> 8), float64(g >> 8), float64(b >> 8)})
	}
	return res, nil
}

func (v *verifier) pixels() (bool, error) {
	ok := true
	for _, dark := range []bool{false, true} {
		theme := map[bool]string{false: "light", true: "dark"}[dark]
		var got [2][]rgb
		for i, name := range []string{"site", "ref"} {
			shot, err := v.shoot(name, dark)
			if err != nil {
				return false, err
			}
			if got[i], err = sample(shot, neutral); err != nil {
				return false, err
			}
		}
		site, ref := got[0], got[1]
		// The reference must itself look like the theme asked for, or the
		// capture is not testing what it claims to.
		if (luminance(ref[0]) < 0.2) != dark {
			ok = false
			fmt.Fprintf(v.stdout, "FAIL %s: the REFERENCE rendered %s, not a %s page; the capture is wrong\n", theme, rgbHex(ref[0]), theme)
			continue
		}
		var diff []string
		for i, p := range neutral {
			if site[i] != ref[i] {
				diff = append(diff, fmt.Sprintf("(%d, %d) site %s ref %s", p[0], p[1], rgbHex(site[i]), rgbHex(ref[i])))
			}
		}
		if len(diff) > 0 {
			ok = false
			fmt.Fprintf(v.stdout, "FAIL %s: %d/%d background pixels differ, e.g. %s\n", theme, len(diff), len(neutral), strings.Join(diff[:min(3, len(diff))], ", "))
			continue
		}
		fmt.Fprintf(v.stdout, "PASS %s: %d/%d background pixels identical (%s)\n", theme, len(neutral), len(neutral), rgbHex(site[0]))
	}
	return ok, nil
}
