package liveedit

import (
	"fmt"
	"sort"
	"time"

	"aurink/internal/contract"
	"github.com/egoist/mygo/ui"
)

// Font stacks of 02 §4.2. FE-2.1 replaces these with the theme's.
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
	colorFind      = ui.Hex("#E8B931").Alpha(0.35)
	colorFindCur   = ui.Hex("#E8B931").Alpha(0.75)
	colorCodeBg    = ui.RGBA(20, 22, 35, 0.04)
	colorBorder    = ui.RGBA(20, 22, 35, 0.18)
)

const bodySize = 16

// blockFont returns the font of a row by kind, the typography of 02 §4.2.
func blockFont(b *contract.Block) ui.Font {
	switch b.Kind {
	case contract.BlockHeading:
		switch b.Level {
		case 1:
			return ui.Font{Family: fontSerif, Size: bodySize * 2.6, Weight: 500, LineHeight: 1.14}
		case 2:
			return ui.Font{Family: fontSerif, Size: bodySize * 1.62, Weight: 500, LineHeight: 1.3}
		case 3:
			return ui.Font{Family: fontSans, Size: bodySize * 1.12, Weight: 650, LineHeight: 1.5}
		}
		return ui.Font{Family: fontSans, Size: bodySize, Weight: 600, LineHeight: 1.5}
	case contract.BlockCode, contract.BlockHTML, contract.BlockMath, contract.BlockFrontMatter, contract.BlockLinkDef:
		return ui.Font{Family: fontMono, Size: bodySize * 0.85, LineHeight: 1.5}
	}
	return ui.Font{Family: fontSans, Size: bodySize, LineHeight: 1.75}
}

// blockSpace returns the room above and below a row's lines.
func blockSpace(b *contract.Block) (above, below float32) {
	if b.Kind == contract.BlockHeading {
		switch b.Level {
		case 1:
			return 8, 12
		case 2:
			return bodySize * 1.9, bodySize * 0.5
		}
		return bodySize * 1.5, bodySize * 0.4
	}
	if b.Kind == contract.BlockBlank {
		return 0, 0
	}
	return 0, bodySize * 0.85
}

// spansOf styles the segments of a row, with a preedit underlined at display index
// at. A preedit is one more span, in the layout only.
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
	if st&contract.StyleLink != 0 {
		s.Color = colorAccent
	}
	if st&contract.StyleUnderline != 0 {
		s.Underline = true
	}
	if st&contract.StyleMarker != 0 {
		s.Color = colorMuted
		s.Background = ui.Color{}
		s.Weight = 400
		s.Italic = false
	}
	return s
}

func (e *Editor) fillRange(p *ui.Painter, l *laid, b *contract.Block, x, y float32, from, to int, c ui.Color) {
	pr := e.opts.Projector
	a := pr.ToDisplay(&l.proj, clampTo(from, contract.Range{Start: b.Lead.Start, End: b.Trailer.End}), contract.Upstream)
	z := pr.ToDisplay(&l.proj, clampTo(to, contract.Range{Start: b.Lead.Start, End: b.Trailer.End}), contract.Downstream)
	for _, s := range l.layout.SelectionRects(ui.TextInputRange{Start: a, End: z}) {
		p.Fill(ui.Rect{X: x + s.X, Y: y + s.Y, W: s.W, H: s.H}, c, 0)
	}
}

// paint lays out the rows in view and paints them: selection, text, the active
// row's margin label, the caret and the structure rail.
func (e *Editor) paint(p *ui.Painter, r ui.Rect) {
	e.orgX, e.orgY = r.X, r.Y
	e.layoutView(r.W, r.H)
	p.Fill(r, colorSheet, 0)
	selFrom, selTo := min(e.anchor, e.head), max(e.anchor, e.head)
	x := r.X + e.contentX()
	for i := e.first; i <= e.last; i++ {
		l := e.current(i)
		b := e.block(i)
		y := r.Y + e.rowTextY(i)
		e.paintFind(p, l, b, x, y)
		if selFrom != selTo && selFrom < b.Trailer.End && selTo > b.Lead.Start {
			e.fillRange(p, l, b, x, y, selFrom, selTo, colorSelection)
		}
		text := colorText
		if e.focusMode && i != e.caretRow {
			text = text.Alpha(0.35)
		}
		p.TextLayout(l.layout, x, y, text)
		if i == e.caretRow {
			if label := e.marginLabel(b); label != nil {
				p.TextLayout(label, x-46, y+2, colorMuted)
			}
		}
	}
	if e.focused && selFrom == selTo && !e.readOnly() {
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
	if e.moreOverscan && e.hasView {
		e.services.Invalidate()
	}
	if e.OnPainted != nil {
		e.OnPainted()
	}
}

func (e *Editor) paintFind(p *ui.Painter, l *laid, b *contract.Block, x, y float32) {
	start := sort.Search(len(e.find), func(k int) bool { return e.find[k].End > b.Lead.Start })
	for k := start; k < len(e.find) && e.find[k].Start < b.Trailer.End; k++ {
		c := colorFind
		if k == e.findCur {
			c = colorFindCur
		}
		e.fillRange(p, l, b, x, y, e.find[k].Start, e.find[k].End, c)
	}
}

// marginLabel is the active row's page-margin tag (02 §3): a retained layout per
// label, outside the row's own, so it never costs a row reshape.
func (e *Editor) marginLabel(b *contract.Block) *ui.TextLayout {
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
// per heading and the viewport's slider, without shaping any row.
func (e *Editor) paintRail(p *ui.Painter, r ui.Rect) {
	total := e.hs.top(len(e.rows)) + padTop + padBottom
	if total <= float64(r.H) {
		return
	}
	if e.headingsStale {
		e.headingsStale = false
		e.headings = e.headings[:0]
		for _, h := range e.ast.Outline() {
			if i, ok := e.rowOf[h.Block]; ok {
				e.headings = append(e.headings, headingMark{row: i, level: h.Level})
			}
		}
	}
	scale := float64(r.H) / total
	x := r.X + r.W - 8
	for _, h := range e.headings {
		w := float32(4)
		if h.level == 1 {
			w = 7
		}
		y := r.Y + float32((e.hs.top(h.row)+padTop)*scale)
		p.Fill(ui.Rect{X: x + 6 - w, Y: y, W: w, H: 1}, colorBorder, 0)
	}
	thumb := max(float32(float64(r.H)*scale), 24)
	y := r.Y + float32((e.scroll+padTop)*scale)
	p.Fill(ui.Rect{X: x, Y: y, W: 6, H: thumb}, colorText.Alpha(0.25), 3)
}
