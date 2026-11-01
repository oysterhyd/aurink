package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func settle(tt *ui.Tester) {
	for i := 0; i < 2000 && tt.FramePending(); i++ {
		tt.Advance(16 * time.Millisecond)
	}
}

func TestIdleDrawsNoFrames(t *testing.T) {
	a := &look{}
	tt := ui.NewTester(a.view, 1280, 800)
	settle(tt)
	if tt.FramePending() {
		t.Fatal("the shell asks for frames while idle")
	}
}

func TestEffectsEnd(t *testing.T) {
	for name, label := range map[string]string{
		"pill":  "大纲",
		"panel": "搜索文件或输入命令",
		"find":  "查找",
		"theme": "主题",
		"low":   "低功耗",
	} {
		t.Run(name, func(t *testing.T) {
			a := &look{}
			tt := ui.NewTester(a.view, 1280, 800)
			settle(tt)
			if err := tt.Click(label); err != nil {
				t.Fatal(err)
			}
			tt.Advance(16 * time.Millisecond)
			settle(tt)
			if tt.FramePending() {
				t.Fatal("still asks for frames after the effect")
			}
		})
	}
}

func TestThemeDissolveLeavesOneShell(t *testing.T) {
	a := &look{}
	tt := ui.NewTester(a.view, 1280, 800)
	settle(tt)
	if err := tt.Click("主题"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(100 * time.Millisecond)
	if !a.dark {
		t.Fatal("theme did not change")
	}
	settle(tt)
	n := 0
	for _, s := range tt.Texts() {
		if s == "README.md" {
			n++
		}
	}
	if n != 2 { // the tab and the sidebar row
		t.Fatalf("README.md shows %d times after the dissolve, want 2", n)
	}
}

func TestAuroraStopsAfterTwelveSeconds(t *testing.T) {
	a := &look{welcome: true, testAurora: true}
	tt := ui.NewTester(a.view, 320, 200) // a small window: the CPU draws each frame
	frames := 0
	for tt.FramePending() && frames < 2000 {
		tt.Advance(16 * time.Millisecond)
		frames++
	}
	if tt.FramePending() {
		t.Fatal("the aurora never stops")
	}
	if frames < 12000/16-5 {
		t.Fatalf("the aurora stopped after %d frames", frames)
	}
}

// The Tester draws on the CPU, the low-power path: its fills are the solid
// colors of docs/02-product-ux.md §4.3.
func TestLowPowerColors(t *testing.T) {
	for _, tc := range []struct {
		dark                   bool
		wr, wg, wb, cr, cg, cb uint8
	}{
		{false, 0xE9, 0xEA, 0xEE, 0xFB, 0xFB, 0xFC},
		{true, 0x0B, 0x0C, 0x11, 0x1A, 0x1B, 0x23},
	} {
		a := &look{dark: tc.dark, low: true}
		tt := ui.NewTester(a.view, 1280, 800)
		settle(tt)
		img := tt.Image()
		c := img.RGBAAt(120, 700)
		if c.R != tc.wr || c.G != tc.wg || c.B != tc.wb {
			t.Errorf("dark=%v: window %v", tc.dark, c)
		}
		r, ok := tt.Find("已保存")
		if !ok {
			t.Fatal("no status chip")
		}
		c = img.RGBAAt(int(r.X)-6, int(r.Y)+int(r.H)/2)
		if c.R != tc.cr || c.G != tc.cg || c.B != tc.cb {
			t.Errorf("dark=%v: chip %v", tc.dark, c)
		}
		if dir := os.Getenv("S4_SHOTS"); dir != "" {
			f, err := os.Create(filepath.Join(dir, map[bool]string{false: "low-light.png", true: "low-dark.png"}[tc.dark]))
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(f, img); err != nil {
				t.Fatal(err)
			}
			f.Close()
		}
	}
}
