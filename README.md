# landingkit

The go-* landing pages' shared design, as a **Hugo module**, and the command
that derives a landing's brand from its logo and checks a landing against the
reference.

Taken from **go-fileshare.github.io**, the reference. A landing that uses this
kit renders go-fileshare's page with its own logo and its own accent colour,
and nothing else differs.

Before this repository existed, every landing **copied** the partials, and the
copies drifted: six landings carry a variant `styles.html` today. A landing
that imports the module cannot drift; it can only fall behind a version, and
Renovate proposes the bump.

| path | role |
|:--|:--|
| `layouts/partials/styles.html` | the whole design. Brand colours come from `[params.brand]`, defaulting to go-fileshare's cyan. It also emits `<meta name="darkreader-lock">` and the fork ribbon's CSS |
| `layouts/partials/theme-toggle.html` | system / light / dark |
| `layouts/partials/fork-ribbon.html` | the "Fork me on GitHub" corner ribbon |
| `layouts/partials/icons/github.html` | the GitHub mark (Octicons, MIT: `icons/OCTICONS-LICENSE`) |
| `layouts/partials/icons/book.html` | an open book (Octicons "book", MIT) for every "Read the documentation" button |
| `cmd/landingkit` | `brand` and `verify`, below |
| `example/` | the smallest site importing the module; CI builds it and verifies it |

## Importing it in a landing

The partial names are unchanged, so `{{ partial "styles.html" . }}` keeps
working. A site's own `layouts/partials/<name>` still wins over the module's,
which is how a landing keeps a partial of its own (go-fft's `icons/play.html`)
and how a deliberate override stays possible and visible.

**1. `hugo.toml`**

```toml
[module]
  [[module.imports]]
    path = "github.com/go-fleettools/landingkit"
```

**2. `go.mod`** at the root of the site. Hugo resolves modules with the `go`
command, so the version is pinned here:

```
module github.com/<org>/<org>.github.io

go 1.27.1

require github.com/go-fleettools/landingkit v0.1.0
```

and run `hugo mod get github.com/go-fleettools/landingkit@v0.1.0` to write
`go.sum`. A nested Go module (go-fft's and go-ndarray's `playground/`) is not
affected: it has its own `go.mod`, which makes it a separate module.

**3. `kit.go`** next to it. ⛔ Do not skip this one:

```go
//go:build tools

package site

import _ "github.com/go-fleettools/landingkit"
```

A Hugo site has no Go code, so `go mod tidy` finds nothing that uses the module
and **deletes the requirement** -- measured on `example/`: with this file the
`require` survives tidy, without it the line is gone. The fleet's Renovate
presets set `postUpdateOptions: ["gomodTidy"]`, so every bump would arrive as a
pull request that silently removes the module, and the grouped dependency
branch is auto-merged. The build tag keeps the file out of every build; it
exists only to be seen by tidy. CI checks this on `example/`.

**4. The Pages workflow needs Go.** Add `actions/setup-go` before `hugo` in
every job that builds the site (`go-version-file: go.mod`).

**5. Delete the copied partials** -- `styles.html`, `theme-toggle.html`,
`fork-ribbon.html`, `icons/github.html`, `icons/book.html`,
`icons/OCTICONS-LICENSE` -- and prove the built site is byte-identical before
and after (`hugo` on both commits, `diff -r public-before public-after`).

## How the brand colours are derived

The brand colour is **read from the logo's gradient**, never chosen, and it is
the only thing an organisation sets. Backgrounds, surfaces, greys and borders
belong to the sheet and cannot be configured: go-fft and go-ndarray once grew
`surfaces` parameters that tinted their backgrounds emerald (`#f0fdf9` light,
`#04130d` dark, where the reference is `#ffffff` and `#0b0e14`), and this kit
removes the means to do that.

```
landingkit brand static/img/logo.svg >> hugo.toml   # then remove the old block
```

The first two `stop-color="#rrggbb"` of the SVG are the gradient; the lighter
one (by WCAG relative luminance) is `bright`, the darker one is `base`.

| key | rule |
|:--|:--|
| `base` | the gradient's DARK stop: it carries text and links on white |
| `bright` | the gradient's LIGHT stop: the accent in dark mode |
| `deep` | `base` darkened by 39 % |
| `shadow`, `glow`, `wash` | `base` at alpha .7, .6, .10 |
| `washDeep` | `deep` at .06 |
| `chipBg`, `chipBorder` | `bright` mixed into white at 12 % and 30 % |
| `chipFg` | `deep` |
| `chipBgDark`, `chipBorderDark` | `bright` at .14 and .32 |
| `chipFgDark` | `bright` mixed with 45 % white |

The block also records both contrast ratios, so a logo whose dark stop is too
light to carry text on white is visible in the diff that introduces it.

This is the Go port of the `kit.py` that lived beside the partials. Its output
is tested **byte for byte** against kit.py's on the ten landings' logos and two
synthetic ones (`cmd/landingkit/testdata/logos`), and was cross-checked on 1500
random gradients with no difference. Two things had to be ported exactly:

- Python's `round()` is **half to even**. Ties are real: a bright channel that
  is a multiple of 10 puts `chipBorder` on an exact .5. Rounding half up
  differed from kit.py on 725 of the 1500 random logos, and
  `testdata/logos/tie-dark-first.svg` keeps that from coming back.
- Go may fuse `x*y + z` into one instruction on arm64, ppc64 and s390x, which
  skips a rounding Python performs, so each such product is wrapped in an
  explicit `float64()`. A mutant without the guards matched kit.py on all 1500
  logos on darwin/arm64, so this is insurance the spec calls for rather than the
  fix of an observed difference.

## The fork ribbon

```
{{ partial "fork-ribbon.html" "https://github.com/<org>/<repo>" }}
```

right after `</header>`. It is the "Fork me on GitHub" ribbon (github.blog,
2008) drawn in CSS with the brand gradient instead of the original PNGs, so it
follows the theme. It REPLACES the nav bar's GitHub button, and the hero has no
"Browse the source" button: all three led to the same place.

## darkreader-lock

The page draws its own dark theme (`prefers-color-scheme` and the toggle). The
Dark Reader extension otherwise repaints it on top -- the hero's brand wash
vanished under it on go-fft while the same CSS showed on go-ndarray -- so
`styles.html`, which every landing includes in `<head>`, emits
`<meta name="darkreader-lock">` and Dark Reader stands down.

## Verifying a landing: text AND pixels

```
landingkit verify [-hugo "pkgx hugo@0.167.0"] [-chrome PATH] [-pixels=false] [-out DIR] <site> <ref-site>
```

It builds both sites with Hugo and fails unless both hold:

1. **Text:** the built `<style>` block equals the reference's once each site's
   brand values (and the sheet's cyan defaults) are masked. Any other
   difference -- a tinted surface, an extra rule -- fails.
2. **Pixels:** headless Chrome renders both pages, light AND dark, and the page
   background is sampled at nine points in the gutters: every sample must equal
   the reference's exactly. The theme is forced in both directions (headless
   Chrome otherwise inherits the host's appearance, and a first version compared
   dark against dark and called it light), and the reference itself must look
   like the requested theme, or the capture is declared wrong.

The pixel check is the one that found the defect: a colour-normalised CSS diff
had hidden go-fft's emerald backgrounds. It runs Chrome directly and decodes the
PNG with the standard library, so the module has no dependency at all.
`-pixels=false` runs the text check alone where there is no browser.

Builds and screenshots go to a new temporary directory, or `-out`; either is
**refused inside a git work tree**, because a `.gitignore` entry does not stop
`git add -f` or a fresh clone from publishing a render.

CI proves both directions on every pull request: `example/` against itself
must pass, and a copy of it with the background tinted must fail both checks.

### What it does not see

Only the `<style>` block and the background. A landing whose `index.html`
differs -- a longer headline, an extra band -- passes; that is what
[landingdiff](https://github.com/go-fleettools/landingdiff) counts. Use both.

## Licence

BSD-3-Clause. The Octicons in `layouts/partials/icons/` are MIT, see
`layouts/partials/icons/OCTICONS-LICENSE`.
