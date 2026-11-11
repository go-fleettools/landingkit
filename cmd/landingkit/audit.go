package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// `audit` runs check's rules on PUBLISHED landings, by URL.
//
//	landingkit audit https://go-numerics.github.io/ https://go-ndarray.github.io/ …
//
// `check` is the gate, but only for a landing whose CI runs it; the drift it
// was written for (a ghost "Try it" button, a missing icon, a row in the wrong
// order, a fact said twice) lives in the landings that do not. audit reads
// what visitors see, one page per URL, so the whole fleet is measured without
// a clone, and prints one verdict line per page: "ok", the findings, or why
// the page could not be read. An unreadable page is never a pass.
//
// Bounds: https only (http is refused, so a typo cannot send a request in
// clear), 15 s per page, 8 MiB per body, at most 5 redirects, nothing written.

const (
	auditMaxBody  = 8 << 20
	auditTimeout  = 15 * time.Second
	auditRedirect = 5
)

// auditClient is replaced by the tests, which serve pages over TLS.
var auditClient = &http.Client{
	Timeout: auditTimeout,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= auditRedirect {
			return fmt.Errorf("more than %d redirects", auditRedirect)
		}
		if req.URL.Scheme != "https" {
			return fmt.Errorf("redirect to %s, not https", req.URL.Scheme)
		}
		return nil
	},
}

// auditPage returns the findings for one URL, or an error when it could not
// be read in full.
func auditPage(raw string) ([]string, int, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, 0, fmt.Errorf("not an https URL")
	}
	resp, err := auditClient.Get(u.String())
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, auditMaxBody+1))
	if err != nil {
		return nil, 0, err
	}
	if len(b) > auditMaxBody {
		return nil, 0, fmt.Errorf("page larger than %d bytes", auditMaxBody)
	}
	page := string(b)
	var out []string
	for _, r := range redundant(page) {
		out = append(out, r+", which the mantra strip already says")
	}
	order, rows := ctaDisorder(page)
	out = append(out, order...)
	out = append(out, ctaStray(page)...)
	return out, rows, nil
}

// auditList reads one URL per line from a file; blank lines and lines
// starting with # are skipped. The fleet's list is produced by the forge
// (`gh api … --jq`), so the file carries no shell processing.
func auditList(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if l != "" && !strings.HasPrefix(l, "#") {
			out = append(out, l)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s lists no URL", path)
	}
	return out, nil
}

func cmdAudit(args []string, stdout, stderr io.Writer) int {
	if len(args) == 2 && args[0] == "-f" {
		list, err := auditList(args[1])
		if err != nil {
			fmt.Fprintf(stderr, "landingkit audit: %v\n", err)
			return 2
		}
		args = list
	}
	if len(args) == 0 || args[0] == "-f" {
		fmt.Fprint(stderr, "usage: landingkit audit <https-url>... | -f <file of URLs>\n")
		return 2
	}
	bad, failed := 0, 0
	for _, raw := range args {
		found, rows, err := auditPage(raw)
		switch {
		case err != nil:
			fmt.Fprintf(stdout, "UNREAD %s: %v\n", raw, err)
			failed++
		case len(found) == 0:
			fmt.Fprintf(stdout, "ok     %s (%d hero row(s))\n", raw, rows)
		default:
			bad++
			for _, f := range found {
				fmt.Fprintf(stdout, "DRIFT  %s: %s\n", raw, f)
			}
		}
	}
	fmt.Fprintf(stdout, "audit: %d page(s), %d ok, %d drifted, %d unread\n", len(args), len(args)-bad-failed, bad, failed)
	if bad > 0 || failed > 0 {
		return 1
	}
	return 0
}
