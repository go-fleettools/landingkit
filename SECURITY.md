# Security policy

## Reporting a vulnerability

Please report it privately through GitHub's
[private vulnerability reporting](https://github.com/go-fleettools/landingkit/security/advisories/new)
for this repository, not in a public issue. You will get an answer within a
week.

## What is in scope

- **The partials.** They are rendered into every landing that imports this
  module, so a defect here is a defect on all of them at once. Values from
  `[params.brand]` are passed through `safeCSS`, which marks them as trusted
  CSS: the site's own `hugo.toml` is trusted input, and nothing a visitor
  sends reaches it. A way to get untrusted text into those values is in scope.
- **`landingkit verify`** runs the `hugo` and Chrome binaries it is given on
  the sites it is given, and writes builds and screenshots to a directory it
  refuses to create inside a git work tree. It opens no network port and reads
  no credential. Chrome loads the built page from `file://`, so a page that
  fetches remote resources fetches them from the machine running the check.
- **`landingkit brand`** reads one SVG file with a regular expression; it
  does not parse XML and follows no reference in it.

## Supported versions

Only the latest tag. Landings pin a version in their `go.mod`, and Renovate
proposes the bump.
