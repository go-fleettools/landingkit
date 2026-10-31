package landingkit

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// kitLanguages are the languages the kit ships strings for besides English.
var kitLanguages = []string{"fr", "de", "es", "it", "pl", "pt", "ru", "uk", "zh", "ko", "ja"}

var yamlLine = regexp.MustCompile(`^(kit_[A-Za-z]+): "(.*)"$`)

// strings reads i18n/<lang>.yaml: key: "value" lines, which is all the kit's
// files hold, so that the module keeps no dependency.
func kitStrings(t *testing.T, lang string) map[string]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("i18n", lang+".yaml"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for i, line := range strings.Split(string(b), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := yamlLine.FindStringSubmatch(line)
		if m == nil {
			t.Fatalf("i18n/%s.yaml:%d is not a `kit_key: \"value\"` line: %q", lang, i+1, line)
		}
		if _, dup := out[m[1]]; dup {
			t.Fatalf("i18n/%s.yaml: %s twice", lang, m[1])
		}
		out[m[1]] = m[2]
	}
	return out
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// legacy are the words the partials wrote before they were translatable. A
// site in English must get exactly them: TestEnglishIsTheOldText.
var legacy = map[string]string{
	"kit_forkMe":        "Fork me on GitHub",
	"kit_themeToggle":   "Toggle light, dark or system theme",
	"kit_themeSystem":   "System",
	"kit_themeLight":    "Light",
	"kit_themeDark":     "Dark",
	"kit_mantra":        "Licence and build of this organisation's code",
	"kit_stopRotation":  "Stop slide rotation",
	"kit_startRotation": "Start slide rotation",
	"kit_previousSlide": "Previous slide",
	"kit_nextSlide":     "Next slide",
	"kit_chooseSlide":   "Choose slide to display",
	"kit_slideN":        "Slide {{ .n }}",
	"kit_nOfM":          "{{ .n }} of {{ .m }}",
	"kit_readMore":      "Read more",
	"kit_about":         "about {{ .title }}",
	"kit_heroLabel":     "{{ .title }} in pictures",
	"kit_carouselLabel": "What this project does",
	"kit_start":         "Start",
}

func TestEnglishIsTheOldText(t *testing.T) {
	en := kitStrings(t, "en")
	for k, want := range legacy {
		if en[k] != want {
			t.Errorf("%s = %q, the partials used to write %q", k, en[k], want)
		}
	}
	// the only English key that is new is the selector's
	for _, k := range keys(en) {
		if _, old := legacy[k]; !old && k != "kit_language" {
			t.Errorf("%s is new: say so here, and check the English pages stay as they were", k)
		}
	}
}

func TestEveryLanguageHasEveryKey(t *testing.T) {
	en := kitStrings(t, "en")
	for _, lang := range kitLanguages {
		got := kitStrings(t, lang)
		if a, b := strings.Join(keys(got), " "), strings.Join(keys(en), " "); a != b {
			t.Errorf("i18n/%s.yaml has keys\n  %s\nthe English file has\n  %s", lang, a, b)
		}
		for k, v := range got {
			if v == "" {
				t.Errorf("i18n/%s.yaml: %s is empty", lang, k)
			}
			// a value lands in attributes and script strings, untouched
			if strings.ContainsAny(v, "\"<>\\`") {
				t.Errorf("i18n/%s.yaml: %s has a character that does not belong in an attribute or a script string: %q", lang, k, v)
			}
			// the same placeholders as the English string
			for _, ph := range []string{"{{ .n }}", "{{ .m }}", "{{ .title }}"} {
				if strings.Contains(en[k], ph) != strings.Contains(v, ph) {
					t.Errorf("i18n/%s.yaml: %s must use %s as the English one does", lang, k, ph)
				}
			}
		}
	}
}

var usedKey = regexp.MustCompile(`i18n "(kit_[A-Za-z]+)"`)

func TestPartialsUseOnlyKnownKeysAndAllOfThem(t *testing.T) {
	en := kitStrings(t, "en")
	used := map[string]bool{}
	files, err := filepath.Glob("layouts/partials/*.html")
	if err != nil || len(files) == 0 {
		t.Fatal("no partials", err)
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range usedKey.FindAllStringSubmatch(string(b), -1) {
			used[m[1]] = true
			if _, ok := en[m[1]]; !ok {
				t.Errorf("%s uses %s, which i18n/en.yaml does not define", f, m[1])
			}
		}
	}
	for k := range en {
		if !used[k] {
			t.Errorf("i18n/en.yaml defines %s, which no partial uses", k)
		}
	}
}

// The selector and the head partial print nothing for a site with one
// language: that is what keeps the other landings as they were.
func TestMultilingualPartialsAreGuarded(t *testing.T) {
	for _, f := range []string{"language-selector.html", "i18n-head.html"} {
		b, err := os.ReadFile(filepath.Join("layouts", "partials", f))
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		i := strings.Index(s, "{{- if hugo.IsMultilingual -}}")
		if i < 0 || !strings.HasSuffix(s, "{{- end -}}") {
			t.Errorf("%s is not wrapped in {{- if hugo.IsMultilingual -}} ... {{- end -}}", f)
		}
	}
}
