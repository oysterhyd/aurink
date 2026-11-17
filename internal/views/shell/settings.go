package shell

import (
	"fmt"
	"math"
	"os"
	"slices"
	"strings"

	"aurink/internal/contract"
	"aurink/internal/views/effects"
	"aurink/internal/views/kit"
	"aurink/internal/views/motion"
	"aurink/internal/views/theme"
	"aurink/locales"
	"github.com/egoist/mygo/ui"
)

// The settings dialog (FE-2.7, 02 §5.15): 820×580, five categories, every
// change applied at once through the settings store.

const (
	catAppearance = iota
	catEditor
	catFiles
	catExtensions
	catShortcuts
)

// Accent hues of 02 §2.6.
var accentHues = []struct {
	hue  int
	name func() string
}{
	{182, locales.SettingsHueTeal}, {278, locales.SettingsHueIris}, {48, locales.SettingsHueEmber}, {140, locales.SettingsHueMoss},
}

// The ranges of 02 §3.2 and §5.15.
var (
	fontSizeRange   = sliderRange{13, 24, 1}
	lineHeightRange = sliderRange{1.4, 2.2, .05}
	widthRange      = sliderRange{600, 960, 20}
)

type sliderRange struct{ lo, hi, step float64 }

func (r sliderRange) snap(v float64) float64 {
	return math.Round(max(r.lo, min(v, r.hi))/r.step) * r.step
}

type shortcutConflict struct {
	cmd    contract.CommandID
	chord  contract.Chord
	others []contract.CommandID
}

// settingsView is the dialog's state. Sliders and switches bind values the
// toolkit writes between frames; they live here, synced from the store.
type settingsView struct {
	open, about   bool
	cat           int
	floats        map[string]*float64
	bools         map[string]*bool
	shortcutQuery string
	recording     contract.CommandID
	focusRec      bool
	recWarn       string
	conflict      *shortcutConflict
}

func (v *settingsView) float(key string) *float64 {
	if v.floats == nil {
		v.floats = map[string]*float64{}
	}
	p := v.floats[key]
	if p == nil {
		p = new(float64)
		v.floats[key] = p
	}
	return p
}

func (v *settingsView) bool(key string) *bool {
	if v.bools == nil {
		v.bools = map[string]*bool{}
	}
	p := v.bools[key]
	if p == nil {
		p = new(bool)
		v.bools[key] = p
	}
	return p
}

// OpenSettings shows the settings dialog.
func (s *Shell) OpenSettings() {
	s.reloadCustomThemes()
	s.set.recording = ""
	s.set.recWarn = ""
	s.set.conflict = nil
	s.set.open = true
	s.invalidate()
}

// SettingsOpen reports whether the settings dialog shows.
func (s *Shell) SettingsOpen() bool { return s.set.open }

// CloseSettings hides the settings dialog, as Esc does. For tools and tests.
func (s *Shell) CloseSettings() {
	s.set.recording = ""
	s.set.recWarn = ""
	s.set.conflict = nil
	s.set.open = false
	s.invalidate()
}

// update changes the settings and reports a failed write.
func (s *Shell) update(fn func(v *contract.Settings)) {
	if err := s.o.Settings.Update(fn); err != nil {
		s.fail(err)
	}
	s.applyTheme()
}

func (s *Shell) settingsView(c *ui.Context, t *theme.Tokens) {
	if !s.live {
		return
	}
	was := s.set.open
	ui.DialogBase(c.Key("settings"), &s.set.open, func(backdrop, panel ui.Element) {
		backdrop.Background(t.Scrim)
		panel.Width(820).Height(580).Radius(theme.RadiusFloat+4).Border(1, t.Border).
			Shadow(0, 32, 80, 0, t.ShadowColor).Material(effects.Glass(t, effects.Modal)).Clip().Label(locales.SettingsTitle())
		ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
			s.settingsNav(c, t)
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				ui.Row(c).Padding(22, 18, 8, 26).AlignItems(ui.Center).Children(func() {
					ui.Text(c, s.catTitle(s.set.cat)).Font(theme.Serif).FontSize(24).FontWeight(500).Grow(1)
					if kit.IconButton(c.Key("close"), t, kit.IconClose, locales.SettingsClose(), 32, 14).Clicked() {
						s.set.open = false
					}
				})
				ui.Scroll(c.Key(s.set.cat)).Grow(1).MinHeight(0).Padding(0, 26, 22, 26).Children(func() {
					switch s.set.cat {
					case catEditor:
						s.editorPage(c, t)
					case catFiles:
						s.filesPage(c, t)
					case catExtensions:
						s.extensionsPage(c, t)
					case catShortcuts:
						s.shortcutsPage(c, t)
					default:
						s.appearancePage(c, t)
					}
				})
			})
		})
	})
	if was && !s.set.open {
		s.focusNext = true
	}
	s.aboutView(c, t)
}

func (s *Shell) catTitle(i int) string {
	return []func() string{locales.SettingsAppearance, locales.SettingsEditor, locales.SettingsFiles,
		locales.SettingsExtensions, locales.SettingsShortcuts}[i]()
}

func (s *Shell) settingsNav(c *ui.Context, t *theme.Tokens) {
	ui.Column(c.Key("nav")).Width(176).Padding(18, 10).Gap(2).Background(t.Surface).Role(ui.RoleTabList).Children(func() {
		for i := range catShortcuts + 1 {
			label := s.catTitle(i)
			b := ui.Box(c.Key(i)).Height(32).Role(ui.RoleTab).Label(label).Checked(i == s.set.cat).Focusable()
			b.Children(func() {
				if i == s.set.cat {
					pill := ui.Box(c.Key("pill")).Absolute().Fill().Radius(8).Background(t.AccentSoft)
					if s.live {
						pill.Transition(motion.Still(s.still, ui.ElementTransition{Animation: &motion.Snappy, Match: segmentPillOf("settingsNav")}))
					}
				}
				tone, weight := t.Muted, 400
				if i == s.set.cat {
					tone, weight = t.Text, 500
				}
				ui.Row(c).Absolute().Fill().Padding(0, 10).AlignItems(ui.Center).Children(func() {
					ui.Text(c, label).FontSize(theme.UIText + 1).FontWeight(weight).TextColor(tone)
				})
			})
			if b.Clicked() {
				s.set.cat = i
			}
		}
	})
}

type segmentPillOf string

// row is one setting: its label and description on the left, its control on the right.
func (s *Shell) row(c *ui.Context, t *theme.Tokens, key, label, hint string, control func()) {
	ui.Row(c.Key(key)).Padding(11, 0).Gap(24).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, label).FontSize(theme.UIText + 1)
			if hint != "" {
				ui.Text(c, hint).FontSize(theme.UIStatus).TextColor(t.Muted)
			}
		})
		control()
	})
}

// choice is a segmented control over options; it returns the clicked index or -1.
func (s *Shell) choice(c *ui.Context, t *theme.Tokens, key string, options []string, selected int) int {
	clicked := -1
	ui.Row(c.Key(key)).Padding(2).Radius(8).Background(t.Surface).Role(ui.RoleRadioGroup).Children(func() {
		for i, label := range options {
			b := ui.Box(c.Key(i)).Height(26).Role(ui.RoleRadio).Label(label).Checked(i == selected).Focusable()
			b.Children(func() {
				if i == selected {
					pill := ui.Box(c.Key("pill")).Absolute().Fill().Radius(6).Background(t.Sheet).Shadow(0, 1, 2, 0, t.ShadowColor)
					if s.live {
						pill.Transition(motion.Still(s.still, ui.ElementTransition{Animation: &motion.Snappy, Match: segmentPillOf(key)}))
					}
				}
				tone := t.Muted
				if i == selected {
					tone = t.Text
				}
				// The label in the flow sizes the button; the one drawn on the pill shows.
				ui.Text(c, label).FontSize(theme.UIStatus).Padding(5, 11).Opacity(0)
				ui.Row(c).Absolute().Fill().Justify(ui.Center).AlignItems(ui.Center).PassThrough().Children(func() {
					ui.Text(c, label).FontSize(theme.UIStatus).TextColor(tone)
				})
			})
			if b.Clicked() && i != selected {
				clicked = i
			}
		}
	})
	return clicked
}

// toggle is a switch; the knob springs across (02 §5.15).
func (s *Shell) toggle(c *ui.Context, t *theme.Tokens, key, label string, on bool, set func(bool)) {
	p := s.set.bool(key)
	sw := ui.SwitchBase(c.Key(key), p).Size(38, 22).Label(label)
	if sw.Changed() {
		set(*p)
		on = *p
	} else {
		*p = on
	}
	var want float32
	if on {
		want = 1
	}
	knob := sw.AnimateTo("knob", want, motion.Snappy)
	if !s.live {
		knob = want
	}
	track := t.SurfaceHover
	if on {
		track = t.Accent
	}
	sw.Radius(11).Background(track).Children(func() {
		ui.Box(c).Absolute().Top(2).Left(2+16*knob).Size(18, 18).Radius(9).Background(ui.RGB(255, 255, 255)).
			Shadow(0, 1, 2, 0, t.ShadowColor)
	})
}

// slider sets a number in a range and shows it.
func (s *Shell) slider(c *ui.Context, t *theme.Tokens, key, label string, r sliderRange, value float64, format string, set func(float64)) {
	p := s.set.float(key)
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		sl := ui.SliderBase(c.Key(key), p, r.lo, r.hi).Step(r.step).Width(150).Height(16).PaddingX(8).Label(label)
		if sl.Changed() {
			if v := r.snap(*p); v != value {
				set(v)
				value = v
			}
		} else {
			*p = value
		}
		f := float32((value - r.lo) / (r.hi - r.lo))
		accent, rail := t.Accent, t.SurfaceHover
		sl.Draw(func(pt *ui.Painter, b ui.Rect) {
			x0, w := b.X+8, b.W-16
			pt.Fill(ui.Rect{X: x0, Y: b.Y + b.H/2 - 2, W: w, H: 4}, rail, 2)
			pt.Fill(ui.Rect{X: x0, Y: b.Y + b.H/2 - 2, W: w * f, H: 4}, accent, 2)
			pt.Fill(ui.Rect{X: x0 + w*f - 8, Y: b.Y, W: 16, H: 16}, accent, 8)
		})
		ui.Text(c, fmt.Sprintf(format, value)).FontSize(theme.UIStatus).TextColor(t.Muted).Width(40).TextAlign(ui.End)
	})
}

func (s *Shell) appearancePage(c *ui.Context, t *theme.Tokens) {
	v := s.o.Settings.Get()
	a := v.Appearance
	// The theme cards: a window with a sheet and two lines of text.
	ui.Row(c.Key("themes")).Gap(12).Padding(10, 0, 14, 0).Role(ui.RoleRadioGroup).Label(locales.SettingsAppearance()).Children(func() {
		cards := []struct {
			mode, label string
			dark        int // 0 light, 1 dark, 2 both
		}{
			{"light", locales.SettingsThemeLight(), 0}, {"dark", locales.SettingsThemeDark(), 1}, {"system", locales.SettingsThemeSystem(), 2},
		}
		for _, card := range cards {
			on := a.Theme == card.mode || (card.mode == "system" && a.Theme == "")
			b := ui.Column(c.Key(card.mode)).Grow(1).Basis(0).Gap(6).Role(ui.RoleRadio).Label(card.label).Checked(on).Focusable()
			b.Children(func() {
				light, dark := theme.For(theme.Options{Hue: float32(a.AccentHue)}), theme.For(theme.Options{Dark: true, Hue: float32(a.AccentHue)})
				ring := t.Border
				width := float32(1)
				if on {
					ring, width = t.Accent, 2
				}
				// The system card splits each color along a diagonal, light on the left.
				paint := func(e ui.Element, pick func(k *theme.Tokens) ui.Color) ui.Element {
					if card.dark == 2 {
						return e.LinearGradient(ui.LinearGradient{From: pick(light), To: pick(dark), Angle: 115, Start: .5, End: .5})
					}
					return e.Background(pick([]*theme.Tokens{light, dark}[card.dark]))
				}
				paint(ui.Box(c).Height(82).Radius(12).Clip().Border(width, ring).Padding(8, 8, 0, 28), func(k *theme.Tokens) ui.Color { return k.Window }).Children(func() {
					paint(ui.Column(c).Fill().Padding(10).Gap(8).Radius(7), func(k *theme.Tokens) ui.Color { return k.Sheet }).Children(func() {
						paint(ui.Box(c).Height(5).WidthPercent(54).Radius(3), func(k *theme.Tokens) ui.Color { return k.Text })
						paint(ui.Box(c).Height(3).WidthPercent(86).Radius(3), func(k *theme.Tokens) ui.Color { return k.Text.Alpha(.18) })
						paint(ui.Box(c).Height(3).WidthPercent(70).Radius(3), func(k *theme.Tokens) ui.Color { return k.Text.Alpha(.18) })
					})
				})
				w := 400
				if on {
					w = 600
				}
				ui.Text(c, card.label).FontSize(theme.UIStatus).FontWeight(w)
			})
			if b.Clicked() && !on {
				mode := card.mode
				r := b.Bounds()
				s.setTheme(mode, ui.Point{X: r.X + r.W/2, Y: r.Y + r.H/2})
			}
		}
	})
	if s.o.ThemesDir != "" || len(s.customThemes) > 0 {
		s.customThemesSection(c, t, a.Theme)
	}
	s.row(c, t, "accent", locales.SettingsAccent(), locales.SettingsAccentHint(), func() {
		ui.Row(c).Gap(8).Role(ui.RoleRadioGroup).Label(locales.SettingsAccent()).Children(func() {
			for _, h := range accentHues {
				on := a.AccentHue == h.hue
				sw := ui.Box(c.Key(h.hue)).Size(22, 22).Radius(11).Background(theme.For(theme.Options{Hue: float32(h.hue)}).Accent).
					Role(ui.RoleRadio).Label(h.name()).Checked(on).Focusable()
				if on {
					sw.Border(2, t.Sheet).Shadow(0, 0, 0, 2, theme.For(theme.Options{Hue: float32(h.hue)}).Accent)
				}
				if sw.Clicked() && !on {
					hue := h.hue
					s.update(func(v *contract.Settings) { v.Appearance.AccentHue = hue })
				}
			}
		})
	})
	s.row(c, t, "font", locales.SettingsBodyFont(), "", func() {
		sel := 0
		if a.ContentFont == theme.FontSerif {
			sel = 1
		}
		if i := s.choice(c, t, "font", []string{locales.SettingsSans(), locales.SettingsSerif()}, sel); i >= 0 {
			s.update(func(v *contract.Settings) { v.Appearance.ContentFont = []string{"", theme.FontSerif}[i] })
		}
	})
	s.row(c, t, "size", locales.SettingsFontSize(), "13–24", func() {
		s.slider(c, t, "size", locales.SettingsFontSize(), fontSizeRange, float64(a.FontSize), "%.0f", func(x float64) {
			s.update(func(v *contract.Settings) { v.Appearance.FontSize = float32(x) })
		})
	})
	s.row(c, t, "lineHeight", locales.SettingsLineHeight(), locales.SettingsLineHeightHint(), func() {
		s.slider(c, t, "lineHeight", locales.SettingsLineHeight(), lineHeightRange, float64(a.LineHeight), "%.2f", func(x float64) {
			s.update(func(v *contract.Settings) { v.Appearance.LineHeight = float32(x) })
		})
	})
	s.row(c, t, "width", locales.SettingsContentWidth(), "600–960", func() {
		s.slider(c, t, "width", locales.SettingsContentWidth(), widthRange, float64(a.ContentWidth), "%.0f", func(x float64) {
			s.update(func(v *contract.Settings) { v.Appearance.ContentWidth = float32(x) })
		})
	})
	s.row(c, t, "motion", locales.SettingsReduceMotion(), locales.SettingsReduceMotionHint(), func() {
		modes := []string{"system", "on", "off"}
		if i := s.choice(c, t, "motion", []string{locales.SettingsSystem(), locales.SettingsOn(), locales.SettingsOff()},
			max(0, slices.Index(modes, a.ReduceMotion))); i >= 0 {
			s.update(func(v *contract.Settings) { v.Appearance.ReduceMotion = modes[i] })
		}
	})
	s.row(c, t, "transparency", locales.SettingsReduceTransparency(), locales.SettingsReduceTransparencyHint(), func() {
		s.toggle(c, t, "transparency", locales.SettingsReduceTransparency(), a.ReduceTransparency, func(on bool) {
			s.update(func(v *contract.Settings) { v.Appearance.ReduceTransparency = on })
		})
	})
	s.row(c, t, "language", locales.SettingsLanguage(), "", func() {
		// Language names are in their own language.
		tags := []string{"", "zh-Hans", "en"}
		if i := s.choice(c, t, "language", []string{locales.SettingsSystem(), "简体中文", "English"},
			max(0, slices.Index(tags, v.Language))); i >= 0 {
			s.update(func(v *contract.Settings) { v.Language = tags[i] })
			if s.o.SetLanguage != nil {
				s.o.SetLanguage(tags[i])
			}
		}
	})
}

func (s *Shell) customThemesSection(c *ui.Context, t *theme.Tokens, activeMode string) {
	s.row(c, t, "customThemes", locales.SettingsCustomThemes(), locales.SettingsCustomThemesHint(), func() {
		if s.o.ThemesDir != "" && s.o.OpenPath != nil {
			b := ui.Row(c.Key("openThemes")).Height(28).Padding(0, 10).AlignItems(ui.Center).Radius(8).
				Background(t.Surface).Border(1, t.Border).Role(ui.RoleButton).Label(locales.SettingsOpenThemesFolder()).Focusable()
			if b.Hovered() {
				b.Background(t.SurfaceHover)
			}
			b.Children(func() {
				ui.Text(c, locales.SettingsOpenThemesFolder()).FontSize(theme.UIStatus).SingleLine()
			})
			if b.Clicked() {
				_ = os.MkdirAll(s.o.ThemesDir, 0o755)
				s.o.OpenPath(s.o.ThemesDir)
			}
		}
	})
	if len(s.customThemes) > 0 {
		ui.Row(c.Key("customThemeList")).Wrap().Gap(8).Padding(0, 0, 10, 0).
			Role(ui.RoleRadioGroup).Label(locales.SettingsCustomThemes()).Children(func() {
			for _, ct := range s.customThemes {
				on := activeMode == ct.ID
				sample := ct.Tokens(theme.Options{Dark: ct.Dark, Hue: ct.AccentHue})
				chip := ui.Row(c.Key(ct.ID)).Height(30).Padding(0, 10).Gap(8).AlignItems(ui.Center).Radius(8).
					Background(t.Surface).Border(1, t.Border).Role(ui.RoleRadio).Label(ct.Name).Checked(on).Focusable()
				if on {
					chip.Background(t.AccentSoft).Border(1.5, t.Accent)
				} else if chip.Hovered() {
					chip.Background(t.SurfaceHover)
				}
				chip.Children(func() {
					ui.Box(c).Size(12, 12).Radius(6).Background(sample.Sheet).Border(2, sample.Accent)
					w := 400
					if on {
						w = 600
					}
					ui.Text(c, ct.Name).FontSize(theme.UIStatus).FontWeight(w).SingleLine()
				})
				if chip.Clicked() && !on {
					id := ct.ID
					r := chip.Bounds()
					s.setTheme(id, ui.Point{X: r.X + r.W/2, Y: r.Y + r.H/2})
				}
			}
		})
	}
}

// setTheme chooses light, dark, system, or a custom theme ID, dissolving from a point.
func (s *Shell) setTheme(mode string, at ui.Point) {
	old := s.palette(s.dark)
	wantDark := s.dark
	switch {
	case strings.HasPrefix(mode, theme.CustomPrefix):
		if ct, ok := s.customTheme(mode); ok {
			wantDark = ct.Dark
		}
	case mode == "dark":
		wantDark = true
	case mode == "light":
		wantDark = false
	}
	s.prevTheme = old
	s.dark = wantDark
	s.themeGen++
	s.origin = at
	s.update(func(v *contract.Settings) { v.Appearance.Theme = mode })
}

func (s *Shell) editorPage(c *ui.Context, t *theme.Tokens) {
	e := s.o.Settings.Get().Editor
	s.row(c, t, "reveal", locales.SettingsReveal(), locales.SettingsRevealHint(), func() {
		if i := s.choice(c, t, "reveal", []string{locales.SettingsRevealInline(), locales.SettingsRevealBlock(), locales.SettingsRevealAlways()}, int(e.Reveal)); i >= 0 {
			s.update(func(v *contract.Settings) { v.Editor.Reveal = contract.RevealMode(i) })
		}
	})
	s.row(c, t, "softBreak", locales.SettingsSoftBreak(), locales.SettingsSoftBreakHint(), func() {
		if i := s.choice(c, t, "softBreak", []string{locales.SettingsSoftBreakLine(), locales.SettingsSoftBreakSpace()}, int(e.SoftBreak)); i >= 0 {
			s.update(func(v *contract.Settings) { v.Editor.SoftBreak = contract.SoftBreakMode(i) })
		}
	})
	s.row(c, t, "autoPair", locales.SettingsAutoPair(), "", func() {
		s.toggle(c, t, "autoPair", locales.SettingsAutoPair(), e.AutoPair, func(on bool) {
			s.update(func(v *contract.Settings) { v.Editor.AutoPair = on })
		})
	})
	s.row(c, t, "smoothCaret", locales.SettingsSmoothCaret(), locales.SettingsSmoothCaretHint(), func() {
		s.toggle(c, t, "smoothCaret", locales.SettingsSmoothCaret(), e.SmoothCaret, func(on bool) {
			s.update(func(v *contract.Settings) { v.Editor.SmoothCaret = on })
		})
	})
	marks := func(key, label string, options []string, cur string, set func(v *contract.Settings, x string)) {
		s.row(c, t, key, label, "", func() {
			if i := s.choice(c, t, key, options, max(0, slices.Index(options, cur))); i >= 0 {
				s.update(func(v *contract.Settings) { set(v, options[i]) })
			}
		})
	}
	marks("bullet", locales.SettingsBullet(), []string{"-", "*", "+"}, e.BulletMarker, func(v *contract.Settings, x string) { v.Editor.BulletMarker = x })
	marks("strong", locales.SettingsStrong(), []string{"**", "__"}, e.StrongDelim, func(v *contract.Settings, x string) { v.Editor.StrongDelim = x })
	marks("emphasis", locales.SettingsEmphasis(), []string{"*", "_"}, e.EmphasisDelim, func(v *contract.Settings, x string) { v.Editor.EmphasisDelim = x })
	s.row(c, t, "tabSize", locales.SettingsTabSize(), "", func() {
		sizes := []int{2, 4}
		if i := s.choice(c, t, "tabSize", []string{"2", "4"}, max(0, slices.Index(sizes, e.TabSize))); i >= 0 {
			s.update(func(v *contract.Settings) { v.Editor.TabSize = sizes[i] })
		}
	})
}

func (s *Shell) filesPage(c *ui.Context, t *theme.Tokens) {
	f := s.o.Settings.Get().Files
	flag := func(key, label, hint string, on bool, set func(v *contract.Settings, on bool)) {
		s.row(c, t, key, label, hint, func() {
			s.toggle(c, t, key, label, on, func(on bool) { s.update(func(v *contract.Settings) { set(v, on) }) })
		})
	}
	flag("autosave", locales.SettingsAutosave(), locales.SettingsAutosaveHint(), f.Autosave, func(v *contract.Settings, on bool) { v.Files.Autosave = on })
	flag("restore", locales.SettingsRestoreSession(), "", f.RestoreSession, func(v *contract.Settings, on bool) { v.Files.RestoreSession = on })
	flag("showAll", locales.SettingsShowAllFiles(), "", f.ShowAllFiles, func(v *contract.Settings, on bool) { v.Files.ShowAllFiles = on })
	flag("copyImages", locales.SettingsCopyImages(), "", f.CopyImages, func(v *contract.Settings, on bool) { v.Files.CopyImages = on })
	s.row(c, t, "imageDir", locales.SettingsImageDir(), "", func() {
		dirs := []string{"./assets", "./${filename}.assets"}
		if i := s.choice(c, t, "imageDir", dirs, max(0, slices.Index(dirs, f.ImageDir))); i >= 0 {
			s.update(func(v *contract.Settings) { v.Files.ImageDir = dirs[i] })
		}
	})
}

// extensions are the syntax switches, in the order of 02 §2.1.
var extensions = []struct {
	bit         contract.Extensions
	label, hint func() string
}{
	{contract.ExtFrontMatter, locales.SettingsExtFrontMatter, locales.SettingsExtFrontMatterHint},
	{contract.ExtDollarMath, locales.SettingsExtMath, locales.SettingsExtMathHint},
	{contract.ExtHighlight, locales.SettingsExtHighlight, nil},
	{contract.ExtSupSub, locales.SettingsExtSupSub, nil},
	{contract.ExtTildeSub, locales.SettingsExtTildeSub, locales.SettingsExtTildeSubHint},
	{contract.ExtEmoji, locales.SettingsExtEmoji, nil},
	{contract.ExtWikiLink, locales.SettingsExtWiki, nil},
	{contract.ExtTOC, locales.SettingsExtToc, nil},
}

// extensionsPage switches the syntax extensions. Like Typora's, they apply
// to documents opened afterwards: an open document keeps how it was parsed.
func (s *Shell) extensionsPage(c *ui.Context, t *theme.Tokens) {
	exts := s.o.Settings.Get().Editor.Exts
	ui.Text(c, locales.SettingsExtHint()).FontSize(theme.UIStatus).TextColor(t.Muted).Padding(0, 0, 10, 0)
	for _, x := range extensions {
		hint := ""
		if x.hint != nil {
			hint = x.hint()
		}
		bit := x.bit
		key := fmt.Sprint("ext", bit)
		s.row(c, t, key, x.label(), hint, func() {
			s.toggle(c, t, key, x.label(), exts&bit != 0, func(on bool) {
				s.update(func(v *contract.Settings) {
					if on {
						v.Editor.Exts |= bit
					} else {
						v.Editor.Exts &^= bit
					}
				})
			})
		})
	}
}

// shortcutsPage lists registered commands with search filtering, interactive
// chord recording, conflict detection/override, and per-item / global reset
// (09 D11, FE-2.8).
func (s *Shell) shortcutsPage(c *ui.Context, t *theme.Tokens) {
	keymap := s.o.Settings.Get().Keymap
	ui.Row(c.Key("shortcutTop")).Gap(8).Padding(4, 0, 6, 0).AlignItems(ui.Center).Children(func() {
		ui.SearchField(c.Key("shortcutFilter"), &s.set.shortcutQuery).
			Placeholder(locales.SettingsShortcutsSearch()).
			Label(locales.SettingsShortcutsSearch()).
			Grow(1).Shrink(1).MinWidth(0)
		resetAll := ui.Row(c.Key("resetAll")).Height(30).Padding(0, 10).AlignItems(ui.Center).Radius(8).
			Background(t.Surface).Border(1, t.Border).Role(ui.RoleButton).Label(locales.SettingsShortcutsResetAll()).Focusable()
		if len(keymap) == 0 {
			resetAll.Opacity(0.45)
		} else if resetAll.Hovered() {
			resetAll.Background(t.SurfaceHover)
		}
		resetAll.Children(func() {
			ui.Text(c, locales.SettingsShortcutsResetAll()).FontSize(theme.UIStatus).SingleLine()
		})
		if resetAll.Clicked() && len(keymap) > 0 {
			s.resetAllShortcuts()
		}
	})
	ui.Text(c, locales.SettingsShortcutsHint()).FontSize(theme.UIStatus).TextColor(t.Muted).Padding(0, 0, 10, 0)

	q := strings.ToLower(strings.TrimSpace(s.set.shortcutQuery))
	overridden := map[contract.CommandID]bool{}
	for _, kb := range keymap {
		overridden[kb.Command] = true
	}

	for _, cmd := range s.o.Commands.All() {
		keys := s.o.Commands.KeysFor(cmd.ID)
		title := Title(cmd.Title)
		var labels []string
		for _, k := range keys {
			labels = append(labels, chordLabel(k))
		}
		if q != "" {
			matched := strings.Contains(strings.ToLower(title), q) ||
				strings.Contains(strings.ToLower(string(cmd.ID)), q)
			if !matched {
				for _, l := range labels {
					if strings.Contains(strings.ToLower(l), q) {
						matched = true
						break
					}
				}
			}
			if !matched {
				continue
			}
		}

		isRec := s.set.recording == cmd.ID
		var subHint string
		if isRec && s.set.recWarn != "" {
			subHint = s.set.recWarn
		} else if s.set.conflict != nil && s.set.conflict.cmd == cmd.ID {
			subHint = locales.SettingsShortcutsConflict(s.conflictNames(s.set.conflict.others))
		}

		id := cmd.ID
		custom := overridden[id]
		s.row(c, t, string(id), title, subHint, func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				if s.set.conflict != nil && s.set.conflict.cmd == id {
					cf := *s.set.conflict
					ui.Text(c, chordLabel(cf.chord)).FontSize(theme.UIBadge).FontWeight(600).TextColor(t.Warning).
						Padding(3, 8).Radius(6).Border(1, t.Warning).Background(t.Surface)
					overBtn := ui.Row(c.Key("override")).Height(26).Padding(0, 8).AlignItems(ui.Center).Radius(6).
						Background(t.InkButton).Role(ui.RoleButton).Label(locales.SettingsShortcutsOverride()).Focusable()
					overBtn.Children(func() {
						ui.Text(c, locales.SettingsShortcutsOverride()).FontSize(theme.UIBadge).FontWeight(600).TextColor(t.OnInkButton).SingleLine()
					})
					if overBtn.Clicked() {
						s.applyShortcut(cf.cmd, cf.chord, cf.others)
						s.set.conflict = nil
					}
					cancelBtn := ui.Row(c.Key("cancelConflict")).Height(26).Padding(0, 8).AlignItems(ui.Center).Radius(6).
						Background(t.Surface).Border(1, t.Border).Role(ui.RoleButton).Label(locales.ShellCancel()).Focusable()
					cancelBtn.Children(func() {
						ui.Text(c, locales.ShellCancel()).FontSize(theme.UIBadge).SingleLine()
					})
					if cancelBtn.Clicked() {
						s.set.conflict = nil
					}
					return
				}

				if isRec {
					rec := ui.Row(c.Key("recorder")).Height(26).Padding(0, 10).AlignItems(ui.Center).Radius(6).
						Background(t.AccentSoft).Border(1.5, t.Accent).
						Role(ui.RoleButton).Label(locales.SettingsShortcutsRecord()).Focusable().
						HandleInput(func(ev ui.InputEvent) bool {
							return s.handleShortcutRecord(id, ev)
						})
					if s.set.focusRec {
						s.set.focusRec = false
						rec.Focus()
					}
					rec.Children(func() {
						ui.Text(c, locales.SettingsShortcutsRecord()).FontSize(theme.UIBadge).FontWeight(600).TextColor(t.Accent).SingleLine()
					})
				} else {
					curText := locales.SettingsShortcutsUnbound()
					if len(labels) > 0 {
						curText = strings.Join(labels, ", ")
					}
					btnLabel := title + ": " + curText
					pill := ui.Row(c.Key("bind")).MinHeight(26).Padding(2, 8).Gap(4).AlignItems(ui.Center).Radius(6).
						Border(1, t.Border).Background(t.Surface).Role(ui.RoleButton).Label(btnLabel).Focusable()
					if pill.Hovered() {
						pill.Background(t.SurfaceHover)
					}
					pill.Children(func() {
						if len(labels) == 0 {
							ui.Text(c, locales.SettingsShortcutsUnbound()).FontSize(theme.UIBadge).TextColor(t.Faint).SingleLine()
							return
						}
						for i, l := range labels {
							if i > 0 {
								ui.Text(c.Key("sep"+fmt.Sprint(i)), "·").FontSize(theme.UIBadge).TextColor(t.Faint)
							}
							tone := t.Muted
							if custom {
								tone = t.Accent
							}
							ui.Text(c.Key(i), l).FontSize(theme.UIBadge).TextColor(tone).SingleLine()
						}
					})
					if pill.Clicked() {
						s.set.recording = id
						s.set.focusRec = true
						s.set.recWarn = ""
						s.set.conflict = nil
					}
				}

				if custom {
					rst := ui.Row(c.Key("reset")).Height(26).Padding(0, 8).AlignItems(ui.Center).Radius(6).
						Background(t.Surface).Border(1, t.Border).
						Role(ui.RoleButton).Label(locales.SettingsShortcutsReset() + " " + title).Focusable()
					if rst.Hovered() {
						rst.Background(t.SurfaceHover)
					}
					rst.Children(func() {
						ui.Text(c, locales.SettingsShortcutsReset()).FontSize(theme.UIBadge).TextColor(t.Muted).SingleLine()
					})
					if rst.Clicked() {
						s.resetShortcut(id)
					}
				}
			})
		})
	}
}

func (s *Shell) conflictNames(ids []contract.CommandID) string {
	var names []string
	for _, id := range ids {
		if cmd, ok := s.o.Commands.Lookup(id); ok {
			names = append(names, Title(cmd.Title))
		} else {
			names = append(names, string(id))
		}
	}
	return strings.Join(names, ", ")
}

func (s *Shell) handleShortcutRecord(id contract.CommandID, ev ui.InputEvent) bool {
	if ev.Kind != ui.InputKeyDown {
		return false
	}
	if ev.Key == ui.KeyEscape && ev.Mods == 0 {
		s.set.recording = ""
		s.set.recWarn = ""
		s.invalidate()
		return true
	}
	name, ok := keyName(ev.Key)
	if !ok {
		return false
	}
	ch := contract.Chord{Mods: contractMods(ev.Mods), Key: name}
	if ch.Mods&(contract.ModPrimary|contract.ModAlt) == 0 && !isFunctionKey(name) {
		s.set.recWarn = locales.SettingsShortcutsNeedMod()
		s.invalidate()
		return true
	}
	var others []contract.CommandID
	for _, cmd := range s.o.Commands.All() {
		if cmd.ID == id {
			continue
		}
		if slices.Contains(s.o.Commands.KeysFor(cmd.ID), ch) {
			others = append(others, cmd.ID)
		}
	}
	s.set.recording = ""
	s.set.recWarn = ""
	if len(others) > 0 {
		s.set.conflict = &shortcutConflict{cmd: id, chord: ch, others: others}
		s.invalidate()
		return true
	}
	s.applyShortcut(id, ch, nil)
	return true
}

func (s *Shell) applyShortcut(id contract.CommandID, ch contract.Chord, unbind []contract.CommandID) {
	var next []contract.KeyBinding
	s.update(func(v *contract.Settings) {
		drop := map[contract.CommandID]bool{id: true}
		for _, u := range unbind {
			drop[u] = true
		}
		kept := make([]contract.KeyBinding, 0, len(v.Keymap)+1+len(unbind))
		for _, kb := range v.Keymap {
			if drop[kb.Command] {
				if kb.Command != id && kb.Chord != ch && kb.Chord.Key != "" {
					kept = append(kept, kb)
				}
				continue
			}
			kept = append(kept, kb)
		}
		for _, u := range unbind {
			hasRemaining := false
			for _, kb := range kept {
				if kb.Command == u {
					hasRemaining = true
					break
				}
			}
			if !hasRemaining {
				kept = append(kept, contract.KeyBinding{Command: u})
			}
		}
		kept = append(kept, contract.KeyBinding{Command: id, Chord: ch})
		v.Keymap = kept
		next = slices.Clone(kept)
	})
	s.o.Commands.SetUserBindings(next)
	s.invalidate()
}

func (s *Shell) resetShortcut(id contract.CommandID) {
	s.set.recording = ""
	s.set.recWarn = ""
	if s.set.conflict != nil && s.set.conflict.cmd == id {
		s.set.conflict = nil
	}
	var next []contract.KeyBinding
	s.update(func(v *contract.Settings) {
		v.Keymap = slices.DeleteFunc(slices.Clone(v.Keymap), func(kb contract.KeyBinding) bool {
			return kb.Command == id
		})
		next = slices.Clone(v.Keymap)
	})
	s.o.Commands.SetUserBindings(next)
	s.invalidate()
}

func (s *Shell) resetAllShortcuts() {
	s.set.recording = ""
	s.set.recWarn = ""
	s.set.conflict = nil
	s.update(func(v *contract.Settings) {
		v.Keymap = nil
	})
	s.o.Commands.SetUserBindings(nil)
	s.invalidate()
}

// aboutView is the about dialog: the name, the version and what ships inside.
func (s *Shell) aboutView(c *ui.Context, t *theme.Tokens) {
	ui.DialogBase(c.Key("about"), &s.set.about, func(backdrop, panel ui.Element) {
		backdrop.Background(t.Scrim)
		panel.Width(420).Radius(theme.RadiusFloat+4).Border(1, t.Border).Padding(28).Gap(10).
			Shadow(0, 32, 80, 0, t.ShadowColor).Material(effects.Glass(t, effects.Modal)).Label(locales.SettingsAbout())
		ui.Column(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Image(c, kit.Brand(t.Aurora1, t.Aurora2, t.Aurora3)).Size(40, 40).TextColor(t.Text)
			ui.Text(c, locales.AppName()).Font(theme.Serif).FontSize(24).FontWeight(500)
			ui.Text(c, locales.SettingsVersion(s.o.Version)).FontSize(theme.UIStatus).TextColor(t.Muted)
		})
		ui.Text(c, locales.SettingsLicenses()).FontSize(theme.UIText).FontWeight(600).Padding(10, 0, 0, 0)
		for i, l := range s.o.Licenses {
			ui.Text(c.Key(i), l).FontSize(theme.UIStatus).TextColor(t.Muted)
		}
	})
}
