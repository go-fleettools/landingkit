package landingkit

import (
	"encoding/json"
	"flag"
	"os"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/loading-carousel.golden.html")

// exampleCarousel is what example/layouts/carousel.html gives the partial.
func exampleCarousel(t *testing.T) Carousel {
	t.Helper()
	b, err := os.ReadFile("example/data/carousel_cards.json")
	if err != nil {
		t.Fatal(err)
	}
	var cards []Card
	if err := json.Unmarshal(b, &cards); err != nil {
		t.Fatal(err)
	}
	if len(cards) != 3 {
		t.Fatalf("%d cards in the example, want 3", len(cards))
	}
	return Carousel{Cards: cards, Pictures: "/carousel/", Ext: ".svg", Label: "What landingkit does", Start: "Show the page", Fade: "loading"}
}

const golden = "testdata/loading-carousel.golden.html"

// The Go rendering of the example's carousel is the golden file, which CI
// finds, byte for byte, in the page Hugo builds from the same cards
// (TestLoadingCarouselIsHugos).
func TestLoadingCarouselGolden(t *testing.T) {
	got, err := LoadingCarousel(exampleCarousel(t))
	if err != nil {
		t.Fatal(err)
	}
	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("LoadingCarousel differs from %s:\n%s", golden, got)
	}
	for _, w := range []string{
		`<section id="lk-carousel" class="lk-car" role="region" aria-roledescription="carousel" aria-label="What landingkit does" hidden data-delay="300" data-every="4500" data-idle="6000" data-fade="loading">`,
		`aria-label="1 of 3: One stylesheet"`,
		`<img src="/carousel/2.svg" width="320" height="180" alt="" loading="lazy" decoding="async">`,
		`logo&#39;s pixels`,
		`<span class="lk-car-sr"> about Checked in pixels</span>`,
		`document.getElementById("lk-carousel")`,
		`"Start slide rotation":"Stop slide rotation"`,
		`>Show the page</button>`,
		`window.landingkitCarousel=api`,
	} {
		if !strings.Contains(string(got), w) {
			t.Errorf("the carousel lacks %q", w)
		}
	}
}

// The page Hugo built from example/ holds the Go rendering, byte for byte:
// one carousel, whichever program writes the page. CI's hugo job runs it
// with LANDINGKIT_HUGO_PAGE set to the built example/carousel/index.html.
func TestLoadingCarouselIsHugos(t *testing.T) {
	path := os.Getenv("LANDINGKIT_HUGO_PAGE")
	if path == "" {
		t.Skip("LANDINGKIT_HUGO_PAGE is not set: CI's hugo job runs this against the built example")
	}
	page, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := LoadingCarousel(exampleCarousel(t))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), string(got)) {
		t.Fatalf("%s does not hold LoadingCarousel's output", path)
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), string(want)) {
		t.Fatalf("%s does not hold %s", path, golden)
	}
}

func TestLoadingCarouselOptions(t *testing.T) {
	got, err := LoadingCarousel(Carousel{
		Cards: []Card{
			{Title: "Cells", Text: "a <b>", Href: "https://example.invalid/cells", Img: "pic.svg"},
			{Title: "Ruby", Text: "t", Href: "https://example.invalid/ruby"},
		},
		ID: "pg", Pictures: "cards/", Ext: ".svg", Suffix: "?v=1", Label: "L", Start: "Go",
		Fade: "loading", More: "More", Width: 640, Height: 360, Delay: 10, Every: 20, Idle: 30, Lang: "fr",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{
		`<section id="pg" class="lk-car"`, `aria-label="L"`,
		`data-delay="10" data-every="20" data-idle="30" data-fade="loading"`,
		`<img src="pic.svg" width="640" height="360"`,         // its own picture
		`<img src="cards/2.svg?v=1" width="640" height="360"`, // the convention
		`a &lt;b&gt;`, // text is text
		`>More<span class="lk-car-sr"> à propos de Cells</span>`,
		`aria-label="2 sur 2: Ruby"`,
		`aria-label="Arrêter le défilement des diapositives"`,
		`id="pg-start" class="btn btn-primary lk-car-start" hidden>Go</button>`,
		`document.getElementById("pg"+"-start")`,
	} {
		if !strings.Contains(string(got), w) {
			t.Errorf("the carousel lacks %q", w)
		}
	}
	// the French defaults
	got, err = LoadingCarousel(Carousel{Cards: []Card{{Title: "x"}}, Lang: "fr"})
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{`aria-label="Ce que fait ce projet"`, `>Démarrer</button>`, `>En savoir plus<`, `src="1.jpg"`} {
		if !strings.Contains(string(got), w) {
			t.Errorf("the French carousel lacks %q", w)
		}
	}
	// signal mode: the section says so; without it, it does not
	sig, err := LoadingCarousel(Carousel{Cards: []Card{{Title: "x"}}, Signal: true})
	if err != nil || !strings.Contains(string(sig), ` hidden data-signal="1" data-delay="300"`) {
		t.Errorf("signal: %v\n%s", err, sig)
	}
	if plain, _ := LoadingCarousel(Carousel{Cards: []Card{{Title: "x"}}}); strings.Contains(string(plain), ` data-signal="1"`) {
		t.Error("the default carousel carries data-signal")
	}
	// no cards, nothing
	if got, err := LoadingCarousel(Carousel{}); err != nil || got != "" {
		t.Errorf("no cards: %q, %v", got, err)
	}
}

func TestLoadingCarouselRefuses(t *testing.T) {
	for _, lang := range []string{"../en", "EN", "xx"} {
		if _, err := LoadingCarousel(Carousel{Lang: lang}); err == nil {
			t.Errorf("language %q accepted", lang)
		}
	}
	defer func(p string) { loadingCarouselPartial = p }(loadingCarouselPartial)
	loadingCarouselPartial = "{{ if }}"
	if _, err := LoadingCarousel(Carousel{}); err == nil {
		t.Error("a partial that does not parse rendered")
	}
	loadingCarouselPartial = `{{ i18n "kit_none" }}`
	if _, err := LoadingCarousel(Carousel{}); err == nil {
		t.Error("a partial that fails rendered")
	}
}

func TestHugoFuncs(t *testing.T) {
	f := hugoFuncs(map[string]string{"k": "{{ .n }}"})
	dict := f["dict"].(func(...any) (map[string]any, error))
	if m, err := dict("a", 1, "b", "c"); err != nil || m["a"] != 1 || m["b"] != "c" {
		t.Errorf("dict: %v, %v", m, err)
	}
	if _, err := dict("a"); err == nil {
		t.Error("dict with an odd count")
	}
	if _, err := dict(1, 2); err == nil {
		t.Error("dict with a number for a key")
	}
	i18n := f["i18n"].(func(string, ...any) (string, error))
	if s, err := i18n("k", map[string]any{"n": 3}); err != nil || s != "3" {
		t.Errorf("i18n: %q, %v", s, err)
	}
	if s := f["add"].(func(int, int) int)(2, 3); s != 5 {
		t.Errorf("add: %d", s)
	}
}

func TestHugoDefault(t *testing.T) {
	var nilp *int
	one := 1
	for _, c := range []struct {
		given []any
		want  any
	}{
		{nil, "d"},
		{[]any{nil}, "d"},
		{[]any{""}, "d"}, {[]any{"x"}, "x"},
		{[]any{0}, "d"}, {[]any{7}, 7},
		{[]any{0.0}, "d"},
		{[]any{false}, false}, {[]any{true}, true},
		{[]any{struct{}{}}, struct{}{}},
		{[]any{[]int{}}, "d"}, {[]any{map[string]int{}}, "d"},
		{[]any{nilp}, "d"}, {[]any{&one}, &one},
	} {
		if got := hugoDefault("d", c.given...); got != c.want {
			t.Errorf("default d %v = %v, want %v", c.given, got, c.want)
		}
	}
}

func TestTranslate(t *testing.T) {
	w := map[string]string{"plain": "Start", "ph": "{{ .n }} of {{ .m }}", "bad": "{{ .n ", "missing": "{{ .x }}"}
	for _, c := range []struct {
		key  string
		data []any
		want string
		ok   bool
	}{
		{"plain", nil, "Start", true},
		{"ph", []any{map[string]any{"n": 1, "m": 2}}, "1 of 2", true},
		{"nope", nil, "", false},
		{"bad", nil, "", false},
		{"missing", []any{map[string]any{}}, "", false},
	} {
		got, err := translate(w, c.key, c.data...)
		if got != c.want || (err == nil) != c.ok {
			t.Errorf("translate %s = %q, %v", c.key, got, err)
		}
	}
}

func TestParseWords(t *testing.T) {
	m, err := parseWords("en", "# a comment\n\nkit_a: \"A\"\n")
	if err != nil || m["kit_a"] != "A" || len(m) != 1 {
		t.Errorf("parseWords: %v, %v", m, err)
	}
	if _, err := parseWords("en", "kit_a: A\n"); err == nil || !strings.Contains(err.Error(), "i18n/en.yaml:1") {
		t.Errorf("an unquoted value: %v", err)
	}
	// every language the kit ships reads
	for _, lang := range append([]string{"en"}, kitLanguages...) {
		if m, err := kitWords(lang); err != nil || m["kit_start"] == "" {
			t.Errorf("%s: %v", lang, err)
		}
	}
}
