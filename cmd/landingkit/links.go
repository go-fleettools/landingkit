package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// `links` is the landing kit's link check (it was workflows/links.yml beside
// kit.py): every http(s) URL in the BUILT site must answer 2xx or 3xx.
//
// ⛔ It reads the built site, not the layouts. The first version grepped
// layouts/ and failed on `https://github.com/go-fileshare/{{ .name }}` -- a
// Hugo template expression, not a URL. Skipping anything with a brace would
// have left the per-repository card links unchecked, which is most of a page.
//
// ⛔ A github.com blob link is probed on the RAW host. github.com answers a
// blob page fetched by a program with 503 most of the time -- 3 attempts out
// of 3 on every Actions run, 7 of 8 locally, with githubstatus green -- so a
// checker that asks github.com goes red on any such link, and a lane that is
// red at random is a lane people learn to ignore. A blob link claims "this
// file exists at this ref", and raw.githubusercontent.com answers exactly
// that: 200 for a file, 404 for a missing one. go-ndarray/docs#30 found and
// fixed it there; this is the same rewrite.

var (
	urlRE  = regexp.MustCompile(`https?://[^)">< \t\n\v\f\r]+`)
	blobRE = regexp.MustCompile(`^([^/]+/[^/]+)/blob/`)
	tailRE = regexp.MustCompile(`[?#].*$`)

	// rawBase is the raw host; the tests point it at a local server.
	rawBase = "https://raw.githubusercontent.com"
	sleep   = time.Sleep
)

// probe is the URL actually fetched for u.
func probe(u string) string {
	rest, ok := strings.CutPrefix(u, "https://github.com/")
	if !ok {
		return u
	}
	// The shell's `case https://github.com/*/*/blob/*`: some "/blob/" with a
	// "/" before it.
	i := strings.Index(rest, "/blob/")
	if i < 0 || !strings.Contains(rest[:i], "/") {
		return u
	}
	return rawBase + "/" + tailRE.ReplaceAllString(blobRE.ReplaceAllString(rest, "$1/"), "")
}

// siteURLs are the distinct URLs in the text files under dir. A file with a
// NUL byte is binary (a wasm, an image) and is skipped, as grep skips it.
func siteURLs(dir string) ([]string, error) {
	set := map[string]bool{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil || bytes.IndexByte(b, 0) >= 0 {
			return err
		}
		for _, m := range urlRE.FindAll(b, -1) {
			u := string(m)
			if strings.HasSuffix(u, ".") || strings.HasSuffix(u, ",") {
				u = u[:len(u)-1]
			}
			set[u] = true
		}
		return nil
	})
	urls := make([]string, 0, len(set))
	for u := range set {
		urls = append(urls, u)
	}
	sort.Strings(urls)
	return urls, err
}

// status is the HTTP status of one GET, following redirects, or 0 when there
// was no answer at all (curl's 000).
func status(c *http.Client, u string) int {
	resp, err := c.Get(u)
	if err != nil {
		return 0
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode
}

func cmdLinks(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("links", flag.ContinueOnError)
	fl.SetOutput(stderr)
	attempts := fl.Int("attempts", 3, "tries per URL: one is a flake detector rather than a link checker")
	wait := fl.Duration("wait", 3*time.Second, "pause between tries")
	timeout := fl.Duration("timeout", 25*time.Second, "time allowed per request")
	fl.Usage = func() {
		fmt.Fprint(stderr, "usage: landingkit links [flags] <built-site-dir>\n")
		fl.PrintDefaults()
	}
	if err := fl.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fl.NArg() != 1 || *attempts < 1 {
		fl.Usage()
		return 2
	}
	urls, err := siteURLs(fl.Arg(0))
	if err != nil {
		fmt.Fprintln(stderr, "landingkit:", err)
		return 1
	}
	if len(urls) == 0 {
		fmt.Fprintf(stderr, "landingkit: no links found under %s; did the site build?\n", fl.Arg(0))
		return 1
	}
	fmt.Fprintf(stdout, "checking %d distinct URLs\n", len(urls))
	client := &http.Client{Timeout: *timeout}
	failed := 0
	for _, u := range urls {
		// img.shields.io renders anything asked of it and proves nothing.
		if strings.Contains(u, "img.shields.io") {
			continue
		}
		p, code := probe(u), 0
		for try := 1; try <= *attempts; try++ {
			if code = status(client, p); code >= 200 && code < 400 {
				break
			}
			if try < *attempts {
				sleep(*wait)
			}
		}
		via := ""
		if p != u {
			via = "  (via " + p + ")"
		}
		if code >= 200 && code < 400 {
			fmt.Fprintf(stdout, "  ok   %03d  %s%s\n", code, u, via)
			continue
		}
		fmt.Fprintf(stdout, "  FAIL %03d  %s%s\n", code, u, via)
		failed++
	}
	if failed > 0 {
		fmt.Fprintf(stderr, "landingkit: %d link(s) or image(s) do not resolve\n", failed)
		return 1
	}
	return 0
}
