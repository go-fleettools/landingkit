// Package landingkit is the go-* landing pages' shared design, as a Hugo
// module, plus the one stylesheet a Go program may need to read.
//
// A landing imports this repository in its hugo.toml:
//
//	[module]
//	  [[module.imports]]
//	    path = "github.com/go-fleettools/landingkit"
//
// and Hugo mounts layouts/partials/ under the site's own, so
// {{ partial "styles.html" . }} resolves to this module's copy unless the site
// overrides it. The partials are the design; this Go package exists for two
// smaller reasons:
//
//   - `landingkit verify` masks the sheet's own cyan defaults, and it reads
//     them from the sheet it was built with rather than from a copy;
//   - a landing's go.mod has no Go code that needs this module, so
//     `go mod tidy` (which Renovate runs after every bump) would DELETE the
//     requirement. A landing keeps it by importing this package from a file
//     that never builds; see the README.
package landingkit

import _ "embed"

// Styles is layouts/partials/styles.html, byte for byte.
//
//go:embed layouts/partials/styles.html
var Styles string
