// Command landingkit derives a landing's brand from its logo and checks a
// landing against the reference.
//
//	landingkit brand  <logo.svg>          print the [params.brand] block
//	landingkit verify <site> <ref-site>   text AND pixel check against the reference
//	landingkit links  <built-site-dir>    every link in the built site resolves
//	landingkit check  <built-site-dir>    no footer or hero text repeats the mantra strip; the hero's buttons are in the kit's order
//
// It is the Go port of the kit.py that lived beside the partials before they
// became a Hugo module. `brand` is ported rule for rule and its output is
// tested byte for byte against kit.py's on ten of the fleet's logos.
package main

import (
	"fmt"
	"io"
	"os"
)

const usage = `usage:
  landingkit brand  <logo.svg>                print [params.brand], read from the logo's gradient
  landingkit verify [flags] <site> <ref>      the <style> block and the page background against the reference
  landingkit links  [flags] <built-site-dir>   every http(s) link in the built site resolves
  landingkit check  <built-site-dir>           fails when the footer or hero sub-text repeats what the mantra strip says, or the hero's buttons are out of the kit's order (docs, try, rest)

run 'landingkit verify -h' for the verify flags
`

// exit is os.Exit, replaced by the tests so that main itself is exercised.
var exit = os.Exit

func main() { exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "brand":
		return cmdBrand(args[1:], stdout, stderr)
	case "verify":
		return cmdVerify(args[1:], stdout, stderr)
	case "links":
		return cmdLinks(args[1:], stdout, stderr)
	case "check":
		return cmdCheck(args[1:], stdout, stderr)
	case "help", "-h", "-help", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	}
	fmt.Fprintf(stderr, "landingkit: unknown command %q\n%s", args[0], usage)
	return 2
}
