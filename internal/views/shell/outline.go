package shell

import (
	"strings"

	"aurink/internal/contract"
	"aurink/internal/views/editorapi"
	"aurink/internal/views/theme"
	"aurink/locales"
	"github.com/egoist/mygo/ui"
)

type heading struct {
	id    contract.BlockID
	level int
	title string
	off   int
}

// outline caches the active document's headings by its version: typing in a
// paragraph rereads them once, not every frame.
type outline struct {
	doc      *doc
	version  contract.Version
	headings []heading
	rows     ui.ListState
}

func (s *Shell) headings(d *doc) []heading {
	o := &s.outline
	m := d.s.Model()
	text := m.Text()
	if o.doc == d && o.version == text.Version() {
		return o.headings
	}
	ast := m.AST()
	o.doc, o.version, o.headings = d, text.Version(), o.headings[:0]
	for i := range ast.Len() {
		b := ast.Block(i)
		if b.Kind != contract.BlockHeading {
			continue
		}
		title := strings.TrimSpace(text.Slice(b.Content))
		if title == "" {
			continue
		}
		o.headings = append(o.headings, heading{id: b.ID, level: b.Level, title: title, off: b.Range.Start})
	}
	return o.headings
}

func (s *Shell) outlinePanel(c *ui.Context, t *theme.Tokens) {
	d := s.active
	var hs []heading
	if d != nil {
		hs = s.headings(d)
	}
	if len(hs) == 0 {
		ui.Text(c, locales.ShellNoHeadings()).FontSize(theme.UIText).TextColor(t.Muted).Padding(12, 16)
		return
	}
	// The heading of the section at the top of the view (scroll spy).
	var current contract.BlockID
	if d.mode == editorapi.ModeLive {
		current, _ = d.live.VisibleHeading()
	}
	minLevel := 6
	for _, h := range hs {
		minLevel = min(minLevel, h.level)
	}
	ui.List(c.Key("outline"), &s.outline.rows, len(hs), func(i int) {
		h := hs[i]
		row := ui.Row(c.Key(h.id)).Height(28).Padding(0, 8).PaddingStart(8 + float32(h.level-minLevel)*14).
			AlignItems(ui.Center).Radius(theme.RadiusRow).Role(ui.RoleButton).Label(h.title)
		on := h.id == current
		if on {
			row.Background(t.AccentSoft)
		} else if row.Hovered() {
			row.Background(t.Hover)
		}
		row.Children(func() {
			text := ui.Text(c, h.title).FontSize(theme.UIText).SingleLine()
			if on {
				text.TextColor(t.Accent).FontWeight(600)
			} else if h.level > minLevel {
				text.TextColor(t.Muted)
			}
		})
		if row.Clicked() {
			s.goToOffset(d, h.off)
		}
	}).Grow(1).MinHeight(0).Padding(0, 8, 8)
}

// searchPanel searches the open folder; it arrives with the search card.
func (s *Shell) searchPanel(c *ui.Context, t *theme.Tokens) {
	ui.Text(c, locales.ShellSearchSoon()).FontSize(theme.UIText).TextColor(t.Muted).Padding(12, 16)
}
