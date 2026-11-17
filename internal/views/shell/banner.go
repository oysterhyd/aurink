package shell

import (
	"aurink/internal/contract"
	"aurink/internal/views/effects"
	"aurink/internal/views/kit"
	"aurink/internal/views/motion"
	"aurink/internal/views/theme"
	"aurink/locales"
	"github.com/egoist/mygo/ui"
)

type bannerKind uint8

const (
	bannerNone bannerKind = iota
	bannerConflict
	bannerDeleted
	bannerEncoding
	bannerReadOnly
	bannerLarge
)

// bannerOf is the banner a document's state calls for, the most urgent first.
func bannerOf(st contract.DocState, encoding string) bannerKind {
	switch {
	case st.Conflict:
		return bannerConflict
	case st.Deleted:
		return bannerDeleted
	case st.ReadOnly && encoding == "gb18030":
		return bannerEncoding
	case st.ReadOnly:
		return bannerReadOnly
	case st.LargeFile:
		return bannerLarge
	}
	return bannerNone
}

// banner is the glass strip over the document for a conflict, a deleted file,
// a read-only file or a large one (02 §3, §5.5.1). It sits above the editor,
// which keeps its scroll offset: the text slides down as the banner collapses
// in and stays on the line it was at (07 FE-2.5).
func (s *Shell) banner(c *ui.Context, t *theme.Tokens, d *doc) {
	kind := bannerOf(d.s.State(), d.s.Model().Meta().Encoding)
	if kind == bannerNone {
		return
	}
	var icon *ui.SVG
	var tone ui.Color
	var title, detail string
	switch kind {
	case bannerConflict:
		icon, tone, title, detail = kit.IconWarn, t.Warning, locales.ShellBannerConflict(), locales.ShellBannerConflictDetail()
	case bannerDeleted:
		icon, tone, title, detail = kit.IconTrash, t.Danger, locales.ShellBannerDeleted(), locales.ShellBannerDeletedDetail()
	case bannerEncoding:
		icon, tone, title, detail = kit.IconLock, t.Warning, locales.ShellBannerEncoding(), locales.ShellBannerEncodingDetail()
	case bannerReadOnly:
		icon, tone, title, detail = kit.IconLock, t.Muted, locales.ShellBannerReadOnly(), locales.ShellBannerReadOnlyDetail()
	default:
		icon, tone, title, detail = kit.IconInfo, t.Accent, locales.ShellBannerLarge(), locales.ShellBannerLargeDetail()
	}
	// The outer row collapses; the strip inside keeps its margins as it does.
	outer := ui.Column(c.Key(kind)).Padding(10, 12, 0, 12).Role(ui.RoleStatus).Label(title)
	if s.live {
		outer.Transition(motion.Still(s.still, ui.ElementTransition{Animation: &motion.Snappy,
			Enter: &ui.Motion{Collapse: true, Opacity: 0}, Exit: &ui.Motion{Collapse: true, Opacity: 0}}))
	}
	outer.Children(func() {
		ui.Row(c).Height(44).Padding(0, 8, 0, 14).Gap(10).AlignItems(ui.Center).Radius(12).Border(1, t.Border).
			Shadow(0, 4, 14, 0, t.ShadowColor).Material(effects.Glass(t, effects.Chip)).Children(func() {
			ui.Icon(c, icon).Size(16, 16).TextColor(tone)
			ui.Text(c, title).FontSize(theme.UIText).FontWeight(600).SingleLine()
			ui.Text(c, detail).FontSize(theme.UIStatus).TextColor(t.Muted).SingleLine().Shrink(1).Grow(1)
			switch kind {
			case bannerConflict:
				if bannerButton(c.Key("keepMine"), t, locales.ShellKeepMine(), true).Clicked() {
					d.s.KeepMine(s.reportErr)
				}
				if bannerButton(c.Key("loadTheirs"), t, locales.ShellLoadTheirs(), false).Clicked() {
					d.s.LoadTheirs(s.reportErr)
				}
			case bannerDeleted:
				if bannerButton(c.Key("saveAgain"), t, locales.ShellSaveAgain(), true).Clicked() {
					s.save(d, nil)
				}
				if bannerButton(c.Key("closeTab"), t, locales.ShellCloseTab(), false).Clicked() {
					s.closeDoc(d)
				}
			case bannerEncoding:
				if bannerButton(c.Key("convert"), t, locales.ShellConvertUtf8(), true).Clicked() {
					d.s.ConvertToUTF8()
					s.focusNext = true
					s.invalidate()
				}
				if bannerButton(c.Key("saveAs"), t, locales.ShellSaveAs(), false).Clicked() {
					s.saveAs(d, s.reportErr)
				}
			case bannerReadOnly:
				if bannerButton(c.Key("saveAs"), t, locales.ShellSaveAs(), false).Clicked() {
					s.saveAs(d, s.reportErr)
				}
			}
		})
	})
}

// reportErr shows an error a session's callback gives, and asks for a frame
// either way, as the state changed.
func (s *Shell) reportErr(err error) {
	if err != nil {
		s.fail(err)
	}
	s.invalidate()
}

func bannerButton(c *ui.Context, t *theme.Tokens, text string, primary bool) ui.Element {
	bg, fg := t.Surface, t.Text
	if primary {
		bg, fg = t.InkButton, t.OnInkButton
	}
	b := ui.Row(c).Height(30).Padding(0, 12).AlignItems(ui.Center).Radius(8).Background(bg).
		Role(ui.RoleButton).Label(text).Focusable()
	if !primary {
		b.Border(1, t.Border)
		if b.Hovered() {
			b.Background(t.SurfaceHover)
		}
	}
	b.Children(func() { ui.Text(c, text).FontSize(theme.UIStatus).FontWeight(600).TextColor(fg).SingleLine() })
	return b
}
