package main

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"aurink/internal/views/effects"
	"aurink/internal/views/kit"
	"aurink/internal/views/theme"
)

// designSystem saves the tokens, type, glass and controls that exist in M1, in the
// order of the DesignSystem baseline.
func designSystem(out string) {
	light, dark := theme.For(theme.Options{}), theme.For(theme.Options{Dark: true})
	seg := 0
	w := mygo.NewWindow(mygo.WindowOptions{Title: "Aurink", Width: 1500, Height: 1000, Content: ui.View(func(c *ui.Context) {
		ui.Column(c).Fill().Padding(24, 24).Gap(16).Font(theme.Sans).Background(light.Window).Children(func() {
			ui.Row(c).Gap(16).AlignItems(ui.Stretch).Children(func() {
				palette(c, light, "Paper")
				palette(c, dark, "Midnight Ink")
			})
			ui.Row(c).Gap(16).Grow(1).AlignItems(ui.Stretch).Children(func() {
				glass(c, light)
				ui.Column(c).Grow(1).Basis(0).Padding(20, 24).Gap(12).Radius(theme.RadiusSheet).Background(light.Sheet).TextColor(light.Text).Children(func() {
					sample(c, "H1 文档标题 Editorial", theme.Serif, theme.Body*2.6, 400)
					sample(c, "H2 章节标题，衬线承担气质", theme.Serif, theme.Body*1.62, 400)
					sample(c, "H3 小节标题回到无衬线", theme.Sans, theme.Body*1.12, 600)
					sample(c, "正文 16 / 1.75，CJK 优先，无衬线保证长时间可读。", theme.Sans, theme.Body, 400)
					sample(c, "JetBrains Mono · **标记** 与代码", theme.Mono, theme.Body*.85, 400)
					ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
						kit.Button(c, light, "主操作", "", true)
						kit.Button(c, light, "次操作", "", false)
						kit.KeyHint(c, light, "Ctrl P")
					})
					if i := kit.Segmented(c, light, &seg, []string{"文件", "大纲", "搜索"}, seg, true); i >= 0 {
						seg = i
					}
					kit.Capsule(c, light).Children(func() {
						ui.Text(c, "胶囊").FontSize(theme.UIText).TextColor(light.Text)
					})
				})
			})
		})
	})})
	go func() {
		time.Sleep(1500 * time.Millisecond)
		data, err := w.CapturePage()
		if err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, "DesignSystem.png"), data, 0o644); err != nil {
			log.Fatal(err)
		}
		w.Update(func() { w.Destroy(); mygo.App.Quit() })
	}()
}

func sample(c *ui.Context, text, family string, size float32, weight int) {
	ui.Text(c, text).Font(family).FontSize(size).FontWeight(weight)
}

func palette(c *ui.Context, t *theme.Tokens, name string) {
	colors := []struct {
		name string
		v    ui.Color
	}{{"Window", t.Window}, {"Sheet", t.Sheet}, {"Ink", t.Text}, {"Muted", t.Muted}, {"Faint", t.Faint}, {"Accent", t.Accent}, {"Mark", t.Mark}}
	ui.Column(c).Grow(1).Basis(0).Padding(20, 24).Gap(12).Radius(theme.RadiusSheet).Background(t.Sheet).TextColor(t.Text).Children(func() {
		ui.Text(c, name).Font(theme.Serif).FontSize(22)
		ui.Row(c).Gap(8).Children(func() {
			for _, k := range colors {
				ui.Column(c).Gap(4).Grow(1).Basis(0).Children(func() {
					ui.Column(c).Height(64).Radius(8).Background(k.v).Border(1, t.Border)
					ui.Text(c, k.name).FontSize(theme.UIBadge)
				})
			}
		})
	})
}

func glass(c *ui.Context, t *theme.Tokens) {
	levels := []struct {
		name  string
		level effects.Level
	}{{"1 · 胶囊 16", effects.Chip}, {"2 · 浮层 24", effects.Panel}, {"3 · 模态 36", effects.Modal}}
	ui.Row(c).Grow(1).Basis(0).Padding(24, 24).Gap(14).Radius(theme.RadiusSheet).Draw(func(p *ui.Painter, r ui.Rect) {
		effects.PaintWindow(p, r, t)
	}).Children(func() {
		for _, l := range levels {
			ui.Column(c).Grow(1).Basis(0).Height(180).Padding(14, 16).Radius(theme.RadiusSheet).Border(1, t.Border).
				Material(effects.Glass(t, l.level)).TextColor(t.Text).Children(func() {
				ui.Text(c, l.name).FontSize(theme.UIText).FontWeight(600)
			})
		}
	})
}
