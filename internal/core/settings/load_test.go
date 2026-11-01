package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"aurink/internal/contract"
	"aurink/internal/core/fsstore"
)

func TestLoadedValuesAreNormalized(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		want        func(*contract.Settings)
	}{
		{"hue low", `{"Appearance":{"AccentHue":-1}}`, func(v *contract.Settings) { v.Appearance.AccentHue = 0 }},
		{"hue high", `{"Appearance":{"AccentHue":999}}`, func(v *contract.Settings) { v.Appearance.AccentHue = 359 }},
		{"font low", `{"Appearance":{"FontSize":0}}`, func(v *contract.Settings) { v.Appearance.FontSize = 8 }},
		{"font high", `{"Appearance":{"FontSize":1000}}`, func(v *contract.Settings) { v.Appearance.FontSize = 72 }},
		{"line low", `{"Appearance":{"LineHeight":0}}`, func(v *contract.Settings) { v.Appearance.LineHeight = 1 }},
		{"line high", `{"Appearance":{"LineHeight":50}}`, func(v *contract.Settings) { v.Appearance.LineHeight = 3 }},
		{"width low", `{"Appearance":{"ContentWidth":-1}}`, func(v *contract.Settings) { v.Appearance.ContentWidth = 320 }},
		{"width high", `{"Appearance":{"ContentWidth":10000}}`, func(v *contract.Settings) { v.Appearance.ContentWidth = 2000 }},
		{"tab low", `{"Editor":{"TabSize":0}}`, func(v *contract.Settings) { v.Editor.TabSize = 1 }},
		{"tab high", `{"Editor":{"TabSize":99}}`, func(v *contract.Settings) { v.Editor.TabSize = 8 }},
		{"delay low", `{"Files":{"AutosaveDelayMs":-1}}`, func(v *contract.Settings) { v.Files.AutosaveDelayMs = 100 }},
		{"delay high", `{"Files":{"AutosaveDelayMs":1000000}}`, func(v *contract.Settings) { v.Files.AutosaveDelayMs = 60000 }},
		{"large zero", `{"Files":{"LargeFileBytes":0}}`, func(*contract.Settings) {}},
		{"large negative", `{"Files":{"LargeFileBytes":-1}}`, func(*contract.Settings) {}},
		{"large positive", `{"Files":{"LargeFileBytes":1073741824}}`, func(v *contract.Settings) { v.Files.LargeFileBytes = 1 << 30 }},
		{"theme", `{"Appearance":{"Theme":"invalid"}}`, func(*contract.Settings) {}},
		{"motion", `{"Appearance":{"ReduceMotion":"invalid"}}`, func(*contract.Settings) {}},
		{"reveal", `{"Editor":{"Reveal":255}}`, func(*contract.Settings) {}},
		{"soft break", `{"Editor":{"SoftBreak":255}}`, func(*contract.Settings) {}},
		{"bullet", `{"Editor":{"BulletMarker":"x"}}`, func(*contract.Settings) {}},
		{"strong", `{"Editor":{"StrongDelim":"x"}}`, func(*contract.Settings) {}},
		{"emphasis", `{"Editor":{"EmphasisDelim":"x"}}`, func(*contract.Settings) {}},
		{"extensions", `{"Editor":{"Exts":4294967295}}`, func(v *contract.Settings) { v.Editor.Exts = 255 }},
		{"language", `{"Language":"invalid"}`, func(*contract.Settings) {}},
		{"image dir", `{"Files":{"ImageDir":""}}`, func(*contract.Settings) {}},
		{"valid enums", `{"Appearance":{"Theme":"light","ReduceMotion":"off"},"Editor":{"Reveal":3,"SoftBreak":1,"BulletMarker":"+","StrongDelim":"__","EmphasisDelim":"_"},"Language":"zh-Hans"}`, func(v *contract.Settings) {
			v.Appearance.Theme, v.Appearance.ReduceMotion = "light", "off"
			v.Editor.Reveal, v.Editor.SoftBreak = contract.RevealNone, contract.SoftBreakSpace
			v.Editor.BulletMarker, v.Editor.StrongDelim, v.Editor.EmphasisDelim = "+", "__", "_"
			v.Language = "zh-Hans"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			put(t, path, []byte(tc.input))
			s := open(t, path, fsstore.New(), nil, nil)
			want := Defaults()
			tc.want(&want)
			if got := s.Get(); !reflect.DeepEqual(got, want) {
				t.Fatalf("loaded %+v, want %+v", got, want)
			}
		})
	}
}

func TestMigrationStepsRunInOrderOnRawJSON(t *testing.T) {
	for _, from := range []int{0, 1, 2, 3} {
		var seen []int
		steps := []migration{}
		for step := 0; step < 3; step++ {
			steps = append(steps, func(raw map[string]json.RawMessage) error {
				seen = append(seen, step)
				if step == 0 {
					raw["Intermediate"] = raw["Legacy"]
					delete(raw, "Legacy")
				}
				if step == 1 {
					raw["Current"] = raw["Intermediate"]
					delete(raw, "Intermediate")
				}
				return nil
			})
		}
		raw := map[string]json.RawMessage{"version": json.RawMessage(`0`), "Legacy": json.RawMessage(`"value"`)}
		if from == 1 {
			raw["Intermediate"] = raw["Legacy"]
		}
		if err := migrate(raw, from, 3, steps); err != nil {
			t.Fatal(err)
		}
		want := []int{}
		for step := from; step < 3; step++ {
			want = append(want, step)
		}
		if len(seen) != len(want) || (len(want) > 0 && !reflect.DeepEqual(seen, want)) {
			t.Fatalf("from %d: steps %v, want %v", from, seen, want)
		}
		if string(raw["Version"]) != "3" || raw["version"] != nil {
			t.Fatalf("version not advanced: %v", raw)
		}
		if from <= 1 && string(raw["Current"]) != `"value"` {
			t.Fatalf("steps did not see previous raw edits: %v", raw)
		}
	}
}

func TestConstructorAppliesMigrationBeforeTypedDecode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	data := []byte(`{"Version":0,"LegacyTheme":"dark","Appearance":"old schema","Unknown":true}`)
	put(t, path, data)
	calls := 0
	steps := []migration{func(raw map[string]json.RawMessage) error {
		calls++
		if string(raw["Unknown"]) != "true" {
			t.Error("unknown raw fields removed before migration")
		}
		raw["Appearance"] = json.RawMessage(`{"Theme":` + string(raw["LegacyTheme"]) + `}`)
		delete(raw, "LegacyTheme")
		return nil
	}}
	s, err := newStore(path, fsstore.New(), &queued{}, nil, steps)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Flush() })
	if calls != 1 || s.Get().Version != contract.SettingsVersion || s.Get().Appearance.Theme != "dark" || s.Get().Appearance.FontSize != 16 {
		t.Fatalf("migrated: %+v, calls %d", s.Get(), calls)
	}
	flush(t, s)
	if string(contents(t, path)) != string(data) {
		t.Fatal("constructor rewrote a migration")
	}
	update(t, s, func(v *contract.Settings) { v.Language = "en" })
	flush(t, s)
	reopened, err := newStore(path, fsstore.New(), &queued{}, nil, steps)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || reopened.Get().Appearance.Theme != "dark" {
		t.Fatal("migration reran for current version")
	}
}

func TestMigrationFailuresPreserveOriginal(t *testing.T) {
	cause := errors.New("fake migration failure")
	for _, tc := range []struct {
		name  string
		steps []migration
	}{
		{"missing", nil},
		{"nil", []migration{nil}},
		{"failed", []migration{func(map[string]json.RawMessage) error { return cause }}},
		{"invalid raw result", []migration{func(raw map[string]json.RawMessage) error {
			raw["broken"] = json.RawMessage(`{`)
			return nil
		}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			put(t, path, []byte(`{"Version":0}`))
			s, err := newStore(path, fsstore.New(), &queued{}, nil, tc.steps)
			if s != nil || contract.CodeOf(err) != contract.CodeInvalid {
				t.Fatalf("migration error: %v %v", s, err)
			}
			if tc.name == "failed" && !errors.Is(err, cause) {
				t.Fatalf("lost migration cause: %v", err)
			}
			if string(contents(t, path)) != `{"Version":0}` {
				t.Fatal("migration error changed original")
			}
			if _, err := os.Stat(path + ".bad"); !os.IsNotExist(err) {
				t.Fatalf("migration error backed up valid JSON: %v", err)
			}
		})
	}
}
