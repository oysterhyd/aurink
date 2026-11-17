package liveedit

import (
	"path/filepath"
	"strings"

	"aurink/internal/contract"
	"aurink/internal/views/effects"
	"aurink/internal/views/theme"
	"aurink/locales"
	"github.com/egoist/mygo/ui"
)

// linkAtCaret returns the link holding the caret, and the source range of its
// destination: what Edit selects for typing over, empty for a reference link,
// whose destination is in its definition.
func (e *Editor) linkAtCaret() (in contract.Inline, dest contract.Range, ok bool) {
	if e.anchor != e.head || len(e.rows) == 0 || e.caretRow < 0 || e.caretRow >= len(e.rows) {
		return in, dest, false
	}
	c := e.head
	for _, x := range e.ast.Inlines(e.rows[e.caretRow].leaf) {
		if x.Kind != contract.InlineLink || x.Auto || c <= x.Range.Start || c >= x.Range.End {
			continue
		}
		// Close is `](dest "title")` for an inline link; a reference link has no
		// destination to edit in place.
		close := e.text.Slice(x.Close)
		if !strings.HasPrefix(close, "](") {
			return x, contract.Range{}, true
		}
		raw := close[2:]
		n := 0
		switch {
		case strings.HasPrefix(raw, "<"):
			if k := strings.IndexByte(raw, '>'); k >= 0 {
				n = k + 1
			}
		default:
			n = strings.IndexAny(raw, " \t\n)")
			if n < 0 {
				n = len(raw)
			}
		}
		start := x.Close.Start + 2
		return x, contract.Range{Start: start, End: start + n}, true
	}
	return in, dest, false
}

// editLink selects the destination of the link at the caret, so that typing
// replaces it. It reports whether there was one.
func (e *Editor) editLink() bool {
	_, dest, ok := e.linkAtCaret()
	if !ok || dest.Empty() {
		return false
	}
	e.model.SetSelection(contract.Selection{Ranges: []contract.SelRange{{Anchor: dest.Start, Head: dest.End}}})
	e.focusReq = true
	e.invalidate()
	return true
}

// openLink follows a link: a web or mail address in the browser, anything else
// as a file next to the document, through Options.OpenFile.
func (e *Editor) openLink(dest string) {
	switch {
	case openable(dest):
		if e.hasView {
			e.services.OpenURL(dest)
		}
	case e.opts.OpenFile != nil && dest != "" && !strings.HasPrefix(dest, "#"):
		path := dest
		if k := strings.IndexAny(path, "#?"); k >= 0 {
			path = path[:k]
		}
		path = strings.ReplaceAll(path, "%20", " ")
		if !filepath.IsAbs(path) && e.opts.DocDir != nil {
			if dir := e.opts.DocDir(); dir != "" {
				path = filepath.Join(dir, filepath.FromSlash(path))
			}
		}
		e.opts.OpenFile(path)
	}
}

// LinkBar shows the link holding the caret under its line, with Edit and Open
// (02 §5 "链接弹层").
func (e *Editor) LinkBar(c *ui.Context) {
	if !e.focused || e.preedit != "" {
		return
	}
	link, dest, ok := e.linkAtCaret()
	if !ok {
		return
	}
	cx, cy, ch := e.caretRect()
	if cy+ch < 0 || cy > e.viewH {
		return
	}
	t := e.t
	left := max(e.contentX(), min(cx-60, e.contentX()+e.width-360))
	bar := ui.Row(c).Absolute().Left(left).Top(cy+ch+8).Height(36).Padding(5, 5, 5, 12).Gap(6).AlignItems(ui.Center).
		Radius(12).Border(1, t.Border).Shadow(0, 8, 24, 0, t.ShadowColor).Material(effects.Glass(t, effects.Panel)).
		Role(ui.RoleToolbar).Label(locales.LiveeditLink())
	bar.Children(func() {
		ui.Text(c, link.Dest).Font(theme.Mono).FontSize(theme.UIBadge).TextColor(t.Muted).SingleLine().MaxWidth(220)
		if !e.readOnly() && !dest.Empty() {
			edit := locales.LiveeditEditLink()
			if e.opts.KeyLabel != nil {
				if k := e.opts.KeyLabel(contract.CmdFormatLink); k != "" {
					edit += "  " + k
				}
			}
			if e.popButton(c.Key("edit"), edit, locales.LiveeditEditLink()) {
				e.editLink()
			}
		}
		if e.popButton(c.Key("open"), locales.LiveeditOpenLink(), locales.LiveeditOpenLink()) {
			e.openLink(link.Dest)
		}
	})
}

func (e *Editor) popButton(c *ui.Context, text, label string) bool {
	t := e.t
	b := ui.Row(c).Height(26).Padding(0, 8).Radius(7).AlignItems(ui.Center).Background(t.Surface).
		Role(ui.RoleButton).Label(label).KeepFocus()
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.Children(func() { ui.Text(c, text).FontSize(theme.UIBadge + 1).TextColor(t.Text).SingleLine() })
	return b.Clicked()
}
