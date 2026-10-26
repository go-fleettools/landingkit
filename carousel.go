package landingkit

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"reflect"
	"regexp"
	"strings"
	ttemplate "text/template"
)

// The loading carousel for a page Hugo does not build.
//
// A WebAssembly playground's page is often written by a Go program
// (go-notebook/playground's cmd/build), so it cannot call a Hugo partial.
// LoadingCarousel executes the partial itself, layouts/partials/
// loading-carousel.html as Hugo mounts it, with Go's html/template (which
// Hugo's templates are) and the few Hugo functions it calls, so there is one
// carousel: what a landing renders and what a Go page renders are the same
// bytes for the same cards (TestLoadingCarouselIsHugos holds that against a
// page Hugo built).

//go:embed layouts/partials/loading-carousel.html
var loadingCarouselPartial string

//go:embed i18n/*.yaml
var i18nFiles embed.FS

// Card is one card of the loading carousel: a picture, a title, one line and
// a link to read more.
type Card struct {
	Title, Text, Href string
	// Img is the picture's URL; empty, it is Carousel.Pictures + n + Ext +
	// Suffix, n counting from 1.
	Img string
}

// Carousel is what the partial's dict holds. A zero field takes the
// partial's default, as an absent key does in Hugo.
type Carousel struct {
	Cards []Card
	// ID is the section's id (default "lk-carousel"); the Start button is
	// ID+"-start".
	ID string
	// Pictures, Ext (default ".jpg") and Suffix make the URL of a card
	// without Img.
	Pictures, Ext, Suffix string
	// Label names the region (default: "What this project does", in Lang).
	Label string
	// Start is the button that waits for a reader in the carousel.
	Start string
	// Fade is the id of the element faded away at the end (default: the
	// carousel).
	Fade string
	// More is the link text of each card (default "Read more").
	More string
	// Width and Height are the pictures' (default 320×180).
	Width, Height int
	// Delay is how long the download may last before the carousel shows
	// (default 300 ms); Every the rotation's period (4500 ms); Idle how long
	// after a click the reader counts as in the carousel (6000 ms).
	Delay, Every, Idle int
	// Lang is the language of the kit's words, one of i18n/<lang>.yaml
	// (default "en").
	Lang string
}

// LoadingCarousel renders the loading carousel: its <style>, its <section>,
// its Start button and its <script>, as {{ partial "loading-carousel.html" }}
// does. With no cards it is empty. The page then drives it through
// window.landingkitCarousel (see the README, "The loading carousel"), and
// gives it the variables styles.html defines: --line, --bg-soft, --ink,
// --slate and --accent.
func LoadingCarousel(c Carousel) (template.HTML, error) {
	lang := c.Lang
	if lang == "" {
		lang = "en"
	}
	words, err := kitWords(lang)
	if err != nil {
		return "", err
	}
	t, err := template.New("loading-carousel.html").Funcs(hugoFuncs(words)).Parse(loadingCarouselPartial)
	if err != nil {
		return "", err
	}
	cards := make([]map[string]string, len(c.Cards))
	for i, k := range c.Cards {
		cards[i] = map[string]string{"title": k.Title, "text": k.Text, "href": k.Href, "img": k.Img}
	}
	dict := map[string]any{"cards": cards}
	for k, v := range map[string]any{
		"id": c.ID, "pictures": c.Pictures, "ext": c.Ext, "suffix": c.Suffix,
		"label": c.Label, "start": c.Start, "fade": c.Fade, "more": c.More,
		"width": c.Width, "height": c.Height, "delay": c.Delay, "every": c.Every, "idle": c.Idle,
	} {
		if !reflect.ValueOf(v).IsZero() {
			dict[k] = v
		}
	}
	var b bytes.Buffer
	if err := t.Execute(&b, dict); err != nil {
		return "", err
	}
	return template.HTML(b.String()), nil
}

// hugoFuncs are the Hugo template functions the partial calls, with Hugo's
// meaning.
func hugoFuncs(words map[string]string) template.FuncMap {
	return template.FuncMap{
		"default":  hugoDefault,
		"add":      func(a, b int) int { return a + b },
		"safeHTML": func(s string) template.HTML { return template.HTML(s) },
		"safeJS":   func(s string) template.JS { return template.JS(s) },
		"dict": func(kv ...any) (map[string]any, error) {
			if len(kv)%2 != 0 {
				return nil, fmt.Errorf("dict: %d arguments, want pairs", len(kv))
			}
			m := map[string]any{}
			for i := 0; i < len(kv); i += 2 {
				k, ok := kv[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict: key %v is not a string", kv[i])
				}
				m[k] = kv[i+1]
			}
			return m, nil
		},
		"i18n": func(key string, data ...any) (string, error) {
			return translate(words, key, data...)
		},
	}
}

// hugoDefault is Hugo's default: given when it is set (a non-empty string,
// slice or map, a non-zero number, a true or false bool, a non-nil
// pointer), else dflt.
func hugoDefault(dflt any, given ...any) any {
	if len(given) == 0 || given[0] == nil {
		return dflt
	}
	v := reflect.ValueOf(given[0])
	switch v.Kind() {
	case reflect.Bool, reflect.Struct:
		return given[0]
	case reflect.String, reflect.Slice, reflect.Array, reflect.Map:
		if v.Len() == 0 {
			return dflt
		}
	case reflect.Pointer, reflect.Interface, reflect.Chan, reflect.Func:
		if v.IsNil() {
			return dflt
		}
	default: // the numbers
		if v.IsZero() {
			return dflt
		}
	}
	return given[0]
}

// translate is Hugo's i18n: the word, executed as a template on data when it
// has placeholders ("{{ .n }} of {{ .m }}").
func translate(words map[string]string, key string, data ...any) (string, error) {
	w, ok := words[key]
	if !ok {
		return "", fmt.Errorf("i18n: no %q", key)
	}
	if !strings.Contains(w, "{{") {
		return w, nil
	}
	t, err := ttemplate.New(key).Option("missingkey=error").Parse(w)
	if err != nil {
		return "", err
	}
	var arg any
	if len(data) > 0 {
		arg = data[0]
	}
	var b strings.Builder
	if err := t.Execute(&b, arg); err != nil {
		return "", err
	}
	return b.String(), nil
}

var wordLine = regexp.MustCompile(`^(kit_[A-Za-z]+): "(.*)"$`)

// kitWords reads i18n/<lang>.yaml.
func kitWords(lang string) (map[string]string, error) {
	if !regexp.MustCompile(`^[a-z]{2,3}$`).MatchString(lang) {
		return nil, fmt.Errorf("landingkit: %q is not a language", lang)
	}
	b, err := i18nFiles.ReadFile("i18n/" + lang + ".yaml")
	if err != nil {
		return nil, fmt.Errorf("landingkit: no words in %q: %w", lang, err)
	}
	return parseWords(lang, string(b))
}

// parseWords reads `kit_key: "value"` lines, which is all the kit's files
// hold, so that the module keeps no dependency.
func parseWords(lang, s string) (map[string]string, error) {
	out := map[string]string{}
	for i, line := range strings.Split(s, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := wordLine.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("landingkit: i18n/%s.yaml:%d is not a `kit_key: \"value\"` line", lang, i+1)
		}
		out[m[1]] = m[2]
	}
	return out, nil
}
