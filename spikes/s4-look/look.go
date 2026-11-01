package main

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"
)

const (
	serif = "Newsreader, Noto Serif SC, Georgia, SimSun"
	sans  = "Segoe UI Variable, Microsoft YaHei UI"
	hue   = 182
)

// palette holds the tokens of docs/02-product-ux.md §4.1 that the shell uses.
type palette struct {
	window, sheet, text, muted, faint    ui.Color
	surface, surfaceHover, hover, border ui.Color
	accent, mark                         ui.Color
	chip, panel, modal, solid            ui.Color
	aur1, aur2, aur3                     ui.Color
	tintStrength                         float32
}

func paletteFor(dark, low bool) palette {
	if dark {
		p := palette{
			window: ui.RGB(0x09, 0x0A, 0x0F), sheet: ui.RGB(0x11, 0x12, 0x18), text: ui.RGB(0xEC, 0xED, 0xF2),
			muted: ui.RGB(0xA2, 0xA4, 0xB3), faint: ui.RGB(0x6C, 0x6E, 0x7E),
			surface: ui.RGBA(255, 255, 255, .045), surfaceHover: ui.RGBA(255, 255, 255, .10),
			hover: ui.RGBA(255, 255, 255, .055), border: ui.RGBA(255, 255, 255, .085),
			accent: ui.Oklch(.82, .12, hue), mark: ui.RGBA(255, 196, 84, .26),
			chip: ui.RGBA(30, 31, 40, .62), panel: ui.RGBA(26, 27, 36, .68), modal: ui.RGBA(22, 23, 31, .78),
			solid: ui.RGB(0x1A, 0x1B, 0x23),
			aur1:  ui.Oklch(.80, .13, hue), aur2: ui.Oklch(.70, .15, hue+92), aur3: ui.Oklch(.74, .15, hue+165),
			tintStrength: .10,
		}
		if low {
			p.window = ui.RGB(0x0B, 0x0C, 0x11)
		}
		return p
	}
	p := palette{
		window: ui.RGB(0xE8, 0xE9, 0xEE), sheet: ui.RGB(255, 255, 255), text: ui.RGB(0x15, 0x16, 0x1C),
		muted: ui.RGB(0x5F, 0x61, 0x6E), faint: ui.RGB(0x8B, 0x8D, 0x99),
		surface: ui.RGBA(20, 22, 35, .04), surfaceHover: ui.RGBA(20, 22, 35, .09),
		hover: ui.RGBA(20, 22, 35, .05), border: ui.RGBA(20, 22, 35, .09),
		accent: ui.Oklch(.50, .11, hue), mark: ui.RGB(0xFF, 0xEF, 0xB0),
		chip: ui.RGBA(255, 255, 255, .68), panel: ui.RGBA(255, 255, 255, .72), modal: ui.RGBA(252, 252, 254, .82),
		solid: ui.RGB(0xFB, 0xFB, 0xFC),
		aur1:  ui.Oklch(.66, .14, hue), aur2: ui.Oklch(.58, .17, hue+92), aur3: ui.Oklch(.66, .17, hue+165),
		tintStrength: .13,
	}
	if low {
		p.window = ui.RGB(0xE9, 0xEA, 0xEE)
	}
	return p
}

// frost is a glass level of §4.3: the backdrop blurred and saturated, under a
// fill, with a highlight along the top.
var frost = shader.MustCompile(`package main

var Tint vec4
var Sat float

func Fragment(dst vec4, pos vec2) vec4 {
	b := backdropSample(pos)
	g := dot(b.rgb, vec3(0.2126, 0.7152, 0.0722))
	c := mix(vec3(g), b.rgb, Sat)
	c = mix(c, Tint.rgb, Tint.a)
	if pos.y < 1 {
		c = mix(c, vec3(1), 0.3)
	}
	return vec4(c, 1)
}
`)

// tint is the window's ground: three soft pools of the aurora colors.
var tint = shader.MustCompile(`package main

var Base, A, B, C vec4
var Strength float

func pool(pos, at, r vec2) float {
	return 1 - smoothstep(0, 1, length((pos-at)/r))
}

func Fragment(dst vec4, pos vec2) vec4 {
	s := rectSize()
	c := Base.rgb
	c = mix(c, A.rgb, Strength*pool(pos, vec2(0, 0), s*vec2(.9, .7)))
	c = mix(c, B.rgb, Strength*.8*pool(pos, vec2(s.x, 0), s*vec2(.7, .6)))
	c = mix(c, C.rgb, Strength*.6*pool(pos, s, s*vec2(.8, .7)))
	return vec4(c, 1)
}
`)

// aurora is the low-frequency drift of the welcome page.
var aurora = shader.MustCompile(`package main

var Sheet, A, B, C vec4
var Time, Strength float

func pool(pos, at, r vec2) float {
	return 1 - smoothstep(0, 1, length((pos-at)/r))
}

func Fragment(dst vec4, pos vec2) vec4 {
	s := rectSize()
	a := vec2(.2+.06*sin(Time*.5), .1+.05*cos(Time*.4))
	b := vec2(.8+.05*cos(Time*.37), .45+.07*sin(Time*.31))
	d := vec2(.5+.07*sin(Time*.43), .95+.04*cos(Time*.29))
	c := Sheet.rgb
	c = mix(c, A.rgb, Strength*pool(pos, s*a, s*vec2(.5, .6)))
	c = mix(c, B.rgb, Strength*pool(pos, s*b, s*vec2(.45, .55)))
	c = mix(c, C.rgb, Strength*pool(pos, s*d, s*vec2(.42, .45)))
	return vec4(c, 1)
}
`)

// dissolve shows an element inside a circle growing from Origin.
var dissolve = shader.MustCompile(`package main

var Origin vec2
var Radius float

func ColorEffect(pos vec2, color vec4) vec4 {
	return color * (1 - smoothstep(Radius-48, Radius, length(pos-Origin)))
}
`)

// material fills an element with a glass level, or its solid low-power color.
type material struct {
	blur, sat float32
	fill      ui.Color
	solid     ui.Color
	low       bool
}

func (m material) PaintMaterial(p *ui.Painter, box ui.Rect, radii [4]float32) {
	if m.low || !p.Accelerated() {
		p.Fill(box, m.solid, radii[0])
		return
	}
	frost.Draw(p, box, shader.Options{
		BackdropBlur: m.blur,
		Radius:       radii[0],
		Uniforms:     map[string]any{"Tint": m.fill, "Sat": m.sat},
	})
}

type (
	pillMatch  struct{}
	panelMatch struct{}
)

var (
	snappy = ui.Animation{Spring: ui.SpringOptions{Duration: 260 * time.Millisecond, Bounce: .15}}
	smooth = ui.Animation{Spring: ui.SpringOptions{Duration: 360 * time.Millisecond}}
	gentle = ui.Animation{Spring: ui.SpringOptions{Duration: 520 * time.Millisecond, Bounce: .1}}
)

// matched is a transition with a shared identity, but for the old theme's copy:
// one element holds a Match at a time.
func (a *look) matched(anim *ui.Animation, id any) ui.ElementTransition {
	if !a.live {
		return ui.ElementTransition{}
	}
	return ui.ElementTransition{Animation: anim, Match: id}
}

type look struct {
	dark, low   bool
	tab         int // sidebar tab
	palette     bool
	find        bool
	welcome     bool
	themeGen    int
	themeOrigin ui.Point
	scroll      ui.ScrollState
	oldScroll   ui.ScrollState // of the old theme's copy during a dissolve
	live        bool           // false while building that copy
	auroraStart time.Time
	testAurora  bool // the Tester has no GPU; tests run the aurora anyway
	cpu         bool // the last frame was drawn without a GPU
	onFrame     func()
}

func (a *look) mat(level int, t palette) material {
	switch level {
	case 1:
		return material{16, 1.4, t.chip, t.solid, a.low}
	case 2:
		return material{24, 1.5, t.panel, t.solid, a.low}
	}
	return material{36, 1.6, t.modal, t.solid, a.low}
}

func (a *look) toggleTheme(at ui.Point) {
	a.dark = !a.dark
	a.themeGen++
	a.themeOrigin = at
}

func (a *look) view(c *ui.Context) {
	if a.onFrame != nil {
		a.onFrame()
	}
	a.live = true
	root := ui.Box(c).Fill().Clip()
	v := root.Animate("theme", float32(a.themeGen), 400*time.Millisecond)
	progress := v - float32(a.themeGen-1)
	root.Children(func() {
		if progress >= 1 || a.themeGen == 0 {
			a.shell(c.Key("shell"), a.dark)
			return
		}
		// The old theme stays underneath while the new one dissolves in.
		a.live, a.oldScroll.Y = false, a.scroll.Y
		a.shell(c.Key("old"), !a.dark)
		a.live = true
		over := ui.Box(c.Key("new")).Absolute().Fill()
		if a.low || a.cpu || c.Preferences().ReduceMotion {
			over = over.Opacity(progress)
		} else {
			w, h := c.Size()
			over = over.Effect(dissolve.Effect(shader.Options{Uniforms: map[string]any{
				"Origin": []float32{a.themeOrigin.X, a.themeOrigin.Y}, "Radius": progress * (w + h),
			}}))
		}
		over.Children(func() { a.shell(c.Key("shell"), a.dark) })
	})
}

func (a *look) shell(c *ui.Context, dark bool) {
	t := paletteFor(dark, a.low)
	ui.Column(c).Fill().Font(sans).TextColor(t.text).Draw(func(p *ui.Painter, r ui.Rect) {
		a.cpu = !p.Accelerated()
		if a.low || a.cpu {
			p.Fill(r, t.window, 0)
			return
		}
		tint.Draw(p, r, shader.Options{Uniforms: map[string]any{
			"Base": t.window, "A": t.aur1, "B": t.aur2, "C": t.aur3, "Strength": t.tintStrength,
		}})
	}).Children(func() {
		a.titleBar(c, t)
		ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			a.sidebar(c, t)
			a.sheet(c, t)
		})
	})
	if a.palette {
		a.commandPanel(c, t)
	}
}

func (a *look) titleBar(c *ui.Context, t palette) {
	bar := c.TitleBar()
	ui.Row(c).Height(max(bar.Height, 46)).Padding(0, bar.Right+8, 0, bar.Left+12).Gap(8).AlignItems(ui.Center).DragWindow().Children(func() {
		ui.Text(c, "A").Font(serif).FontSize(20).TextColor(t.accent)
		ui.Row(c).Padding(7, 12).Radius(9).Background(t.sheet).Children(func() {
			ui.Text(c, "README.md").FontSize(13).FontWeight(600)
		})
		ui.Row(c).Padding(7, 12).Children(func() { ui.Text(c, "笔记.md").FontSize(13).TextColor(t.muted) })
		ui.Spacer(c)
		ui.Box(c).Size(280, 30).Children(func() {
			if a.palette {
				return
			}
			om := ui.Row(c).Fill().Padding(0, 10).Gap(8).AlignItems(ui.Center).Radius(8).Background(t.surface).Border(1, t.border).
				Transition(a.matched(&gentle, panelMatch{})).Children(func() {
				ui.Text(c, "搜索文件或输入命令").FontSize(13).TextColor(t.muted).Grow(1)
				ui.Text(c, "Ctrl P").FontSize(11).TextColor(t.muted)
			})
			if om.Clicked() {
				a.palette = true
			}
		})
		ui.Spacer(c)
		for i, label := range []string{"查找", "欢迎", "低功耗", "主题"} {
			b := ui.Row(c.Key(i)).Padding(5, 8).Radius(7).Children(func() { ui.Text(c, label).FontSize(12).TextColor(t.muted) })
			if b.Hovered() {
				b.Background(t.hover)
			}
			if b.Clicked() {
				switch i {
				case 0:
					a.find = !a.find
				case 1:
					a.welcome = !a.welcome
					a.auroraStart = time.Time{}
				case 2:
					a.low = !a.low
				case 3:
					w, _ := c.Size()
					a.toggleTheme(ui.Point{X: w - 180, Y: 22})
				}
			}
		}
	})
}

func (a *look) sidebar(c *ui.Context, t palette) {
	ui.Column(c).Width(240).Padding(4, 8, 8, 12).Gap(2).Children(func() {
		ui.Row(c).Padding(3).Radius(10).Background(t.surface).Children(func() {
			for i, label := range []string{"文件", "大纲", "搜索"} {
				tab := ui.Box(c.Key(i)).Grow(1).Height(26)
				tab.Children(func() {
					if a.tab == i {
						ui.Box(c.Key("pill")).Absolute().Fill().Radius(7).Background(t.sheet).
							Transition(a.matched(&smooth, pillMatch{}))
					}
					ui.Row(c).Absolute().Fill().Justify(ui.Center).AlignItems(ui.Center).Children(func() {
						ui.Text(c, label).FontSize(13)
					})
				})
				if tab.Clicked() {
					a.tab = i
				}
			}
		})
		ui.Text(c, "aurink-notes").FontSize(11).TextColor(t.muted).Padding(14, 8, 6)
		for i, f := range []string{"docs", "  笔记.md", "  路线图.md", "  会议记录.md", "assets", "README.md"} {
			row := ui.Row(c.Key(f)).Padding(6, 8).Radius(8).Children(func() { ui.Text(c, f).FontSize(13) })
			if i == 5 {
				row.Background(t.surfaceHover)
			} else if row.Hovered() {
				row.Background(t.hover)
			}
		}
	})
}

func (a *look) sheet(c *ui.Context, t palette) {
	ui.Box(c).Grow(1).Margin(0, 8, 8, 0).Radius(14).Background(t.sheet).Clip().Children(func() {
		if a.welcome {
			a.welcomePage(c, t)
			return
		}
		scroll := &a.scroll
		if !a.live {
			scroll = &a.oldScroll
		}
		ui.Scroll(c).Absolute().Fill().TrackScroll(scroll).Children(func() {
			ui.Column(c).Width(660).AlignSelf(ui.Center).Padding(72, 0, 96).Gap(14).Children(func() {
				for i := range 20 {
					a.section(c.Key(i), t, i)
				}
			})
		})
		// What scrolls under the top edge frosts away. The blur also reads the
		// window above the sheet, which the soft edge covers.
		if !a.low {
			ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(40).PassThrough().Material(glass.Blur{Radius: 14, Mask: &ui.LinearGradient{
				From: ui.RGB(0, 0, 0), To: ui.Transparent, Angle: 180, Start: .2, End: 1,
			}})
		}
		ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(40).PassThrough().Material(glass.ScrollEdge{Background: t.sheet})
		if a.find {
			ui.Row(c).Absolute().Top(12).Right(16).Padding(8, 12).Gap(10).Radius(12).Border(1, t.border).
				Shadow(0, 12, 32, 0, ui.RGBA(0, 0, 0, .14)).Material(a.mat(2, t)).
				Transition(ui.ElementTransition{Animation: &snappy, Enter: &ui.Motion{Scale: .96, Y: -6}, Exit: &ui.Motion{Scale: .96, Y: -6}}).Children(func() {
				ui.Text(c, "查找内容").FontSize(13).TextColor(t.muted)
				ui.Text(c, "3 / 12").FontSize(12).TextColor(t.muted)
			})
		}
		ui.Row(c).Absolute().Bottom(16).Right(16).Padding(6, 14).Gap(10).Radius(999).Border(1, t.border).
			Shadow(0, 4, 14, 0, ui.RGBA(0, 0, 0, .10)).Material(a.mat(1, t)).Children(func() {
			ui.Text(c, "已保存").FontSize(12).TextColor(t.text)
			ui.Text(c, "789 字").FontSize(12).TextColor(t.muted)
		})
	})
}

func (a *look) section(c *ui.Context, t palette, i int) {
	ui.Column(c).Gap(14).Children(func() {
		if i == 0 {
			ui.Text(c, "Aurink 是什么").Font(serif).FontSize(41.6).FontWeight(500).LetterSpacing(-1).LineHeight(1.14)
		} else {
			ui.Text(c, fmt.Sprintf("写作时的感受 %d", i)).Font(serif).FontSize(26).FontWeight(500).LetterSpacing(-.3).Padding(14, 0, 0)
		}
		ui.Text(c, "轻量、即开即写的 Markdown 编辑器。Markdown 文件就是你的文件：打开、编辑、保存，都不会改动你没碰过的任何一个字节。").FontSize(18).LineHeight(1.7)
		ui.Text(c, "光标进入一段文字时，它的标记才淡淡地出现；离开之后，页面又回到干净的排版。这只是显示上的变化，源码本身从未被改写。").FontSize(16).LineHeight(1.75)
		ui.Text(c, "“内容优先：编辑区之外的一切默认安静，需要时才出现。”").Font(serif).Italic().FontSize(18.2).TextColor(t.muted)
		ui.Text(c, "func main() { fmt.Println(\"JetBrains Mono 代码\") }").Font("JetBrains Mono, Cascadia Code, monospace").FontSize(13.6).
			Padding(10, 12).Radius(8).Background(t.surface)
	})
}

func (a *look) welcomePage(c *ui.Context, t palette) {
	ui.Box(c).Absolute().Fill().Draw(func(p *ui.Painter, r ui.Rect) {
		if a.low || !(p.Accelerated() || a.testAurora) {
			p.Fill(r, t.sheet, 0)
			return
		}
		if a.auroraStart.IsZero() {
			a.auroraStart = p.Now()
		}
		e := p.Now().Sub(a.auroraStart)
		// Twelve seconds, then no more frames (§6).
		if e < 12*time.Second {
			p.AnimationFrame()
		} else {
			e = 12 * time.Second
		}
		aurora.Draw(p, r, shader.Options{Uniforms: map[string]any{
			"Sheet": t.sheet, "A": t.aur1, "B": t.aur2, "C": t.aur3, "Time": float32(e.Seconds()), "Strength": float32(.22),
		}})
	})
	ui.Column(c).Absolute().Fill().Justify(ui.Center).AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Text(c, "Aurink").Font(serif).FontSize(68).FontWeight(500).LetterSpacing(-2)
		ui.Text(c, "打开一个文件夹，开始写").FontSize(16).TextColor(t.muted)
	})
}

func (a *look) commandPanel(c *ui.Context, t palette) {
	ui.Overlay(c, func() {
		scrim := ui.Box(c).Absolute().Fill().Background(ui.RGBA(20, 22, 35, .14))
		if scrim.Clicked() {
			a.palette = false
		}
		w, _ := c.Size()
		ui.Column(c).Absolute().Top(70).Left((w-560)/2).Width(560).Height(340).Padding(10).Gap(2).Radius(16).Border(1, t.border).
			Shadow(0, 32, 80, 0, ui.RGBA(0, 0, 0, .22)).Material(a.mat(3, t)).
			Transition(a.matched(&gentle, panelMatch{})).Children(func() {
			ui.Text(c, "输入命令").FontSize(16).TextColor(t.muted).Padding(8, 10, 12)
			for i, cmd := range []string{"打开文件夹", "切换主题", "切换阅读模式", "导出为 HTML", "设置", "查找"} {
				row := ui.Row(c.Key(i)).Padding(8, 10).Radius(9).
					Transition(ui.ElementTransition{Animation: &snappy, Enter: &ui.Motion{Y: 6}}).Children(func() { ui.Text(c, cmd).FontSize(14) })
				if i == 0 {
					row.Background(t.accent.Alpha(.11))
				} else if row.Hovered() {
					row.Background(t.hover)
				}
			}
		})
	})
}
