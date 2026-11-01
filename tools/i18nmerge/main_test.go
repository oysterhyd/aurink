package main

import (
	"strings"
	"testing"
)

func mustParse(t *testing.T, name, data string) *fragment {
	t.Helper()
	owner, lang, ok := parseName(name)
	if !ok {
		t.Fatalf("parseName(%q) failed", name)
	}
	f, err := parseFragment(name, owner, lang, []byte(data))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// pair builds an owner's two fragments holding one key with the same message.
func pair(t *testing.T, owner, key, msg string) []*fragment {
	t.Helper()
	en := `{"` + key + `": "` + msg + `", "@` + key + `": {"description": "Where it shows."}}`
	zh := `{"` + key + `": "` + msg + `"}`
	return []*fragment{mustParse(t, owner+".en.json", en), mustParse(t, owner+".zh-Hans.json", zh)}
}

func TestValidateAccepts(t *testing.T) {
	for _, tt := range []struct{ owner, key, msg string }{
		{"int", "app_name", "Aurink"},
		{"fe-2", "shell_tabs_close", "Close {fileName}"},
		{"fe-1", "liveedit_heading1", "Heading 1"},
		{"int", "mygo.appName", "Aurink"},
		{"be-2", "search_results", "{count, plural, one {# result} other {# results}}"},
	} {
		if errs := validate(pair(t, tt.owner, tt.key, tt.msg)); len(errs) > 0 {
			t.Errorf("%s %s: %v", tt.owner, tt.key, errs)
		}
	}
}

func TestValidateRejects(t *testing.T) {
	for _, tt := range []struct{ rule, owner, key, msg string }{
		{"K1", "int", "appName", "x"},
		{"K1", "int", "app", "x"},
		{"K1", "int", "app__name", "x"},
		{"K1", "int", "app_name_", "x"},
		{"K1", "fe-1", "liveedit_heading_1", "x"},
		{"K2", "fe-1", "shell_tabs_close", "x"},
		{"K4", "fe-2", "mygo.appName", "x"},
		{"K5", "int", "app_name", "{file_name}"},
		{"K5", "int", "app_name", "{Name}"},
		{"K5", "int", "app_name", "{func}"},
	} {
		errs := validate(pair(t, tt.owner, tt.key, tt.msg))
		if !hasRule(errs, tt.rule) {
			t.Errorf("%s %q %q: want %s, got %v", tt.owner, tt.key, tt.msg, tt.rule, errs)
		}
	}
}

func TestValidateRejectsPairs(t *testing.T) {
	const enOK = `{"app_name": "x", "@app_name": {"description": "d"}}`
	for name, files := range map[string][]*fragment{
		"missing language": {mustParse(t, "int.en.json", enOK)},
		"missing key": {
			mustParse(t, "int.en.json", enOK),
			mustParse(t, "int.zh-Hans.json", `{}`),
		},
		"extra key": {
			mustParse(t, "int.en.json", enOK),
			mustParse(t, "int.zh-Hans.json", `{"app_name": "x", "app_other": "y"}`),
		},
		"different placeholders": {
			mustParse(t, "int.en.json", enOK),
			mustParse(t, "int.zh-Hans.json", `{"app_name": "{name}"}`),
		},
		"no description": {
			mustParse(t, "int.en.json", `{"app_name": "x"}`),
			mustParse(t, "int.zh-Hans.json", `{"app_name": "x"}`),
		},
		"description in a translation": {
			mustParse(t, "int.en.json", enOK),
			mustParse(t, "int.zh-Hans.json", `{"app_name": "x", "@app_name": {"description": "d"}}`),
		},
	} {
		if errs := validate(files); !hasRule(errs, "K6") {
			t.Errorf("%s: want K6, got %v", name, errs)
		}
	}
}

func TestValidateRejectsKeyInTwoFragments(t *testing.T) {
	files := append(pair(t, "int", "app_name", "x"), pair(t, "fe-2", "app_name", "x")...)
	found := false
	for _, err := range validate(files) {
		found = found || strings.Contains(err.Error(), "is also in int.en.json")
	}
	if !found {
		t.Errorf("no duplicate reported: %v", validate(files))
	}
}

func TestParseRejects(t *testing.T) {
	for name, tt := range map[string]struct{ file, data string }{
		"key written twice": {"int.en.json", `{"app_name": "a", "app_name": "b"}`},
		"wrong locale":      {"int.en.json", `{"@@locale": "de"}`},
		"not a string":      {"int.en.json", `{"app_name": 1}`},
		"not an object":     {"int.en.json", `[]`},
	} {
		owner, lang, _ := parseName(tt.file)
		if _, err := parseFragment(tt.file, owner, lang, []byte(tt.data)); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	for _, name := range []string{"qa.en.json", "int.de.json", "int.json", "int.en.txt", ".gitkeep"} {
		if _, _, ok := parseName(name); ok {
			t.Errorf("parseName(%q) accepted", name)
		}
	}
}

func TestMergeSortsKeysAndKeepsMeta(t *testing.T) {
	files := append(pair(t, "int", "app_name", "A<b>"), pair(t, "be-2", "search_empty", "S")...)
	files = append(files, pair(t, "fe-2", "kit_close", "K")...)
	got, err := merge(files)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "@@locale": "en",
  "app_name": "A<b>",
  "@app_name": {
    "description": "Where it shows."
  },
  "kit_close": "K",
  "@kit_close": {
    "description": "Where it shows."
  },
  "search_empty": "S",
  "@search_empty": {
    "description": "Where it shows."
  }
}
`
	if s := string(got["en.json"]); s != want {
		t.Errorf("en.json:\n%s\nwant:\n%s", s, want)
	}
	wantZh := `{
  "@@locale": "zh-Hans",
  "app_name": "A<b>",
  "kit_close": "K",
  "search_empty": "S"
}
`
	if s := string(got["zh-Hans.json"]); s != wantZh {
		t.Errorf("zh-Hans.json:\n%s\nwant:\n%s", s, wantZh)
	}
}

func hasRule(errs []error, rule string) bool {
	for _, err := range errs {
		if strings.Contains(err.Error(), rule+":") {
			return true
		}
	}
	return false
}
