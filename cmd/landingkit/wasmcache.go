package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-fleettools/landingkit/wasmcache"
)

const wasmcacheUsage = `usage:
  landingkit wasmcache [flags]            write the service worker, page script and version file into a built site
  landingkit wasmcache testkit -out <dir>  write the browser tests (wasmcache-e2e.cjs) a playground runs in its CI

flags: -dir <built site> -asset <glob|dir/> (repeat) -scope </path/> (repeat) [-exclude </prefix/> (repeat, default /api/)] [-version <deploy id>] [-home </path/>] [-name <cache name>]
`

type list []string

func (l *list) String() string     { return strings.Join(*l, ",") }
func (l *list) Set(v string) error { *l = append(*l, v); return nil }

func cmdWasmcache(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "testkit" {
		return cmdWasmcacheTestkit(args[1:], stdout, stderr)
	}
	fl := flag.NewFlagSet("wasmcache", flag.ContinueOnError)
	fl.SetOutput(stderr)
	var cfg wasmcache.Config
	var assets, scopes, exclude list
	fl.StringVar(&cfg.Dir, "dir", "", "the built site")
	fl.Var(&assets, "asset", "a glob or a directory/ to cache (repeat)")
	fl.Var(&scopes, "scope", "a worker's scope, /playground/ (repeat)")
	fl.Var(&exclude, "exclude", "a path prefix never cached (repeat; default /api/)")
	fl.StringVar(&cfg.Version, "version", "", "the deploy's identity (default: the content hash)")
	fl.StringVar(&cfg.Home, "home", "", "where the page script and version file go (default: the first scope)")
	fl.StringVar(&cfg.Name, "name", "", "the cache's name prefix (default wasmcache)")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if cfg.Dir == "" {
		fmt.Fprint(stderr, wasmcacheUsage)
		return 2
	}
	cfg.Assets, cfg.Scopes, cfg.Exclude = assets, scopes, exclude
	m, err := wasmcache.Generate(cfg)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	var size int64
	for _, f := range m.Files {
		size += f.Size
	}
	fmt.Fprintf(stdout, "wasmcache: build %s, version %s, %d files, %.1f MiB, %d scope(s)\n", m.Build, m.Version, len(m.Files), float64(size)/(1<<20), len(scopes))
	return 0
}

func cmdWasmcacheTestkit(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("wasmcache testkit", flag.ContinueOnError)
	fl.SetOutput(stderr)
	out := fl.String("out", "", "the directory to write wasmcache-e2e.cjs into")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if *out == "" {
		fmt.Fprint(stderr, wasmcacheUsage)
		return 2
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	p := filepath.Join(*out, "wasmcache-e2e.cjs")
	if err := os.WriteFile(p, wasmcache.Testkit(), 0o644); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, p)
	return 0
}
