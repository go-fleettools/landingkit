module github.com/go-fleettools/landingkit/example-i18n

// Raised with its siblings on 2026-11-15: the root module and `example/` were
// already at 1.27.4 and this one was not, so the repository read as a finding
// for weeks while looking finished in every diff.
//
// ⛔ A NESTED MODULE KEEPS ITS OWN DIRECTIVE. `gofloor` lists each go.mod of a
// repository separately, so a raise that reaches two of three leaves the third
// reporting — and nothing about the diff says which one was missed. Same shape
// as go-richdoc/html#16, where one `setup-go` step out of seven kept an old pin.
// It was the last actionable finding in this organisation:
//
//   gofloor --floor 1.27.4 go-fleettools        (2026-11-15 13:08)
//   12 Go repositories: 2 with findings, 0 unreadable
//     landingkit                FOUND  example-i18n/go.mod: below the floor
//     go-fleettools.github.io   FOUND  no govulncheck step   <- deliberate, #206
//
// ⚠ HYGIENE, NOT EXPOSURE, and the distinction is worth keeping. This module
// has NO GO PACKAGES AT ALL — `go build ./...` here answers "matched no
// packages", because it is a HUGO example whose go.mod exists to pin the kit
// through `replace github.com/go-fleettools/landingkit => ../`. It carries no
// tag of its own, so no consumer resolves it from the proxy either. The floor
// that mattered was the published one: the eight modules of this organisation
// that read BELOW at the proxy are released at 1.27.4.
//
// ⛔ So this line is raised for CONSISTENCY with its two siblings, and to stop
// a permanent finding — not for security. The better fix is in the instruments,
// and it is filed: go-fleettools/fleettools#206 and go-fleettools/modfloor#19
// both argue that "is this a Go module to judge" should be decided by BUILDABLE
// PACKAGES rather than by the presence of a go.mod. Until one of them lands,
// every Hugo-only module in the fleet reports like this one did.
//
// The number itself is GO-2026-6859 (net/http, CVE-2026-97054, fixed in 1.26.11
// and 1.27.4), read at https://vuln.go.dev/ID/GO-2026-6859.json.
go 1.27.4

require github.com/go-fleettools/landingkit v0.14.0

// Built against the checkout, as example/ is.
replace github.com/go-fleettools/landingkit => ../
