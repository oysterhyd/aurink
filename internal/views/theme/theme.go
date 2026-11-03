// Package theme holds the design tokens of docs/02-product-ux.md §4. Components
// use these and write no literal colors, fonts or sizes of their own.
package theme

import "github.com/egoist/mygo/ui"

// DefaultHue is the accent hue.
const DefaultHue = 182

// Font stacks of §4.2. The first three families ship with the app.
const (
	Serif = "Newsreader, Noto Serif SC, Georgia, SimSun"
	Sans  = "Segoe UI Variable, Microsoft YaHei UI, SF Pro Text, PingFang SC, Cantarell, Noto Sans CJK SC"
	// FontSerif is Appearance.ContentFont's value for the serif body.
	FontSerif = "serif"
	Mono      = "JetBrains Mono, Cascadia Code, SF Mono, DejaVu Sans Mono"
)

// Sizes of §3 and §4.
const (
	Spacing      = 4
	Body         = 16
	UIText       = 13
	UIStatus     = 12
	UIBadge      = 11
	TitleBar     = 46
	SidebarWidth = 248
	WindowButton = 46
	RadiusButton = 8
	RadiusRow    = 9
	RadiusSheet  = 14
	RadiusFloat  = 12
	RadiusPill   = 999
	MaxColumn    = 720
	PadTop       = 72
	PadSide      = 28
	PadBottom    = 360
	MarginLabel  = 46
)

// Options choose a palette.
type Options struct {
	Dark, LowPower, HighContrast bool
	Hue                          float32
}

// Tokens are the colors of §4.1 and the fills of §4.3.
type Tokens struct {
	Dark, LowPower                       bool
	Window, Sheet, Text, Muted, Faint    ui.Color
	Surface, SurfaceHover, Hover, Border ui.Color
	Accent, AccentSoft, AccentRing       ui.Color
	Selection, OnAccent, Mark, CodeBg    ui.Color
	Warning, Success, Danger             ui.Color
	Scrim, ScrimStrong                   ui.Color
	SynKeyword, SynFunc, SynComment      ui.Color
	SynString                            ui.Color
	Aurora1, Aurora2, Aurora3            ui.Color
	Chip, Panel, Modal, Solid            ui.Color
	InkButton, OnInkButton, ShadowColor  ui.Color
	TintStrength                         float32
}

// For returns the tokens of a theme.
func For(o Options) *Tokens {
	h := o.Hue
	if h == 0 {
		h = DefaultHue
	}
	h64 := h
	var t Tokens
	if o.Dark {
		accent := ui.Oklch(.82, .12, h64)
		t = Tokens{
			Window: ui.RGB(0x09, 0x0A, 0x0F), Sheet: ui.RGB(0x11, 0x12, 0x18), Text: ui.RGB(0xEC, 0xED, 0xF2),
			Muted: ui.RGB(0xA2, 0xA4, 0xB3), Faint: ui.RGB(0x6C, 0x6E, 0x7E),
			Surface: ui.RGBA(255, 255, 255, .045), SurfaceHover: ui.RGBA(255, 255, 255, .10),
			Hover: ui.RGBA(255, 255, 255, .055), Border: ui.RGBA(255, 255, 255, .085),
			Accent: accent, AccentSoft: accent.Alpha(.17), AccentRing: accent.Alpha(.25), Selection: accent.Alpha(.30),
			OnAccent: ui.Hex("#08110F"), Mark: ui.RGBA(255, 196, 84, .26), CodeBg: ui.Hex("#0C0D12"),
			Warning: ui.Hex("#FFB224"), Success: ui.Hex("#4CC38A"), Danger: ui.Hex("#FF6369"),
			Scrim: ui.RGBA(4, 5, 9, .35), ScrimStrong: ui.RGBA(4, 5, 9, .55),
			SynKeyword: ui.Oklch(.78, .13, h64+110), SynFunc: accent, SynComment: ui.Hex("#7F8192"), SynString: ui.Hex("#E6C07B"),
			Aurora1: ui.Oklch(.80, .13, h64), Aurora2: ui.Oklch(.70, .15, h64+92), Aurora3: ui.Oklch(.74, .15, h64+165),
			Chip: ui.RGBA(30, 31, 40, .62), Panel: ui.RGBA(26, 27, 36, .68), Modal: ui.RGBA(22, 23, 31, .78), Solid: ui.Hex("#1A1B23"),
			InkButton: ui.Hex("#ECEDF2"), OnInkButton: ui.Hex("#111218"), ShadowColor: ui.RGBA(0, 0, 0, .45), TintStrength: .10,
		}
		if o.LowPower {
			t.Window = ui.Hex("#0B0C11")
		}
		if o.HighContrast {
			t.Muted, t.Border = ui.Hex("#C8CAD6"), ui.RGBA(255, 255, 255, .22)
			t.AccentRing = accent
		}
	} else {
		accent := ui.Oklch(.50, .11, h64)
		t = Tokens{
			Window: ui.Hex("#E8E9EE"), Sheet: ui.RGB(255, 255, 255), Text: ui.Hex("#15161C"),
			Muted: ui.Hex("#5F616E"), Faint: ui.Hex("#8B8D99"),
			Surface: ui.RGBA(20, 22, 35, .04), SurfaceHover: ui.RGBA(20, 22, 35, .09),
			Hover: ui.RGBA(20, 22, 35, .05), Border: ui.RGBA(20, 22, 35, .09),
			Accent: accent, AccentSoft: accent.Alpha(.11), AccentRing: accent.Alpha(.20), Selection: accent.Alpha(.20),
			OnAccent: ui.RGB(255, 255, 255), Mark: ui.Hex("#FFEFB0"), CodeBg: ui.Hex("#F7F7F9"),
			Warning: ui.Hex("#C77A00"), Success: ui.Hex("#1E8A58"), Danger: ui.Hex("#D93D42"),
			Scrim: ui.RGBA(20, 22, 35, .14), ScrimStrong: ui.RGBA(20, 22, 35, .28),
			SynKeyword: ui.Oklch(.46, .16, h64+110), SynFunc: accent, SynComment: ui.Hex("#70727F"), SynString: ui.Hex("#946100"),
			Aurora1: ui.Oklch(.66, .14, h64), Aurora2: ui.Oklch(.58, .17, h64+92), Aurora3: ui.Oklch(.66, .17, h64+165),
			Chip: ui.RGBA(255, 255, 255, .68), Panel: ui.RGBA(255, 255, 255, .72), Modal: ui.RGBA(252, 252, 254, .82), Solid: ui.Hex("#FBFBFC"),
			InkButton: ui.Hex("#15161C"), OnInkButton: ui.RGB(255, 255, 255), ShadowColor: ui.RGBA(0, 0, 0, .14), TintStrength: .13,
		}
		if o.LowPower {
			t.Window = ui.Hex("#E9EAEE")
		}
		if o.HighContrast {
			t.Muted, t.Border = ui.Hex("#3F414C"), ui.RGBA(20, 22, 35, .28)
			t.AccentRing = accent
		}
	}
	t.Dark, t.LowPower = o.Dark, o.LowPower
	return &t
}

// Typography is the document's type: the body size and line height and the
// column width of Settings.Appearance, with the window zoom applied, and a
// serif body for reading mode (02 §2.2, §5.10). The other levels of §4.2 keep
// their ratios to the body.
type Typography struct {
	Body       float32 // body text size, DIP
	LineHeight float32 // body line height, times the size
	Column     float32 // widest text column, DIP
	Serif      bool    // body text in the serif face
}

// DefaultTypography is §4.2 at 100%.
func DefaultTypography() Typography {
	return Typography{Body: Body, LineHeight: 1.75, Column: MaxColumn}
}

// Zoomed scales the sizes by z (1 is 100%).
func (ty Typography) Zoomed(z float32) Typography {
	ty.Body *= z
	ty.Column *= z
	return ty
}

func (ty Typography) body() string {
	if ty.Serif {
		return Serif
	}
	return Sans
}

// HeadingFont returns the font of a heading level (§4.2).
func (ty Typography) HeadingFont(level int) ui.Font {
	b := ty.Body
	switch level {
	case 1:
		return ui.Font{Family: Serif, Size: b * 2.6, Weight: 500, LineHeight: 1.14}
	case 2:
		return ui.Font{Family: Serif, Size: b * 1.62, Weight: 500, LineHeight: 1.3}
	case 3:
		return ui.Font{Family: ty.body(), Size: b * 1.12, Weight: 650, LineHeight: 1.5}
	case 6:
		return ui.Font{Family: ty.body(), Size: b * .95, Weight: 600, LineHeight: 1.5}
	}
	return ui.Font{Family: ty.body(), Size: b, Weight: 600, LineHeight: 1.5}
}

// BodyFont is the body text.
func (ty Typography) BodyFont() ui.Font {
	return ui.Font{Family: ty.body(), Size: ty.Body, LineHeight: ty.LineHeight}
}

// LeadFont is the first paragraph's level.
func (ty Typography) LeadFont() ui.Font {
	return ui.Font{Family: ty.body(), Size: ty.Body * 1.14, LineHeight: ty.LineHeight - .05}
}

// CodeFont is the level of code and of shown markers.
func (ty Typography) CodeFont() ui.Font {
	return ui.Font{Family: Mono, Size: ty.Body * .85, LineHeight: 1.55}
}

// HeadingFont, BodyFont, LeadFont and CodeFont are the levels at 100%.
func HeadingFont(level int) ui.Font { return DefaultTypography().HeadingFont(level) }

// BodyFont is the body text at 100%.
func BodyFont() ui.Font { return DefaultTypography().BodyFont() }

// LeadFont is the first paragraph's level at 100%.
func LeadFont() ui.Font { return DefaultTypography().LeadFont() }

// CodeFont is the level of code at 100%.
func CodeFont() ui.Font { return DefaultTypography().CodeFont() }
