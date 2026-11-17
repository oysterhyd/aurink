package liveedit

import (
	"strconv"
	"unicode/utf8"

	"aurink/internal/contract"
	"aurink/internal/views/effects"
	"aurink/internal/views/kit"
	"aurink/internal/views/motion"
	"aurink/internal/views/theme"
	"aurink/locales"
	"github.com/egoist/mygo/ui"
)

// formatBar is the state of the selection's format bar (02 §5.11): whether its
// link button has turned it into an address field, for which selection.
type formatBar struct {
	linking  bool
	url      string
	from, to int
	focusURL bool
}

const (
	formatBarW = 318
	formatBarH = 40
)

// formatBarShown reports whether the bar is up: over a selection in the live
// view (the shell builds it there only), with the pointer up, not while
// composing, or while its field is open.
func (e *Editor) formatBarShown() bool {
	if e.fmtBar.linking {
		return true
	}
	return e.focused && !e.readOnly() && e.preedit == "" && e.anchor != e.head && !e.dragging
}

// FormatBar shows the glass bar over the selection (02 §5.11): Transform,
// bold, italic, underline, strike, code, link and highlight, the selection's
// formats lit. Link turns the bar into an address field in place.
func (e *Editor) FormatBar(c *ui.Context) {
	fb := &e.fmtBar
	// Anything that moves the selection closes the address field.
	if fb.linking && (e.anchor != fb.from || e.head != fb.to || e.readOnly()) {
		fb.linking = false
		e.focusReq = !e.readOnly()
		e.invalidate()
	}
	if !e.formatBarShown() {
		return
	}
	r, ok := e.SelectionRect()
	if !ok {
		return
	}
	x, y := r.X-e.orgX, r.Y-e.orgY
	top := y - formatBarH - 8
	if top < 6 {
		top = y + r.H + 8
	}
	// The bar fits its buttons; its width, for centring, is what they take.
	w := float32(formatBarW)
	if !fb.linking && e.opts.Exts&contract.ExtHighlight == 0 {
		w -= 32
	}
	left := max(8, min(x+r.W/2-w/2, e.viewW-w-8))
	t := e.t
	bar := ui.Row(c).Absolute().Left(left).Top(top).Height(formatBarH).Padding(0, 6).Gap(2).
		AlignItems(ui.Center).Radius(12).Border(1, t.Border).Shadow(0, 4, 14, 0, t.ShadowColor).
		Material(effects.Glass(t, effects.Chip)).Role(ui.RoleToolbar).Label(locales.LiveeditFormatBar()).
		Cursor(ui.CursorDefault).KeepFocus()
	if !e.reduceMotion {
		bar.Transition(ui.ElementTransition{Animation: &motion.Snappy,
			Enter: &ui.Motion{Y: 6, Scale: .96, Opacity: 0}, Exit: &ui.Motion{Y: 6, Scale: .96, Opacity: 0}})
	}
	bar.Children(func() {
		if fb.linking {
			e.linkField(c)
			return
		}
		f := e.Formatting()
		e.transform(c, f)
		ui.Box(c).Size(1, 18).Margin(0, 4).Background(t.Border)
		e.fmtButton(c.Key("b"), func(c *ui.Context) ui.Element { return ui.Text(c, "B").FontWeight(700) },
			locales.LiveeditBold(), contract.CmdFormatStrong, f.Strong)
		e.fmtButton(c.Key("i"), func(c *ui.Context) ui.Element { return ui.Text(c, "I").Italic().Font(theme.Serif) },
			locales.LiveeditItalic(), contract.CmdFormatEmphasis, f.Emphasis)
		e.fmtButton(c.Key("u"), func(c *ui.Context) ui.Element { return ui.Text(c, "U").Underline() },
			locales.LiveeditUnderline(), contract.CmdFormatUnderline, contract.Off)
		e.fmtButton(c.Key("s"), func(c *ui.Context) ui.Element { return ui.Text(c, "S").Strikethrough() },
			locales.LiveeditStrike(), contract.CmdFormatStrike, f.Strike)
		e.fmtButton(c.Key("code"), func(c *ui.Context) ui.Element { return ui.Text(c, "<>").Font(theme.Mono) },
			locales.LiveeditInlineCode(), contract.CmdFormatCode, f.Code)
		link := e.fmtButton(c.Key("link"), func(c *ui.Context) ui.Element { return ui.Icon(c, kit.IconLink).Size(15, 15) },
			locales.LiveeditLink(), "", f.Link)
		if link {
			fb.linking, fb.url, fb.focusURL = true, f.LinkDest, true
			fb.from, fb.to = e.anchor, e.head
		}
		// Highlight is there while its syntax is on.
		if e.opts.Exts&contract.ExtHighlight != 0 {
			e.fmtButton(c.Key("h"), func(c *ui.Context) ui.Element { return ui.Text(c, "H").FontWeight(600) },
				locales.LiveeditHighlight(), contract.CmdFormatHighlight, f.Highlight)
		}
	})
}

// fmtButton is one of the bar's buttons, lit while the selection has its
// format; it runs id, or reports the click when id is "".
func (e *Editor) fmtButton(c *ui.Context, glyph func(*ui.Context) ui.Element, label string, id contract.CommandID, state contract.TriState) bool {
	t := e.t
	b := ui.Row(c).Size(30, 28).Radius(7).Justify(ui.Center).AlignItems(ui.Center).
		Role(ui.RoleToggleButton).Checked(state == contract.On).Label(label).Tooltip(label).KeepFocus()
	tone := t.Text
	switch {
	case state == contract.On:
		b.Background(t.AccentSoft)
		tone = t.Accent
	case b.Hovered():
		b.Background(t.Hover)
	}
	b.Children(func() { glyph(c).FontSize(theme.UIText).TextColor(tone) })
	if !b.Clicked() {
		return false
	}
	if id != "" {
		_ = e.RunEdit(contract.EditCommand{ID: id})
		e.focusReq = true
		e.invalidate()
	}
	return true
}

// transform is the bar's Transform menu: body text, heading 1 and 2, quote,
// through the block commands; the button names the block the selection is in.
func (e *Editor) transform(c *ui.Context, f contract.FormatState) {
	t := e.t
	quote := e.inQuote()
	label := locales.LiveeditBody()
	switch {
	case f.Block == contract.BlockHeading:
		label = locales.LiveeditHeadingN(strconv.Itoa(f.Level))
	case quote:
		label = locales.LiveeditQuote()
	}
	b := ui.Row(c.Key("transform")).Height(28).Padding(0, 6, 0, 8).Gap(3).Radius(7).AlignItems(ui.Center).
		Role(ui.RoleMenuButton).Label(locales.LiveeditTransform()).Value(label).KeepFocus()
	if b.Hovered() {
		b.Background(t.Hover)
	}
	b.Children(func() {
		ui.Text(c, label).FontSize(theme.UIText).TextColor(t.Text).SingleLine()
		ui.Icon(c, kit.IconChevron).Size(11, 11).TextColor(t.Muted)
	})
	run := func(cmd contract.EditCommand) {
		_ = e.RunEdit(cmd)
		e.focusReq = true
		e.invalidate()
	}
	b.Menu(func(m *ui.Menu) {
		para := f.Block == contract.BlockParagraph && !quote
		if m.Item(locales.LiveeditBody()).Checked(para).Chosen() {
			if quote {
				run(contract.EditCommand{ID: contract.CmdBlockQuote})
			}
			run(contract.EditCommand{ID: contract.CmdBlockParagraph})
		}
		for _, h := range []struct {
			level int
			id    contract.CommandID
		}{{1, contract.CmdBlockHeading1}, {2, contract.CmdBlockHeading2}} {
			on := f.Block == contract.BlockHeading && f.Level == h.level
			if m.Item(locales.LiveeditHeadingN(strconv.Itoa(h.level))).Checked(on).Chosen() {
				run(contract.EditCommand{ID: h.id})
			}
		}
		if m.Item(locales.LiveeditQuote()).Checked(quote).Chosen() {
			run(contract.EditCommand{ID: contract.CmdBlockQuote})
		}
	})
}

// inQuote reports whether the caret's block is in a block quote.
func (e *Editor) inQuote() bool {
	if e.caretRow < 0 || e.caretRow >= len(e.rows) {
		return false
	}
	for i := e.ast.Block(e.rows[e.caretRow].leaf).Parent; i >= 0; i = e.ast.Block(i).Parent {
		if e.ast.Block(i).Kind == contract.BlockQuote {
			return true
		}
	}
	return false
}

// linkField is the bar turned into an address field: Enter links the
// selection to it, Escape goes back to the document.
func (e *Editor) linkField(c *ui.Context) {
	fb := &e.fmtBar
	t := e.t
	ui.Box(c).Size(30, 28).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, kit.IconLink).Size(15, 15).TextColor(t.Muted)
	})
	in := ui.TextInputBase(c.Key("url"), &fb.url).Grow(1).Shrink(1).Height(28).Padding(0, 8).Radius(7).
		Background(t.Surface).FontSize(theme.UIText).Placeholder(locales.LiveeditLinkAddress()).
		Label(locales.LiveeditLinkAddress())
	if fb.focusURL {
		fb.focusURL = false
		in.Focus().SetTextSelection(0, utf8.RuneCountInString(fb.url))
	} else if !in.Focused() {
		// The focus went elsewhere: the field closes, as a popover does.
		fb.linking = false
		e.invalidate()
		return
	}
	done := func() {
		fb.linking = false
		e.focusReq = true
		e.invalidate()
	}
	in.OnTextKey(func(k ui.TextKey) bool {
		if k.Key != ui.KeyEscape || k.Mods != 0 {
			return false
		}
		done()
		return true
	})
	if in.Submitted() {
		if fb.url != "" || e.Formatting().Link != contract.Off {
			_ = e.RunEdit(contract.EditCommand{ID: contract.CmdFormatLink, URL: fb.url})
		}
		done()
	}
}
