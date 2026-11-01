package liveedit

import (
	"fmt"
	"time"

	"aurink.local/spikes/contract-rc/contract"
	"github.com/egoist/mygo/ui"
)

// Font stacks of 02 §4.2. The bundled families fall through to system ones until
// S4 decides them; the layouts do not care which family a span has.
const (
	fontSerif = "Newsreader, Noto Serif SC, Georgia, SimSun"
	fontSans  = "Segoe UI Variable, Microsoft YaHei UI"
	fontMono  = "JetBrains Mono, Cascadia Code, Consolas"
)

var (
	colorText      = ui.Hex("#15161C")
	colorMuted     = ui.Hex("#5F616E")
	colorSheet     = ui.Hex("#FFFFFF")
	colorAccent    = ui.Hex("#00716C")
	colorSelection = colorAccent.Alpha(0.20)
	colorCodeBg    = ui.RGBA(20, 22, 35, 0.04)
	colorBorder    = ui.RGBA(20, 22, 35, 0.18)
)

const bodySize = 16

// blockFont returns the font of a block by kind, the typography of 02 §4.2.
func blockFont(b *block) ui.Font {
	switch {
	case b.Kind == contract.BlockHeading:
		switch b.Level {
		case 1:
			return ui.Font{Family: fontSerif, Size: bodySize * 2.6, Weight: 500, LineHeight: 1.14}
		case 2:
			return ui.Font{Family: fontSerif, Size: bodySize * 1.62, Weight: 500, LineHeight: 1.3}
		case 3:
			return ui.Font{Family: fontSans, Size: bodySize * 1.12, Weight: 650, LineHeight: 1.5}
		}
		return ui.Font{Family: fontSans, Size: bodySize, Weight: 600, LineHeight: 1.5}
	case b.Kind == contract.BlockParagraph:
		return ui.Font{Family: fontSans, Size: bodySize, LineHeight: 1.75}
	}
	return ui.Font{Family: fontMono, Size: bodySize * 0.85, LineHeight: 1.5}
}

// blockSpace returns the room above and below a block's lines.
func blockSpace(b *block) (above, below float32) {
	if b.Kind == contract.BlockHeading {
		switch b.Level {
		case 1:
			return 8, 12
		case 2:
			return bodySize * 1.9, bodySize * 0.5
		}
		return bodySize * 1.5, bodySize * 0.4
	}
	return 0, bodySize * 0.85
}

// spansOf styles the segments of a block, with a preedit underlined at display
// index at. A preedit is one more span, in the layout only.
func spansOf(p *contract.BlockProjection, font ui.Font, preedit string, at int) []ui.Span {
	spans := make([]ui.Span, 0, len(p.Segments)+2)
	add := func(text string, st contract.Style) {
		if text != "" {
			spans = append(spans, styled(text, st, font))
		}
	}
	u := 0
	for i := range p.Segments {
		s := &p.Segments[i]
		if s.Display == "" {
			continue
		}
		n := utf16Len(s.Display)
		if preedit != "" && at >= u && at < u+n {
			cut := utf16ByteOffset(s.Display, at-u)
			add(s.Display[:cut], s.Style)
			spans = append(spans, ui.Span{Text: preedit, Underline: true, Color: colorText})
			preedit = ""
			add(s.Display[cut:], s.Style)
		} else {
			add(s.Display, s.Style)
		}
		u += n
	}
	if preedit != "" {
		spans = append(spans, ui.Span{Text: preedit, Underline: true, Color: colorText})
	}
	return spans
}

func styled(text string, st contract.Style, font ui.Font) ui.Span {
	s := ui.Span{Text: text}
	if st&contract.StyleStrong != 0 {
		s.Weight = 700
	}
	if st&contract.StyleEmphasis != 0 {
		s.Italic = true
	}
	if st&contract.StyleCode != 0 {
		s.Font = fontMono
		s.Size = font.Size * 0.85
		s.Background = colorCodeBg
		s.Radius = 4
	}
	if st&contract.StyleStrike != 0 {
		s.Strikethrough = true
	}
	if st&contract.StyleMarker != 0 {
		s.Color = colorMuted
		s.Background = ui.Color{}
		s.Weight = 400
		s.Italic = false
	}
	return s
}

// paint lays out the blocks in view and paints them: selection, text, the active
// block's margin label, the caret and the structure rail.
func (e *Editor) paint(p *ui.Painter, r ui.Rect) {
	e.layoutView(r.W, r.H)
	p.Fill(r, colorSheet, 0)
	selFrom, selTo := min(e.anchor, e.caret), max(e.anchor, e.caret)
	x := r.X + e.contentX()
	for i := e.first; i <= e.last; i++ {
		l := e.current(i)
		b := &e.d.blocks[i]
		y := r.Y + e.blockTextY(i)
		if selFrom != selTo && selFrom < b.End && selTo > b.Start {
			from := toDisplay(&l.proj, max(selFrom, b.Start), contract.Upstream)
			to := toDisplay(&l.proj, min(selTo, b.End), contract.Downstream)
			for _, s := range l.layout.SelectionRects(ui.TextInputRange{Start: from, End: to}) {
				p.Fill(ui.Rect{X: x + s.X, Y: y + s.Y, W: s.W, H: s.H}, colorSelection, 0)
			}
		}
		p.TextLayout(l.layout, x, y, colorText)
		if i == e.caretBlock {
			if label := e.marginLabel(b); label != nil {
				p.TextLayout(label, x-46, y+2, colorMuted)
			}
		}
	}
	if e.focused && selFrom == selTo {
		const blink = 530 * time.Millisecond
		if phase := p.Now().Sub(e.blinkStart); (phase/blink)%2 == 0 || phase < 0 {
			cx, cy, ch := e.caretRect()
			p.Fill(ui.Rect{X: r.X + cx, Y: r.Y + cy, W: max(1/p.Scale(), 1), H: ch}, colorAccent, 0)
		}
	}
	e.paintRail(p, r)
	e.Stats.Frames++
	e.Stats.MaxFrameShapes = max(e.Stats.MaxFrameShapes, e.Stats.FrameShapes)
	e.Stats.FrameShapes = 0
	if e.moreOverscan {
		e.services.Invalidate()
	}
	if e.OnPainted != nil {
		e.OnPainted()
	}
}

// marginLabel is the active block's page-margin tag (02 §3): a retained layout
// per label, outside the block's own, so it never costs a block reshape.
func (e *Editor) marginLabel(b *block) *ui.TextLayout {
	text := "¶"
	if b.Kind == contract.BlockHeading {
		text = fmt.Sprintf("H%d", b.Level)
	}
	l := e.labels[text]
	if l == nil {
		l = ui.ShapeText(text, ui.Font{Family: fontMono, Size: 11}, 0)
		e.labels[text] = l
	}
	return l
}

// paintRail draws the structure rail of 02 §3 from the Fenwick tree alone: a tick
// per heading and the viewport's slider, without shaping any block.
func (e *Editor) paintRail(p *ui.Painter, r ui.Rect) {
	total := e.hs.top(len(e.views)) + padTop + padBottom
	if total <= float64(r.H) {
		return
	}
	if e.headingsFrom != e.d {
		e.headingsFrom = e.d
		e.headings = e.headings[:0]
		for i := range e.d.blocks {
			if e.d.blocks[i].Kind == contract.BlockHeading {
				e.headings = append(e.headings, i)
			}
		}
	}
	scale := float64(r.H) / total
	x := r.X + r.W - 8
	for _, i := range e.headings {
		w := float32(4)
		if e.d.blocks[i].Level == 1 {
			w = 7
		}
		y := r.Y + float32((e.hs.top(i)+padTop)*scale)
		p.Fill(ui.Rect{X: x + 6 - w, Y: y, W: w, H: 1}, colorBorder, 0)
	}
	thumb := max(float32(float64(r.H)*scale), 24)
	y := r.Y + float32((e.scroll+padTop)*scale)
	p.Fill(ui.Rect{X: x, Y: y, W: 6, H: thumb}, colorText.Alpha(0.25), 3)
}
