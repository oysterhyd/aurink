package main

import (
	"strings"
	"testing"
)

func mustParse(t *testing.T, name, data string) *catalog {
	t.Helper()
	c, err := parseCatalog(name, []byte(data))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// pair builds both catalogs holding one key with the same message.
func pair(t *testing.T, key, msg string) (en, zh *catalog) {
	t.Helper()
	en = mustParse(t, "en.json", `{"`+key+`": "`+msg+`", "@`+key+`": {"description": "Where it shows."}}`)
	zh = mustParse(t, "zh-Hans.json", `{"`+key+`": "`+msg+`"}`)
	return en, zh
}

func TestValidateAccepts(t *testing.T) {
	for _, tt := range []struct{ key, msg string }{
		{"app_name", "Aurink"},
		{"shell_tabs_close", "Close {fileName}"},
		{"liveedit_heading1", "Heading 1"},
		{"mygo.appName", "Aurink"},
		{"search_results", "{count, plural, one {# result} other {# results}}"},
	} {
		en, zh := pair(t, tt.key, tt.msg)
		if errs := validate(en, zh); len(errs) > 0 {
			t.Errorf("%s: %v", tt.key, errs)
		}
	}
}

func TestValidateRejects(t *testing.T) {
	for _, tt := range []struct{ rule, key, msg string }{
		{"K1", "appName", "x"},
		{"K1", "app", "x"},
		{"K1", "app__name", "x"},
		{"K1", "app_name_", "x"},
		{"K1", "liveedit_heading_1", "x"},
		{"K2", "nopackage_tabs_close", "x"},
		{"K5", "app_name", "{file_name}"},
		{"K5", "app_name", "{Name}"},
		{"K5", "app_name", "{func}"},
	} {
		en, zh := pair(t, tt.key, tt.msg)
		if errs := validate(en, zh); !hasRule(errs, tt.rule) {
			t.Errorf("%q %q: want %s, got %v", tt.key, tt.msg, tt.rule, errs)
		}
	}
}

func TestValidateRejectsMismatch(t *testing.T) {
	en := mustParse(t, "en.json", `{"app_name": "x", "@app_name": {"description": "d"}}`)
	for name, zh := range map[string]string{
		"missing key":            `{}`,
		"extra key":              `{"app_name": "x", "app_other": "y"}`,
		"different placeholders": `{"app_name": "{name}"}`,
	} {
		if errs := validate(en, mustParse(t, "zh-Hans.json", zh)); !hasRule(errs, "K3") {
			t.Errorf("%s: want K3, got %v", name, errs)
		}
	}
	noDesc := mustParse(t, "en.json", `{"app_name": "x"}`)
	if errs := validate(noDesc, mustParse(t, "zh-Hans.json", `{"app_name": "x"}`)); !hasRule(errs, "K3") {
		t.Errorf("no description: want K3, got %v", errs)
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
