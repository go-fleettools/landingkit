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
| `layouts/partials/icons/browser.html` | a browser window (Octicons "browser", MIT) for "Try it in your browser": `{{ partial "icons/browser.html" . }}` inside the `btn btn-try` link, white on the red like the text (v0.10.0) |
| `layouts/partials/icons/book.html` | an open book (Octicons "book", MIT) for every "Read the documentation" button |
| `layouts/partials/loading-carousel.html` | optional: a carousel of what a project does, shown while a page loads something large (a WebAssembly playground); `landingkit.LoadingCarousel` renders the same file for a page Hugo does not build (v0.12.0) |
| `layouts/partials/mantra.html`, `mantra-items.html` | the strip's chips as text (rendered from `theme-toggle.html`), and the checked list of them |
| `layouts/partials/mantra-on.html` | whether the page shows the mantra strip, for a landing's own licence line (shown only without the strip) |
| `layouts/partials/hero-carousel.html` | optional: the hero's terminal as a carousel of cards (picture, terminal, caption); with no cards, the terminal byte for byte; more than one terminal with `terminal = "<name>"` (v0.11.0) |
| `i18n/<lang>.yaml` | the words the partials write (labels, aria-labels, "Read more"…), in English, French, German, Spanish, Italian, Polish, Brazilian Portuguese, Russian, Ukrainian, Simplified Chinese, Korean and Japanese (v0.8.0; Italian to Ukrainian in v0.11.0) |
| `layouts/partials/btn-try-style.html` | opt-in: the red `btn btn-try` button (white text, hover and focus states); a site that does not call it is unchanged (v0.9.0) |
| `layouts/partials/cta.html` | the hero's call-to-action row, rendered from `[[params.cta]]` (`kind` = docs, try, github or link) in the kit's order: docs, try, then the rest (v0.14.0) |
| `layouts/partials/language-selector.html` | the language combo of the top bar; nothing on a site with one language (v0.8.0) |
| `layouts/partials/i18n-head.html`, `i18n-style.html` | the hreflang alternates, and the selector's and the CJK languages' sheet, a second `<style>`; nothing on a site with one language (v0.8.0) |
| `layouts/partials/html-lang.html`, `language-label.html` | a Language's locale (`zh-Hans`) for `lang=`, and its name in its own language (v0.8.0) |
| `cmd/landingkit` | `brand`, `verify` and `links`, below |
| `workflows/links.yml` | a template for a landing's link check, running `landingkit links` at the version the landing pins |
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

require github.com/go-fleettools/landingkit v0.3.0
```

and run `hugo mod get github.com/go-fleettools/landingkit@v0.3.0` to write
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
| `statsEnd`, `bandText` | `bright` mixed into white at 24 % and 16 %: the stats figures' gradient end and the CTA band's paragraph (v0.3.0) |

The block also records both contrast ratios, so a logo whose dark stop is too
light to carry text on white is visible in the diff that introduces it.

`statsEnd` and `bandText` were the last cyan literals in the rules (`#cdf1fa`,
`#d8f3fb`). The sheet keeps those as their defaults, so a site without
`[params.brand]` renders the same `<style>` block as before -- checked by
building `example/` before and after and diffing. A landing whose block was
generated before v0.3.0 keeps the cyan for those two until it adds the two
lines `landingkit brand` now prints. Both fractions are multiples of 4 %, so no
channel lands on a .5 and the rounding mode cannot matter; the test checks every
channel value against integer arithmetic, and the logos' two new lines against
a reference computed from kit.py's `bright` without calling the Go rule.

### Gradient text: `--grad-text` (v0.5.1)

The hero's accent line, the performance hero's accent and the stat figures
are text painted with a gradient. `--grad` carries white text on buttons and
bands, so it cannot be used for text that sits on the page's own background.
In v0.5.0 the accent line was painted with `--grad` (base→deep) in dark mode
too, and was nearly invisible: go-slides' indigo on `#0b0e14` reached
**1.2:1**, and no organisation reached 4.5:1. `--grad-text` has a value for
each theme, built from the brand tokens with `color-mix()`:

| theme | stops |
|:--|:--|
| light | `base` darkened by 15 %, then `deep` |
| dark | `chipFgDark`, then `bright` with 30 % white |

A browser without `color-mix()` gets `base`→`deep` in light mode and a
plain `chipFgDark` in dark mode.

`TestGradientTextContrast` reads the sheet, resolves it for the kit's
defaults, for every logo in `testdata/logos` and for go-slides' indigo, and
requires every stop to reach **4.5:1** against `--bg` in light mode and in
both dark modes (system and toggle). That is SC 1.4.3's threshold for body
text; the accent is large text, which needs only 3:1. Today's lowest stops:

- light: 4.95:1 (go-widgets' teal, whose `base` alone is 3.74:1);
- dark: 5.66:1 (go-slides).

The test fails on v0.5.0's sheet.

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

## The loading carousel

A WebAssembly playground downloads megabytes before its editor is ready.
`loading-carousel.html` shows, during that wait, cards of what the project
does: a picture, one line, and a link to the documentation. It is fed by a
list of cards and costs nothing until it shows.

```go-html-template
{{ partial "loading-carousel.html" (dict
     "cards"    site.Data.playground_cards  // [{title, text, href, img?}]
     "pictures" "/playground/cards/"        // a card without img: <pictures><n><ext><suffix>
     "ext" ".jpg" "suffix" "?v=<deploy sha>"
     "label"    "What go-slides does"        // the region's name
     "start"    "Start the playground"       // the button that waits for a reader
     "fade"     "pg-placeholder") }}         // the element faded away (default: the carousel)
```

The page's loader tells it two things:

```js
landingkitCarousel.downloaded();          // the large download is over
landingkitCarousel.done(reveal, onWait);  // ready: reveal() shows the page
```

- **Shown only while the download is still going after 300 ms** (`delay`). A
  cached module is downloaded at once, so it never flashes the carousel,
  whatever its compile then takes. Measuring elapsed time instead flashed it
  on a slow CI runner.
- **WAI-ARIA APG carousel pattern:**
  - a labelled region, the rotation button first;
  - slides as labelled groups;
  - dots as buttons, the current one `aria-disabled`;
  - `aria-live` off while rotating;
  - hover pauses, focus stops, and only the button starts it again.
- **`prefers-reduced-motion`**: no rotation, no animation, no fade.
- **At the end** it fades away, unless the reader is in it (hovering,
  focused, or a click in the last 6 s): then `onWait()` is called and a
  Start button waits.
- **Nothing is fetched unless it shows**: the pictures are lazy, and only
  the next card's is preloaded. `landingkitCarousel.state` exposes `shown`,
  `downloaded`, `advanced` and `startShown` for a test.

### On a page Hugo does not build (v0.12.0)

A playground page written by a Go program cannot call a partial.
`landingkit.LoadingCarousel` executes **the same partial file** with Go's
`html/template` and the few Hugo functions it calls (`default`, `add`,
`dict`, `i18n`, `safeHTML`, `safeJS`), so there is one carousel, not a copy:

```go
html, err := landingkit.LoadingCarousel(landingkit.Carousel{
	Cards:    []landingkit.Card{{Title: "Cells", Text: "…", Href: "https://…"}},
	Pictures: "cards/", Ext: ".svg", Suffix: "?v=" + version,
	Label:    "What go-notebook does", Start: "Open the notebook",
	Fade:     "loading",
	Lang:     "en", // the kit's words, from i18n/<lang>.yaml
})
```

A zero field takes the partial's default, as an absent key does in Hugo. The
page supplies the variables `styles.html` defines (`--line`, `--bg-soft`,
`--ink`, `--slate`, `--accent`) and calls `downloaded()` and `done()` as
above. `testdata/loading-carousel.golden.html` is the Go rendering of
`example/`'s cards, and CI's hugo job checks that the page Hugo builds from
them holds it **byte for byte** (`TestLoadingCarouselIsHugos`).
go-notebook/playground's `cmd/build` uses it.

`example/carousel/` uses it with a pretend download (`?cached=1` for a
cached one). go-slides.github.io's playground uses it, and
`browsertest/tour.cjs` there drives it in Chrome: throttled, hovered,
reduced motion and cached. The **go-tex** and **go-fft** playgrounds share
the same loader and can use it the same way.

## The hero carousel (v0.5.0)

The hero's terminal picture can become a carousel of cards: a picture, the
terminal itself, or a short caption with a link. Adopting it changes
nothing until cards are listed:

1. move the hero's `<div class="term">…</div>` into
   `layouts/partials/hero-terminal.html`;
2. call `{{ partial "hero-carousel.html" . }}` where it was.

With no cards the partial renders `hero-terminal.html` and nothing else.
CI builds the same page both ways and compares them byte for byte. Cards
come from the page's `hero.cards`, else from the site's:

```toml
[params.hero]
  interval = 6000                     # ms between cards
  label = "go-slides in pictures"     # the region's name
[[params.hero.cards]]
  terminal = true                     # the landing's hero-terminal.html
[[params.hero.cards]]
  terminal = "ruby"                   # a second terminal: hero-terminal-ruby.html (v0.11.0)
  title = "Ruby cells"                # the card's name for assistive technology
[[params.hero.cards]]
  img = "/img/hero/editor.png"        # with its intrinsic size
  width = 1200
  height = 750
  alt = "The playground's editor"
  caption = "Write a deck in your browser"
  href = "/playground/"
[[params.hero.cards]]
  title = "VS Code"                   # a caption card
  text = "The same preview beside your Markdown."
  href = "https://…"
  more = "Install"                    # the link's text (default "Read more")
```

- **More than one terminal (v0.11.0).** `terminal = "<name>"` renders the
  landing's `layouts/partials/hero-terminal-<name>.html`, so a second example
  is a real terminal, its text selectable and searchable, rather than a
  picture of one (go-notebook.github.io shows a notebook with Ruby cells and
  what it printed). The name selects a template, so it must be lower-case
  letters, digits and single hyphens; anything else (`../x`) fails the build.
  `terminal = true` is unchanged, byte for byte.
- **No layout shift.** Every card sits in the same grid cell, so the hero
  takes its tallest card's size from the first paint.
- **LCP no worse.**
  - The first card renders as written: a first image is eager and
    `fetchpriority="high"`.
  - The other cards' images wait in `data-src`. The next card's image is
    fetched after the page's `load`, and each later one just before it
    shows.
- **WAI-ARIA APG carousel pattern**, as for the loading carousel. In
  addition, ←/→ move between cards, and hidden cards are `aria-hidden` and
  out of the tab order.
- **`prefers-reduced-motion`**: no rotation and no fade.
- `window.heroCarousel` exposes `show(n)`, `index` and `stopped`, for a test.
  go-slides.github.io's `browsertest/hero.cjs` drives the carousel in Chrome.

## The mantra strip (v0.5.0)

Under the top bar of every page, two chips state what the fleet's code is:
`LICENSE=BSD-3-Clause` and `CGO_ENABLED=0`. The licence uses its SPDX
identifier.

**Since v0.7.0 the chips are text.** `mantra.html` renders them as
`<code class="mantra-chip">` elements. `theme-toggle.html`, which every
landing calls inside its nav bar, calls it, so a landing needs no change.
Up to v0.6.0 they were CSS generated content (`.nav::before{content:"…"}`).
That looked right in a screenshot, but nothing could select, search, index
or reliably read it, and a page check reading text found neither value. CI
and `TestMantra` now fail if the chips stop being text nodes.

```toml
[params.mantra]
  show = false                                        # an organisation for which it is not true
  items = ["LICENSE=Apache-2.0", "CGO_ENABLED=0"]     # or other values: at most four
```

- **On by default.** Turn it off with `show = false`, or replace the values
  with `items`.
  - Items are checked at build time: at most four, made of
    `A-Za-z0-9_.=:+/, -`, and up to 40 characters each.
- **Data under another licence gets its own chip (v0.6.0).** The code chip
  stays, and a third or fourth chip names the data and its licence:

  ```toml
  [params.mantra]
    items = ["LICENSE=BSD-3-Clause", "CGO_ENABLED=0", "fonts: OFL-1.1, Bitstream-Vera"]
  ```

  The third and fourth chips are drawn on the nav's `.wrap`. On a screen
  too narrow for all of them on one row, each data chip takes a row of its
  own under the code's two.
- **Said once.** With the strip on, a landing does not repeat the licence or
  `CGO_ENABLED=0` in its footer or body. A mention the landing keeps for
  pages without the strip asks the kit:

  ```go-html-template
  {{ if not (partial "mantra-on.html" .) }}BSD-3-Clause · CGO_ENABLED=0{{ end }}
  ```

  `styles.html` asks `mantra-on.html` the same question, so the two always
  agree. No kit partial mentions either value outside the strip, and a test
  holds that.
- **Turned off for one page** with `mantra: false` in its front matter.
- **Always off on a `layout: playground` page**, whose stage is fixed under
  the bar.
- **The mantra off gives the page of v0.4.0, byte for byte.** CI checks this
  against the base branch.

Only claim what is true. Before turning the strip on, check the
organisation's licences (`gh api repos/O/R/license`) and its cgo use.
`go run github.com/go-fleettools/fleettools/cmd/mantracheck@latest <org>`
does both checks.

## The red button: `btn-try` (v0.9.0)

A call to action that must stand out ("Try it in your browser"). It is opt-in:
call `{{ partial "btn-try-style.html" . }}` after `styles.html` (a second
`<style>`, so `landingkit verify` still reads the kit's sheet first) and give
the link `class="btn btn-try"`, with the icon, as the documentation button has
its book: `<a class="btn btn-try" href="…">{{ partial "icons/browser.html" . }}Try it in your browser</a>`
(decorative, `aria-hidden`; v0.10.0). A site that does not call it renders as it did.

Red `#dc2626`, white text 4.83:1; hover `#c52222`, 5.79:1; keyboard focus a
2px outline in `--ink`, offset 2px, at least 3:1 against the page and the
button in both themes. `TestBtnTryContrast` computes all of these from the
sheet and `TestBtnTryIsOptIn` keeps the variant out of every other partial.

## The hero's call-to-action row: the kit's order (v0.14.0)

The two coloured buttons under the title are the documentation button (the
primary, blue `btn-primary`) and, where there is a playground, the red
`btn-try`. **The kit's order is: docs, then try, then the plain `btn-ghost`
links.** go-slides.github.io (`layouts/index.html`, the docs button above the
`btn-try` one) and `example/` already followed it; nothing wrote it down, and
go-wasmcontainers' landing had the red button first.

It is enforced twice:

- **The kit renders the row.** List the buttons in `[[params.cta]]`, in any
  order, and put `{{ partial "cta.html" . }}` where the row goes:

  ```toml
  [[params.cta]]
    kind  = "docs"      # docs | try | github | link
    href  = "https://example.github.io/docs/"
    label = "Read the documentation →"
  ```

  It comes out `docs`, `try`, `github`, `link`, whatever the order of the list;
  an unknown `kind`, or a button without `href` and `label`, fails the build.
  The words are the site's (params are per language). A `try` button still
  needs `{{ partial "btn-try-style.html" . }}` after `styles.html`.
- **`landingkit check` reads the built page.** For a landing that writes its
  row by hand, it fails when a `btn-primary` comes after a `btn-try` or a
  `btn-ghost`, or a `btn-try` after a `btn-ghost`, in a `.cta` row. The
  success line says how many rows it read. `TestCheckFailsTryBeforeDocs` plants
  the old go-wasmcontainers row and must fail; the rows in the kit's order must
  pass.

## Languages (v0.8.0)

A landing in one language is **exactly the page it was**: the words the
partials write moved into `i18n/en.yaml`, and the page built from them equals
the previous release's byte for byte (CI builds `example/` with both and
runs `diff -r`; `TestEnglishIsTheOldText` pins the strings). The other ~290
landings change by nothing, and need no change.

A landing that wants several languages declares them in `hugo.toml` and
writes a page per language; the kit does the rest:

```toml
defaultContentLanguage = "en"
defaultContentLanguageInSubdir = false   # English at the root, /fr/… for the others

[languages.en]
  label = "English"
  weight = 1
[languages.zh]
  label = "简体中文"      # each language in its own name
  locale = "zh-Hans"     # lang= and hreflang=
  weight = 2
```

- **The selector.** `theme-toggle.html`, which every landing calls inside its
  nav bar, renders `language-selector.html` before the theme button: a
  disclosure with the current language's name and a menu of every language,
  each linking to **the same page** in that language, or to that language's
  home when the page has no translation (Escape closes it, the arrows move in
  it, a click outside closes it). On a phone only the 🌐 shows.
- **`lang` and `hreflang`.** `styles.html` (in `<head>`) adds
  `<link rel="alternate" hreflang="…">` for each translation of the page, with
  `x-default` for the first language by weight. The landing's own
  `<html lang="en">` becomes
  `<html lang="{{ partial "html-lang.html" site.Language }}">`: the language's
  `locale` (`zh-Hans`, so that a browser draws the Han characters Chinese and
  Japanese share with the right glyphs), else its key.
- **The kit's words** (`i18n/<lang>.yaml`, keys `kit_…`) exist in English,
  French, German, Spanish, Italian, Polish, Brazilian Portuguese (`pt`),
  Russian, Ukrainian, Simplified Chinese, Korean and Japanese. A site's
  own `i18n/<lang>.yaml` overrides any of them; a language the kit has no file
  for falls back to English. `TestEveryLanguageHasEveryKey` holds the files to
  the same keys and the same placeholders.
- **CJK.** The language sheet gives `zh-Hans`, `ja` and `ko` a font stack that
  names the CJK system fonts (PingFang, Hiragino, Noto Sans CJK, Apple SD Gothic
  Neo, Malgun Gothic, Yu Gothic, Microsoft YaHei), and lets long runs wrap.
- **`landingkit verify` still passes.** The selector's and the CJK sheet is a
  **second** `<style>` block, which `verify` does not read (CI verifies
  `example-i18n/` against `example/`).
- **Hugo 0.158 or later** for a multilingual landing (`label`, `locale`,
  `hugo.Sites`). One language needs nothing new.

What a landing's own prose says in each language is the landing's: its
`i18n/<lang>.yaml` and per-language params. `example-i18n/` is the smallest
multilingual site.

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
   background is sampled in the gutters, at the points where nothing but the
   page background is painted: every sample must equal the reference's
   background exactly. The theme is forced in both directions (headless
   Chrome otherwise inherits the host's appearance, and a first version compared
   dark against dark and called it light), and the reference itself must look
   like the requested theme, or the capture is declared wrong.

The pixel check is the one that found the defect: a colour-normalised CSS diff
had hidden go-fft's emerald backgrounds. It runs Chrome directly and decodes the
PNG with the standard library, so the module has no dependency at all.
`-pixels=false` runs the text check alone where there is no browser.

### Which pixels are the background

Up to v0.2.0 the samples were nine fixed points, `(4, 900)`, `(4, 1200)`
and so on. A tall full-width band at one of those heights -- go-embedded-ruby's
`.strip`, on `--bg-soft` -- then read as a different background and gave a
FAIL that was the band, not the page, on `main` as on any branch.

Now a probe (`cmd/landingkit/probe.js`) runs first, in a copy of the page
(`landingkit-probe.html` beside `index.html`, in the render directory; the page
itself is screenshotted unchanged). The candidates are the two gutters, x = 4
and x = 1275, every 50 px from y = 100. The probe keeps a candidate only when
**no element paints there**: it walks every element, not only those
`elementsFromPoint` returns, because that list skips `pointer-events:none`
and a transparent element on top would hide a painted one beneath. An element
paints at a point when the point is inside its box and it has a background
colour or image, or the point is on one of its borders, inside its box-shadow
or outline, inside replaced content (an image, an SVG, a form control), or
inside a `::before`/`::after` with a background. `html` and `body` ARE the
page background and never exclude anything. Chrome prints the result with
`--dump-dom`.

Then, per theme:

- fewer than 8 background points on either page is a FAIL: a page whose every
  gutter is covered is not passed for lack of evidence;
- the reference's points must all be one colour, or the probe let something
  through and the check says so instead of judging;
- every point the site kept must equal that colour.

`--dump-dom`'s window keeps room for browser chrome that `--screenshot`'s does
not (2313 px of viewport for 2400 asked, measured on Chrome 154), and a page
laid out at another height is another page wherever it uses `vh`. The probe
reports its viewport; the first probe learns the difference and every later
one asks for a window that much taller, so the probe and the screenshot lay
the page out at 1280 x 2400 both.

Builds and screenshots go to a new temporary directory, or `-out`; either is
**refused inside a git work tree**, because a `.gitignore` entry does not stop
`git add -f` or a fresh clone from publishing a render.

CI proves both directions on every pull request, with real Hugo and real
Chrome: `example/` against itself must pass; a copy of it with a tall
full-width `.strip` band where the old points sampled must pass too (the false
FAIL above); and a copy with the background tinted `#f0fdf9` must fail both
checks.

## Checking every link: `landingkit links`

```
landingkit links [-attempts 3] [-wait 3s] [-timeout 25s] public
```

Every `http(s)` URL in the **built** site must answer 2xx or 3xx, redirects
followed. It reads the build rather than the layouts, because the layouts hold
template expressions (`https://github.com/<org>/{{ .name }}`), not URLs.
`img.shields.io` is skipped, since it renders anything asked of it. Each URL
gets three tries, because a single try detects flakes rather than dead links.
Binary files (a wasm, an image) are not scanned.

⛔ **A github.com blob link is checked on the raw host.** github.com answers
a program's request for `https://github.com/<o>/<r>/blob/<ref>/<path>` with
**503** most of the time: 3 attempts out of 3 on every Actions run, and 7 of 8
locally with curl's default User-Agent, while githubstatus was green. So a
checker that asks github.com goes red on any such link. A blob link claims
"this file exists at this ref", and
`https://raw.githubusercontent.com/<o>/<r>/<ref>/<path>` answers exactly that:
200 for a file, 404 for a missing one. `#anchor` and `?query` are stripped,
and every other URL is fetched unchanged. The output names the URL that was
probed (`(via …)`). This is the rewrite go-ndarray/docs#30 made in its own
checker. The tests prove an existing blob path passes and a missing one fails,
without ever asking github.com. CI also runs `links` on `example/`, which
links to a blob page, live.

`workflows/links.yml` is a template for a landing's link check. It runs
`go run github.com/go-fleettools/landingkit/cmd/landingkit links public`
without a version, so inside a landing that requires this module the version
comes from the landing's `go.mod`: Renovate's bump of the partials bumps the
checker with them.

### A rate limit is not a broken link (v0.7.1)

A 429 Too Many Requests ([RFC 6585 §4](https://www.rfc-editor.org/rfc/rfc6585#section-4)),
or a 503 that carries a `Retry-After` ([RFC 9110 §10.2.3](https://www.rfc-editor.org/rfc/rfc9110#section-10.2.3)),
means "not now", not "not found". Such a link is retried:

- each wait is the larger of an exponential backoff (`-wait`, then twice
  that, and so on) and what `Retry-After` asks for, in seconds or as an
  HTTP-date;
- no wait is longer than `-max-retry-after` (60 s by default).

If every try is rate limited, the link is reported `WARN … unknown`, with a
warning, and it does not fail the check. A 404, a 410, a 503 without
`Retry-After`, or no answer at all still fail. There is no allowlist: the
case that prompted this (go-llmflow's link to docs.vllm.ai, which answered
429 to the CI runner) needed none.

### What it does not see

Only the `<style>` block and the background. A landing whose `index.html`
differs -- a longer headline, an extra band -- passes; that is what
[landingdiff](https://github.com/go-fleettools/landingdiff) counts. Use both.

## Saying it once: `landingkit check` (v0.13.0)

```sh
landingkit check public
```

The mantra strip already says the licence and `CGO_ENABLED=0`. A landing that
also hard-codes them in its footer or in its hero sub-text (`<p class="sub">`)
shows every fact twice. `check` reads the BUILT site, finds the chips (the
strip's generated content, or the `mantra-chip` markup of earlier versions),
and exits 1 for each page whose footer or hero sub-text repeats a chip, its
licence value, or "Pure Go" when the strip carries `CGO_ENABLED=0`. A page
without the strip is not checked: its own mention is then the only one. The fix
is the pattern of `example/layouts/index.html`:

```go-html-template
{{ if not (partial "mantra-on.html" .) }}BSD-3-Clause · CGO_ENABLED=0{{ end }}
```

The heading is not checked: "in pure Go" in an `h1` is the project's identity,
not the strip's fact. Add it to the landing's links workflow, next to
`landingkit links public`.

## No partial ends with a newline

A partial is pasted where it is called, and its final newline comes with it.
`icons/github.html` had one (and `icons/book.html` did not): inside
`<a class="btn">{{ partial "icons/github.html" . }} Source</a>` it put a
whitespace text node between the `<svg>` and the text. That is harmless in
`.btn`, an inline-flex with a gap (measured identical to 0.01 px), but a
landing that replaced its own copy, which had no newline, saw its built page
differ by a byte -- and a byte-for-byte comparison of two builds is how a
migration to the kit is proved. So since v0.3.0 **no partial ends with a
newline**, the block partials included, and `TestNoPartialEndsWithANewline`
fails on one that does. An editor that adds a final newline on save will be
caught there.

## What a landing says: what a module is FOR, never a count from a copy

A landing is a page written once and read for years, so anything on it that
is **copied** from somewhere else -- a list of the organisation's modules, a
count ("39 verbs"), a repository's description -- can only go stale. The
fleet's drift checks keep finding exactly that: verb lists missing the latest
verbs, "39 verbs" when there were 41, descriptions that the repository had
since rewritten. Nothing about the landing was wrong when it was written; it
became wrong by standing still.

So a landing says **what a module is for**, in words that stay true while the
module grows: "the `ls`, `cp` and `mount` of a filesystem you have only as an
image", not the list of its 41 verbs. When a figure or a list matters, link to
the place that owns it (the README, the package index, the releases page)
rather than restating it.

A possible future feature, **not built**: render the repositories'
descriptions from the GitHub API at build time (a Hugo `resources.GetRemote`
of `api.github.com/orgs/<org>/repos`, or a data file a workflow refreshes),
so that the one list a landing legitimately shows comes from its source on
every deploy instead of from a copy.

## Licence

BSD-3-Clause. The Octicons in `layouts/partials/icons/` are MIT, see
`layouts/partials/icons/OCTICONS-LICENSE`.
