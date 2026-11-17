package ui_test

import (
	"testing"

	"aurink/internal/assets/fonts"
	"aurink/internal/views/kit"
	"aurink/internal/views/theme"
	"github.com/egoist/mygo/ui"
)

func TestFontsRegister(t *testing.T) {
	if err := fonts.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
}

func TestPalettesDiffer(t *testing.T) {
	light, dark := theme.For(theme.Options{}), theme.For(theme.Options{Dark: true})
	if light.Dark || !dark.Dark || light.Sheet == dark.Sheet || light.Text == dark.Text {
		t.Fatalf("light %+v dark %+v", light.Sheet, dark.Sheet)
	}
	if !theme.For(theme.Options{LowPower: true}).LowPower {
		t.Fatal("low power is lost")
	}
	if h := theme.For(theme.Options{Hue: 200}); h.Accent == light.Accent {
		t.Fatal("the hue does not move the accent")
	}
	for level := 1; level <= 6; level++ {
		if level < 3 && theme.HeadingFont(level).Family != theme.Serif {
			t.Fatalf("H%d is not serif", level)
		}
		if level > 1 && theme.HeadingFont(level).Size > theme.HeadingFont(level-1).Size {
			t.Fatalf("H%d is larger than H%d", level, level-1)
		}
	}
}

func TestKitButtonsAct(t *testing.T) {
	th := theme.For(theme.Options{})
	presses := 0
	tt := ui.NewTester(func(c *ui.Context) {
		b := kit.Button(c, th, "Open", "", true)
		if b.Clicked() {
			presses++
		}
	}, 400, 200)
	tt.Frame()
	if err := tt.Click("Open"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if presses != 1 {
		t.Fatalf("%d presses", presses)
	}
}

func TestSegmentedSelects(t *testing.T) {
	th := theme.For(theme.Options{})
	sel := 0
	tt := ui.NewTester(func(c *ui.Context) {
		if n := kit.Segmented(c, th, "views", []string{"Live", "Source"}, sel, true); n >= 0 {
			sel = n
		}
	}, 400, 200)
	tt.Frame()
	if err := tt.Click("Source"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if sel != 1 {
		t.Fatalf("selected %d", sel)
	}
}
