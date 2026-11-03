package shell

import (
	"aurink/internal/views/editorapi"
	"aurink/internal/views/kit"
	"aurink/internal/views/motion"
	"aurink/internal/views/theme"
	"aurink/locales"
	"github.com/egoist/mygo/ui"
)

// Reading mode, the zoom and the document's type (FE-1.7, 02 §5.10, D17).

const (
	// zoomStep is one step of view.zoomIn; the zoom goes from 50% to 200%.
	zoomStep         = 0.1
	zoomMin, zoomMax = -5, 10
	// The reading face (02 §3.2): body one larger, line height at least 1.9,
	// column at most 680.
	readLineHeight = 1.9
	readColumn     = 680
	// focusBar is the title bar's opacity in focus mode; fadeHeight the height
	// of the sheet's top and bottom fades there (02 §5.10).
	focusBar   = 0.55
	fadeHeight = 48
	// readTop is the height of reading mode's strip: the title and the exit
	// button, over the top margin of the document.
	readTop   = 40
	titleFade = 16
)

// Zoom returns the document's zoom, 1 for actual size.
func (s *Shell) Zoom() float32 { return 1 + zoomStep*float32(s.zoom) }

// Reading reports whether the window is in reading mode.
func (s *Shell) Reading() bool { return s.reading }

// zoomBy zooms in (1) or out (-1) by a step, or back to actual size (0).
func (s *Shell) zoomBy(dir int) {
	if dir == 0 {
		s.zoom = 0
	} else {
		s.zoom = max(zoomMin, min(s.zoom+dir, zoomMax))
	}
	s.invalidate()
}

// typography is the document's type: Settings.Appearance, the zoom, and the
// reading face in reading mode.
func (s *Shell) typography() theme.Typography {
	a := s.o.Settings.Get().Appearance
	ty := theme.DefaultTypography()
	if a.FontSize > 0 {
		ty.Body = a.FontSize
	}
	if a.LineHeight > 0 {
		ty.LineHeight = a.LineHeight
	}
	if a.ContentWidth > 0 {
		ty.Column = a.ContentWidth
	}
	ty.Serif = a.ContentFont == theme.FontSerif
	if s.reading {
		ty.Serif = true
		ty.Body++
		ty.LineHeight = max(ty.LineHeight, readLineHeight)
		ty.Column = min(ty.Column, readColumn)
	}
	return ty.Zoomed(s.Zoom())
}

// setReading enters or leaves reading mode. It is the window's state, not the
// file's: the session stays writable, and leaving gives each tab back the view
// it had (D17).
func (s *Shell) setReading(on bool) {
	if on == s.reading || on && s.active == nil {
		return
	}
	s.reading = on
	if on {
		s.readDoc(s.active)
	} else {
		for _, d := range s.docs {
			s.leaveReading(d)
		}
	}
	s.focusNext = true
	s.invalidate()
}

// readDoc shows a tab in reading mode; a tab activated in reading mode reads too.
func (s *Shell) readDoc(d *doc) {
	if d.live.Mode() == editorapi.ModeRead {
		return
	}
	d.readFrom = d.mode
	if d.mode == editorapi.ModeSource {
		s.switchMode(d, editorapi.ModeLive)
	}
	d.live.SetMode(editorapi.ModeRead)
}

func (s *Shell) leaveReading(d *doc) {
	if d.live.Mode() != editorapi.ModeRead {
		return
	}
	d.live.SetMode(editorapi.ModeLive)
	if d.readFrom == editorapi.ModeSource {
		s.switchMode(d, editorapi.ModeSource)
	}
}

// focused reports whether the active tab is in focus mode.
func (s *Shell) focused() bool {
	return s.active != nil && s.active.mode == editorapi.ModeLive && !s.reading && s.active.live.FocusMode()
}

// readingTop draws reading mode's strip over the sheet: the progress bar, the
// title once the first heading has scrolled away, and the exit button. The
// strip moves the window, as the hidden title bar did.
func (s *Shell) readingTop(c *ui.Context, t *theme.Tokens, d *doc, shown float32) {
	title, past := d.live.TitleAbove()
	if title == "" {
		title = d.s.State().Name
	}
	// The title sits on the paper's color, fading out below it, so the text
	// scrolling under it does not run through it.
	var want float32
	if past {
		want = shown
	}
	tb := ui.Column(c.Key("readTitle")).Absolute().Top(0).Left(0).Right(0).PassThrough()
	o := tb.AnimateTo("title", want, motion.Smooth)
	if o > 0.01 {
		tb.Opacity(o).Children(func() {
			ui.Row(c).Height(readTop).Background(t.Sheet).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
				ui.Text(c, title).FontSize(theme.UIText).TextColor(t.Muted).MaxLines(1)
			})
			ui.Box(c).Height(titleFade).Gradient(t.Sheet, t.Sheet.Alpha(0), 180)
		})
	}
	ui.Row(c.Key("readTop")).Absolute().Top(0).Left(0).Right(0).Height(readTop).Opacity(shown).
		Padding(0, 12).AlignItems(ui.Center).DragWindow().Children(func() {
		ui.Spacer(c)
		ui.Spacer(c)
		if kit.Button(c.Key("exitReading"), t, locales.ShellExitReading(), "", false).Clicked() {
			s.setReading(false)
		}
	})
	// The progress bar: the read part grows from the left.
	f := d.live.ScrollFraction()
	ui.Row(c.Key("readProgress")).Absolute().Top(0).Left(0).Right(0).Height(2).Opacity(shown).PassThrough().Children(func() {
		if f > 0 {
			ui.Box(c).Grow(f).Height(2).Background(t.Accent).Shadow(0, 0, 6, 0, t.Accent.Alpha(0.6))
		}
		if f < 1 {
			ui.Box(c).Grow(1 - f)
		}
	})
}

// focusFades draws focus mode's top and bottom fades over the sheet.
func (s *Shell) focusFades(c *ui.Context, t *theme.Tokens, shown float32) {
	clear := t.Sheet.Alpha(0)
	ui.Box(c.Key("fadeTop")).Absolute().Top(0).Left(0).Right(0).Height(fadeHeight).Opacity(shown).PassThrough().
		Gradient(t.Sheet, clear, 180)
	ui.Box(c.Key("fadeBottom")).Absolute().Bottom(0).Left(0).Right(0).Height(fadeHeight).Opacity(shown).PassThrough().
		Gradient(clear, t.Sheet, 180)
}

// escapeReading leaves reading mode on Escape.
func (s *Shell) escapeReading(c *ui.Context) {
	if s.reading {
		c.OnShortcut(0, ui.KeyEscape, func() { s.setReading(false) })
	}
}

// reduceMotion reports whether motion is reduced: Appearance.ReduceMotion
// "on" or "off" decides, "system" follows the system (D18).
func (s *Shell) reduceMotion(c *ui.Context) bool {
	switch s.o.Settings.Get().Appearance.ReduceMotion {
	case "on":
		return true
	case "off":
		return false
	}
	return c.Preferences().ReduceMotion
}
