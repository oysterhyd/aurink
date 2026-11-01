package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func registerBundled(t *testing.T, file, name string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("fonts", file))
	if err != nil {
		t.Fatal(err)
	}
	if err := ui.RegisterFont(data, name); err != nil {
		t.Fatal(err)
	}
}

func draw(text, family string, weight int, italic bool) []byte {
	tt := ui.NewTester(func(c *ui.Context) {
		e := ui.Text(c, text).Font(family).FontSize(48).FontWeight(weight)
		if italic {
			e.Italic()
		}
	}, 500, 120)
	return tt.Image().Pix
}

// Each bundled family really draws, each face differs, and the subset's weight
// axis works. A family that is not found falls back, and would draw alike.
func TestBundledFonts(t *testing.T) {
	registerBundled(t, "Noto_Serif_SC-gb2312-500-700.ttf", "Noto Serif SC")
	registerBundled(t, "Newsreader-opsz-wght.ttf", "Newsreader")
	registerBundled(t, "Newsreader-Italic-opsz-wght.ttf", "Newsreader")
	registerBundled(t, "JetBrains_Mono-wght.ttf", "JetBrains Mono")
	for _, tc := range []struct {
		name string
		a, b []byte
	}{
		{"noto is found", draw("是什么", "Noto Serif SC", 500, false), draw("是什么", "Aurink No Such Font", 500, false)},
		{"noto weight", draw("是什么", "Noto Serif SC", 500, false), draw("是什么", "Noto Serif SC", 700, false)},
		{"newsreader is found", draw("Aurink", "Newsreader", 500, false), draw("Aurink", "Aurink No Such Font", 500, false)},
		{"newsreader italic", draw("Aurink", "Newsreader", 400, false), draw("Aurink", "Newsreader", 400, true)},
		{"jetbrains is found", draw("func(x)", "JetBrains Mono", 400, false), draw("func(x)", "Aurink No Such Font", 400, false)},
		{"mixed run takes both", draw("Aurink 是", "Newsreader, Noto Serif SC", 500, false), draw("Aurink 是", "Newsreader", 500, false)},
	} {
		if bytes.Equal(tc.a, tc.b) {
			t.Errorf("%s: the two draw the same", tc.name)
		}
	}
}
