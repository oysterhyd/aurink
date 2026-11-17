package theme

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestParseHexColor(t *testing.T) {
	if c, ok := ParseHexColor("#FDF6E3"); !ok || c != ui.RGB(0xFD, 0xF6, 0xE3) {
		t.Fatalf("ParseHexColor 6-digit = %+v, %v", c, ok)
	}
	if c, ok := ParseHexColor("#11223380"); !ok || c.R != 0x11 || c.G != 0x22 || c.B != 0x33 || c.A != 0x80 {
		t.Fatalf("ParseHexColor 8-digit = %+v, %v", c, ok)
	}
	for _, bad := range []string{"", "FDF6E3", "#FFF", "#12345", "#1234567", "#GGGGGG", "rgb(1,2,3)"} {
		if _, ok := ParseHexColor(bad); ok {
			t.Errorf("ParseHexColor(%q) succeeded, want false", bad)
		}
	}
}

func TestLoadCustomDirAndTokensInheritance(t *testing.T) {
	if themes, errs := LoadCustomDir(filepath.Join(t.TempDir(), "nonexistent")); len(themes) != 0 || len(errs) != 0 {
		t.Fatalf("missing dir returned (%v, %v), want empty", themes, errs)
	}

	dir := t.TempDir()
	solarized := `{
		"name": "Solarized Paper",
		"dark": false,
		"accentHue": 175,
		"colors": {
			"window": "#EEE8D5",
			"sheet": "#FDF6E3",
			"text": "#586E75",
			"accent": "#2AA198"
		}
	}`
	if err := os.WriteFile(filepath.Join(dir, "solarized.json"), []byte(solarized), 0o644); err != nil {
		t.Fatal(err)
	}
	// Invalid color token and corrupt JSON must be reported without blocking valid themes.
	if err := os.WriteFile(filepath.Join(dir, "bad_token.json"), []byte(`{"name":"Bad","colors":{"unknownKey":"#000000"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "corrupt.json"), []byte(`{not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}

	themes, errs := LoadCustomDir(dir)
	if len(themes) != 1 || len(errs) != 2 {
		t.Fatalf("LoadCustomDir: %d themes, %d errs; want 1 theme, 2 errs (%v)", len(themes), len(errs), errs)
	}
	ct := themes[0]
	if ct.ID != "custom:solarized" || ct.Name != "Solarized Paper" || ct.Dark {
		t.Fatalf("unexpected CustomTheme: %+v", ct)
	}
	tok := ct.Tokens(Options{})
	base := For(Options{Dark: false, Hue: 175})
	if tok.Sheet != ui.RGB(0xFD, 0xF6, 0xE3) || tok.Window != ui.RGB(0xEE, 0xE8, 0xD5) || tok.Accent != ui.RGB(0x2A, 0xA1, 0x98) {
		t.Fatalf("custom overrides did not apply: %+v", tok)
	}
	// Unspecified fields must inherit from the base palette.
	if tok.Danger != base.Danger || tok.CodeBg != base.CodeBg {
		t.Fatalf("unspecified tokens did not inherit base defaults: danger=%+v codeBg=%+v", tok.Danger, tok.CodeBg)
	}
}
