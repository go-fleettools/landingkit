//go:build tools

// This file never builds. It is here so that `go mod tidy` sees a use of the
// module and keeps its requirement, as in example/.
package site

import _ "github.com/go-fleettools/landingkit"
