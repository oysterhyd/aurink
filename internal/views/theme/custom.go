package theme

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

// CustomPrefix prefixes Settings.Appearance.Theme IDs for custom JSON themes.
const CustomPrefix = "custom:"

// CustomTheme is a validated user theme loaded from PathUserData/themes/*.json
// (09 D10, FE-2.8). Missing color tokens inherit from For(Options{Dark, Hue}).
type CustomTheme struct {
	ID        string
	File      string
	Name      string
	Dark      bool
	AccentHue float32
	Colors    map[string]ui.Color
}

type rawCustomTheme struct {
	Name      string            `json:"name"`
	Dark      bool              `json:"dark"`
	AccentHue *float32          `json:"accentHue"`
	Colors    map[string]string `json:"colors"`
}

var knownColorKeys = map[string]bool{
	"window": true, "sheet": true, "text": true, "muted": true, "faint": true,
	"surface": true, "surfaceHover": true, "hover": true, "border": true,
	"accent": true, "accentSoft": true, "accentRing": true,
	"selection": true, "onAccent": true, "mark": true, "codeBg": true,
	"warning": true, "success": true, "danger": true,
	"scrim": true, "scrimStrong": true,
	"synKeyword": true, "synFunc": true, "synComment": true, "synString": true,
	"aurora1": true, "aurora2": true, "aurora3": true,
	"chip": true, "panel": true, "modal": true, "solid": true,
	"inkButton": true, "onInkButton": true, "shadowColor": true,
}

// ParseHexColor parses strict "#RRGGBB" or "#RRGGBBAA" color strings.
func ParseHexColor(s string) (ui.Color, bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "#") {
		return ui.Color{}, false
	}
	hex := s[1:]
	if len(hex) != 6 && len(hex) != 8 {
		return ui.Color{}, false
	}
	r, err1 := strconv.ParseUint(hex[0:2], 16, 8)
	g, err2 := strconv.ParseUint(hex[2:4], 16, 8)
	b, err3 := strconv.ParseUint(hex[4:6], 16, 8)
	if err1 != nil || err2 != nil || err3 != nil {
		return ui.Color{}, false
	}
	if len(hex) == 8 {
		a, err4 := strconv.ParseUint(hex[6:8], 16, 8)
		if err4 != nil {
			return ui.Color{}, false
		}
		return ui.Color{R: uint8(r), G: uint8(g), B: uint8(b), A: uint8(a)}, true
	}
	return ui.RGB(uint8(r), uint8(g), uint8(b)), true
}

// ParseCustom parses and validates a single custom theme JSON payload.
func ParseCustom(stem string, data []byte) (CustomTheme, error) {
	stem = strings.TrimSpace(stem)
	if stem == "" {
		return CustomTheme{}, fmt.Errorf("empty theme file name")
	}
	var raw rawCustomTheme
	dec := json.NewDecoder(strings.NewReader(string(data)))
	if err := dec.Decode(&raw); err != nil {
		return CustomTheme{}, err
	}
	hue := float32(DefaultHue)
	if raw.AccentHue != nil {
		if *raw.AccentHue < 0 || *raw.AccentHue >= 360 {
			return CustomTheme{}, fmt.Errorf("accentHue %v out of [0, 360)", *raw.AccentHue)
		}
		hue = *raw.AccentHue
	}
	name := strings.TrimSpace(raw.Name)
	if name == "" {
		name = stem
	}
	colors := make(map[string]ui.Color, len(raw.Colors))
	for k, v := range raw.Colors {
		if !knownColorKeys[k] {
			return CustomTheme{}, fmt.Errorf("unknown color token %q", k)
		}
		c, ok := ParseHexColor(v)
		if !ok {
			return CustomTheme{}, fmt.Errorf("invalid hex color %q for %q", v, k)
		}
		colors[k] = c
	}
	return CustomTheme{
		ID:        CustomPrefix + strings.ToLower(stem),
		File:      stem + ".json",
		Name:      name,
		Dark:      raw.Dark,
		AccentHue: hue,
		Colors:    colors,
	}, nil
}

// LoadCustomDir loads all *.json theme files from dir, returning valid themes
// sorted by filename alongside errors for any corrupt or invalid files. A
// non-existent directory returns (nil, nil).
func LoadCustomDir(dir string) ([]CustomTheme, []error) {
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, []error{err}
	}
	slices.SortFunc(entries, func(a, b os.DirEntry) int {
		return strings.Compare(strings.ToLower(a.Name()), strings.ToLower(b.Name()))
	})
	var themes []CustomTheme
	var errs []error
	seenID := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			continue
		}
		stem := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", e.Name(), err))
			continue
		}
		ct, err := ParseCustom(stem, data)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", e.Name(), err))
			continue
		}
		ct.File = e.Name()
		if seenID[ct.ID] {
			continue
		}
		seenID[ct.ID] = true
		themes = append(themes, ct)
	}
	return themes, errs
}

// Tokens builds a complete Tokens palette starting from For(o) (with o.Dark and
// o.Hue defaulted to ct.Dark and ct.AccentHue) and applying ct.Colors overrides.
func (ct *CustomTheme) Tokens(o Options) *Tokens {
	o.Dark = ct.Dark
	if o.Hue == 0 {
		o.Hue = ct.AccentHue
	}
	t := For(o)
	if acc, ok := ct.Colors["accent"]; ok {
		t.Accent = acc
		if ct.Dark {
			t.AccentSoft = acc.Alpha(.17)
			t.AccentRing = acc.Alpha(.25)
			t.Selection = acc.Alpha(.30)
		} else {
			t.AccentSoft = acc.Alpha(.11)
			t.AccentRing = acc.Alpha(.20)
			t.Selection = acc.Alpha(.20)
		}
		t.SynFunc = acc
	}
	set := func(key string, dst *ui.Color) {
		if c, ok := ct.Colors[key]; ok {
			*dst = c
		}
	}
	set("window", &t.Window)
	set("sheet", &t.Sheet)
	set("text", &t.Text)
	set("muted", &t.Muted)
	set("faint", &t.Faint)
	set("surface", &t.Surface)
	set("surfaceHover", &t.SurfaceHover)
	set("hover", &t.Hover)
	set("border", &t.Border)
	set("accentSoft", &t.AccentSoft)
	set("accentRing", &t.AccentRing)
	set("selection", &t.Selection)
	set("onAccent", &t.OnAccent)
	set("mark", &t.Mark)
	set("codeBg", &t.CodeBg)
	set("warning", &t.Warning)
	set("success", &t.Success)
	set("danger", &t.Danger)
	set("scrim", &t.Scrim)
	set("scrimStrong", &t.ScrimStrong)
	set("synKeyword", &t.SynKeyword)
	set("synFunc", &t.SynFunc)
	set("synComment", &t.SynComment)
	set("synString", &t.SynString)
	set("aurora1", &t.Aurora1)
	set("aurora2", &t.Aurora2)
	set("aurora3", &t.Aurora3)
	set("chip", &t.Chip)
	set("panel", &t.Panel)
	set("modal", &t.Modal)
	set("solid", &t.Solid)
	set("inkButton", &t.InkButton)
	set("onInkButton", &t.OnInkButton)
	set("shadowColor", &t.ShadowColor)
	return t
}
