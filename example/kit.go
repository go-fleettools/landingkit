//go:build tools

// This file never builds. It is here so that `go mod tidy` -- which Renovate
// runs after every bump -- sees a use of the module and keeps its requirement:
// a Hugo site has no Go code of its own, and tidy deletes what nothing imports.
package site

import _ "github.com/go-fleettools/landingkit"
