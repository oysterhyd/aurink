package liveedit

import (
	"slices"
	"strings"
	"unicode/utf8"

	"aurink/internal/contract"
	"aurink/internal/views/theme"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

const tabColumns = 4

// rowMetrics is a row's typography and geometry, fixed when it is shaped.
type rowMetrics struct {
	font          ui.Font
	above, below  float32 // room above and below the lines, inside the row
	x             float32 // the text's left, from the column's left
	width         float32 // the width the text wraps at
	muted, struck bool
	// preview is the room at the bottom of the row for the live preview of
	// a math block or diagram whose source shows; it is part of below.
	preview float32
}

// widget is one entry of the block registry (03 §6.5): new syntax adds an entry and
// does not touch the engine. A widget sizes a row and paints around its text.
type widget interface {
	metrics(e *Editor, r int, b *contract.Block, m *rowMetrics)
	back(rc *rowContext)
	front(rc *rowContext)
}

// rowContext is what a widget paints with.
type rowContext struct {
	e      *Editor
	p      *ui.Painter
	t      *theme.Tokens
	row    int
	b      *contract.Block
	l      *laid
	ox     float32 // the column's left in the window
	top    float32 // the row's top in the window
	height float32
}

func (rc *rowContext) textY() float32 { return rc.top + rc.l.m.above }

func (rc *rowContext) textX() float32 { return rc.ox + rc.l.m.x }

type plainWidget struct{}

func (plainWidget) metrics(*Editor, int, *contract.Block, *rowMetrics) {}
func (plainWidget) back(*rowContext)                                   {}
func (plainWidget) front(*rowContext)                                  {}

var widgets = map[contract.BlockKind]widget{
	contract.BlockHeading:       headingWidget{},
	contract.BlockCode:          codeWidget{},
	contract.BlockThematicBreak: breakWidget{},
	contract.BlockHTML:          sourceWidget{},
	contract.BlockFrontMatter:   sourceWidget{},
	contract.BlockLinkDef:       sourceWidget{},
	contract.BlockMath:          sourceWidget{},
	contract.BlockTOC:           sourceWidget{},
	contract.BlockTableCell:     cellWidget{},
	contract.BlockBlank:         blankWidget{},
}

// widgetOf picks a row's widget by its projection's kind, which differs from the
// block's while a lone marker waits for its space (a "#" is a paragraph).
func widgetOf(b *contract.Block, p *contract.BlockProjection) widget {
	// A block shown as its object (a diagram, a header) is not its source's
	// card: the object paints itself.
	if o := blockObject(p); o != nil && o.Kind != contract.ObjImage {
		return plainWidget{}
	}
	kind := b.Kind
	if p.Block == b.ID {
		kind = p.Kind
	}
	if w, ok := widgets[kind]; ok {
		return w
	}
	return plainWidget{}
}

const (
	listIndent  = 26
	quoteIndent = 22 // the bar and the gap after it
	quoteBar    = 4
	codePadX    = 14
	codeGutter  = 30
	codeHeader  = 30
	cellPad     = 10
)

// metricsFor sizes a row: its containers first, then the block's own widget.
func (e *Editor) metricsFor(r int, b *contract.Block, p *contract.BlockProjection, lead bool) rowMetrics {
	ty := e.ty
	m := rowMetrics{font: ty.BodyFont(), below: ty.Body * .85, width: e.width}
	if lead {
		m.font = ty.LeadFont()
	}
	for i := range p.Containers {
		switch c := &p.Containers[i]; c.Kind {
		case contract.BlockQuote:
			// Typora's look (user's ruling 2026-11-02, replacing 02 §4.2's
			// serif italic): body text, muted, beside a bar.
			m.x += quoteIndent
			m.muted = true
		case contract.BlockItem:
			m.x += listIndent
			m.below = ty.Body * .4
			if c.Task == contract.TaskDone {
				m.struck = true
			}
		case contract.BlockFootnoteDef:
			m.muted = true
		}
	}
	// A block shown as its object (a header, a table of contents) is not
	// source: it takes the plain row.
	if o := blockObject(p); o == nil || o.Kind == contract.ObjImage {
		widgetOf(b, p).metrics(e, r, b, &m)
	} else {
		m.above, m.below = ty.Body*.25, ty.Body*.6
	}
	m.width = max(m.width-m.x, 1)
	if h := e.previewHeight(b, e.previewObject(b, p)); h > 0 {
		m.preview = h
		m.below += h
	}
	return m
}

type headingWidget struct{ plainWidget }

func (headingWidget) metrics(e *Editor, _ int, b *contract.Block, m *rowMetrics) {
	m.font = e.ty.HeadingFont(b.Level)
	switch body := e.ty.Body; b.Level {
	case 1:
		m.above, m.below = body*.5, body*.75
	case 2:
		m.above, m.below = body*1.9, body*.5
	default:
		m.above, m.below = body*1.5, body*.4
	}
	m.muted = b.Level == 6
}

type blankWidget struct{ plainWidget }

func (blankWidget) metrics(_ *Editor, _ int, _ *contract.Block, m *rowMetrics) { m.below = 0 }

// sourceWidget shows syntax M1 does not render as pale source (06 §2.1).
type sourceWidget struct{ plainWidget }

func (sourceWidget) metrics(e *Editor, _ int, _ *contract.Block, m *rowMetrics) {
	m.font = e.ty.CodeFont()
	m.muted = true
}

// breakWidget is the section break "···"; its source shows with the caret on it.
type breakWidget struct{ plainWidget }

func (breakWidget) metrics(e *Editor, _ int, _ *contract.Block, m *rowMetrics) {
	m.above, m.below = e.ty.Body, e.ty.Body*1.5
}

func (breakWidget) front(rc *rowContext) {
	if rc.l.display != "" {
		return
	}
	cx := rc.ox + rc.e.width/2
	cy := rc.textY() + rc.l.h/2
	for k := float32(-1); k <= 1; k++ {
		rc.p.Fill(ui.Rect{X: cx + k*14 - 1.5, Y: cy - 1.5, W: 3, H: 3}, rc.t.Muted, 2)
	}
}

// codeWidget is a code block: a tinted card with its language, a copy button and
// line numbers. Highlighting goes through the spans (spansOf).
type codeWidget struct{ plainWidget }

func (codeWidget) metrics(e *Editor, _ int, _ *contract.Block, m *rowMetrics) {
	m.font = e.ty.CodeFont()
	m.above, m.below = codeHeader, e.ty.Body*1.4
	m.x = codePadX + codeGutter
	m.width -= codePadX
}

func (codeWidget) back(rc *rowContext) {
	h := rc.height - rc.l.m.preview - rc.e.ty.Body + 12
	rc.p.Fill(ui.Rect{X: rc.ox, Y: rc.top, W: rc.e.width, H: h}, rc.t.CodeBg, 10)
	rc.p.Stroke(ui.Rect{X: rc.ox, Y: rc.top, W: rc.e.width, H: h}, rc.t.Border, 10, 1)
}

func (codeWidget) front(rc *rowContext) {
	e, p, t := rc.e, rc.p, rc.t
	if rc.b.Lang != "" {
		p.TextLayout(e.label(rc.b.Lang, theme.UIBadge), rc.ox+codePadX, rc.top+9, t.Muted)
	}
	text, w := e.label(e.copyLabel(rc.b), theme.UIBadge), float32(0)
	w, _ = text.Size()
	p.TextLayout(text, rc.ox+e.width-codePadX-w, rc.top+9, t.Muted)
	// Line numbers, one per line of source, whatever wraps.
	line, at := 1, 0
	for {
		c := rc.l.layout.CaretAt(ui.TextCaretPosition{Index: utf16Len(rc.l.display[:at])})
		n := e.label(itoa(line), theme.UIBadge)
		nw, _ := n.Size()
		p.TextLayout(n, rc.textX()-10-nw, rc.textY()+c.Y+3, t.Faint)
		k := strings.IndexByte(rc.l.display[at:], '\n')
		if k < 0 {
			return
		}
		at += k + 1
		line++
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// cellWidget places a cell in its table row's grid (06 §2.1). Cells of a row share
// a band: the row's first cell holds its height and the others hold none.
type cellWidget struct{ plainWidget }

func (cellWidget) metrics(e *Editor, r int, _ *contract.Block, m *rowMetrics) {
	v := &e.rows[r]
	col := e.width / float32(max(v.cols, 1))
	m.x = float32(v.col)*col + cellPad
	m.width = float32(v.col+1)*col - cellPad
	m.above, m.below = 7, 7
	top, bottom := e.tableGaps(r)
	m.above += top
	m.below += bottom
	if v.header {
		m.font.Weight = 600
	}
}

// tableGap sets a table off from the blocks around it, outside its container.
const tableGap = 10

// tableGaps returns the space above and below row r's band that lies outside the
// table: above its first row, below its last.
func (e *Editor) tableGaps(r int) (top, bottom float32) {
	first, last, ok := e.tableRows(r)
	if !ok {
		return 0, 0
	}
	band := r - e.rows[r].col
	if band == first {
		top = tableGap
	}
	if band == last {
		bottom = tableGap
	}
	return top, bottom
}

// tableRadius rounds a table's container (02 §5.8).
const tableRadius = 8

// back fills the header band, rounded at the top as the container is.
func (cellWidget) back(rc *rowContext) {
	v := &rc.e.rows[rc.row]
	if !v.header || v.col != 0 {
		return
	}
	top, bottom := rc.e.tableGaps(rc.row)
	band := ui.Rect{X: rc.ox, Y: rc.top + top, W: rc.e.width, H: rc.height - top - bottom}
	rc.p.Fill(band, rc.t.Surface, tableRadius)
	if band.H > tableRadius {
		rc.p.Fill(ui.Rect{X: band.X, Y: band.Y + tableRadius, W: band.W, H: band.H - tableRadius}, rc.t.Surface, 0)
	}
}

// front draws the grid: a line left of each cell but the first, one under each
// row but the last, the rounded container once per table and frame, and the
// ring of the cell holding the caret.
func (cellWidget) front(rc *rowContext) {
	e, p, t := rc.e, rc.p, rc.t
	v := &e.rows[rc.row]
	cell := rc.cell()
	if v.col > 0 {
		p.Fill(ui.Rect{X: cell.X, Y: cell.Y, W: 1, H: cell.H}, t.Border, 0)
	} else if first, last, ok := e.tableRows(rc.row); ok {
		if rc.row != last {
			p.Fill(ui.Rect{X: rc.ox, Y: cell.Y + cell.H - 1, W: e.width, H: 1}, t.Border, 0)
		}
		if rc.row == first || rc.row == e.first {
			top := rc.top - float32(e.rowTop(rc.row)-e.rowTop(first)) + tableGap
			h := float32(e.rowTop(last)-e.rowTop(first)) + e.rows[last].h - 2*tableGap
			p.Stroke(ui.Rect{X: rc.ox, Y: top, W: e.width, H: h}, t.Border, tableRadius, 1)
		}
	}
	if rc.row == e.caretRow && e.focused && !e.readOnly() {
		p.Stroke(ui.Rect{X: cell.X + 1.5, Y: cell.Y + 1.5, W: cell.W - 3, H: cell.H - 3}, t.AccentRing, 5, 2)
	}
}

// tableRows returns the first and last rows of the table holding row r, each
// the first cell of its band.
func (e *Editor) tableRows(r int) (first, last int, ok bool) {
	b := e.block(r)
	if b.Kind != contract.BlockTableCell || b.Parent < 0 {
		return 0, 0, false
	}
	tr := e.ast.Block(b.Parent)
	if tr == nil || tr.Parent < 0 {
		return 0, 0, false
	}
	tbl := e.ast.Block(tr.Parent)
	first, last = e.rowAt(tbl.Range.Start), e.rowAt(max(tbl.Range.Start, tbl.Range.End-1))
	first -= e.rows[first].col
	last -= e.rows[last].col
	return first, last, first <= r && r <= last+e.rows[last].cols
}

func (rc *rowContext) cell() ui.Rect {
	e, v := rc.e, &rc.e.rows[rc.row]
	col := e.width / float32(max(v.cols, 1))
	top, bottom := e.tableGaps(rc.row)
	return ui.Rect{X: rc.ox + float32(v.col)*col, Y: rc.top + top, W: col, H: e.rows[rc.row-v.col].h - top - bottom}
}

// paintContainers draws what a row's containers show: list markers, checkboxes,
// quote bars and footnote labels.
func (rc *rowContext) paintContainers() {
	e, p, t := rc.e, rc.p, rc.t
	x := rc.ox
	lineH := rc.l.m.font.Size * rc.l.m.font.LineHeight
	y := rc.textY()
	for i := range rc.l.proj.Containers {
		c := &rc.l.proj.Containers[i]
		switch c.Kind {
		case contract.BlockQuote:
			rc.paintQuoteBar(c, x)
			x += quoteIndent
		case contract.BlockItem:
			if c.First {
				switch {
				case c.Task != contract.TaskNone:
					e.paintCheckbox(p, t, c, x+1, y+(lineH-18)/2)
				case c.Ordered:
					n := e.label(itoa(c.Number)+string(rune(c.Marker)), theme.UIText+1)
					w, _ := n.Size()
					p.TextLayout(n, x+listIndent-8-w, y+(lineH-theme.UIText*1.6)/2, t.Muted)
				default:
					p.Fill(ui.Rect{X: x + 8, Y: y + lineH/2 - 2.5, W: 5, H: 5}, t.Muted, 3)
				}
			}
			x += listIndent
		case contract.BlockFootnoteDef:
			if c.First && c.Label != "" {
				n := e.label("[^"+c.Label+"]:", theme.UIStatus)
				w, _ := n.Size()
				p.TextLayout(n, x-w-6, y+4, t.Muted)
			}
		}
	}
}

// paintQuoteBar draws a quote's bar along the row at x. The bars of a quote's
// rows meet, so it runs unbroken from its first line to its last.
func (rc *rowContext) paintQuoteBar(c *contract.ContainerDecor, x float32) {
	top, bottom := rc.top, rc.top+rc.height
	if c.First {
		top = rc.textY()
	}
	if rc.e.lastInQuote(rc.row, c.Block) {
		bottom = rc.textY() + rc.l.h
	}
	rc.p.Fill(ui.Rect{X: x, Y: top, W: quoteBar, H: bottom - top}, rc.t.Faint.Alpha(.35), 0)
}

// lastInQuote reports whether row r is the quote's last: the next row starts
// past the quote's end.
func (e *Editor) lastInQuote(r int, quote contract.BlockID) bool {
	b := e.block(r)
	for b != nil && b.ID != quote {
		if b.Parent < 0 {
			return true
		}
		b = e.ast.Block(b.Parent)
	}
	return b == nil || r+1 >= len(e.rows) || e.block(r+1).Range.Start >= b.Range.End
}

// taskBoxAt returns the task container whose checkbox holds a point of the element.
func (e *Editor) taskBoxAt(r int, x, y float32) *contract.ContainerDecor {
	l := e.form(r)
	left := e.contentX()
	lineH := l.m.font.Size * l.m.font.LineHeight
	top := e.rowTextY(r)
	for i := range l.proj.Containers {
		c := &l.proj.Containers[i]
		if c.Kind == contract.BlockItem && c.First && c.Task != contract.TaskNone {
			if x >= left-3 && x <= left+23 && y >= top-2 && y <= top+lineH+2 {
				return c
			}
		}
		switch c.Kind {
		case contract.BlockQuote:
			left += quoteIndent
		case contract.BlockItem:
			left += listIndent
		}
	}
	return nil
}

// tokenColor returns the color of a code token.
func tokenColor(t *theme.Tokens, k editor.TokenKind) ui.Color {
	switch k {
	case editor.TokenKeyword:
		return t.SynKeyword
	case editor.TokenFunction:
		return t.SynFunc
	case editor.TokenComment:
		return t.SynComment
	case editor.TokenString:
		return t.SynString
	}
	return ui.Color{}
}

func tokenizerOf(lang string) editor.Tokenizer {
	if lang == "" {
		return nil
	}
	if l := editor.LanguageNamed(lang); l != nil {
		return l.Tokenizer
	}
	return nil
}

// spansOf styles the segments of a row, with a preedit underlined at display index
// at. A preedit is one more span, in the layout only.
func (e *Editor) spansOf(l *laid, b *contract.Block, preedit string, at int) []ui.Span {
	p, m, t := &l.proj, &l.m, e.t
	spans := make([]ui.Span, 0, len(p.Segments)+2)
	add := func(s *contract.Segment, text string) {
		if text == "" {
			return
		}
		sp := e.styled(text, s, m)
		if b.Kind == contract.BlockCode && s.Kind == contract.SegText {
			spans = e.appendCode(spans, sp, text, b)
			return
		}
		spans = append(spans, sp)
	}
	e.codeState = 0
	pre := func() ui.Span {
		s := ui.Span{Text: preedit, Underline: true, Color: t.Text}
		preedit = ""
		return s
	}
	u := 0
	for i := range p.Segments {
		s := &p.Segments[i]
		if s.Display == "" {
			continue
		}
		n := utf16Len(s.Display)
		if s.Kind == contract.SegObject {
			if preedit != "" && at == u {
				spans = append(spans, pre())
			}
			spans = append(spans, e.objectSpan(s, m))
			u += n
			continue
		}
		if preedit != "" && at >= u && at < u+n {
			cut := utf16ByteOffset(s.Display, at-u)
			add(s, s.Display[:cut])
			spans = append(spans, pre())
			add(s, s.Display[cut:])
		} else {
			add(s, s.Display)
		}
		u += n
	}
	if preedit != "" {
		spans = append(spans, pre())
	}
	if b.Kind == contract.BlockCode {
		spans = expandTabs(spans, m)
	}
	return spans
}

// expandTabs gives a code row's tabs their width: the shaper draws a tab as nothing.
// An object is one UTF-16 unit like the tab, so display indices do not move.
func expandTabs(spans []ui.Span, m *rowMetrics) []ui.Span {
	first := slices.IndexFunc(spans, func(s ui.Span) bool { return strings.IndexByte(s.Text, '\t') >= 0 })
	if first < 0 {
		return spans
	}
	// A column is measured as the row is shaped, so it is the width the text gets.
	digit := spans[first]
	digit.Text = "0"
	w, _ := ui.ShapeRichTextWith([]ui.Span{digit}, m.font, ui.TextShape{}).Size()
	out := make([]ui.Span, 0, len(spans)+2)
	col := 0
	for _, sp := range spans {
		if sp.ObjectWidth > 0 {
			out = append(out, sp)
			continue
		}
		text, start, mark := sp.Text, 0, 0
		emit := func(to int) {
			if to > start {
				part := sp
				part.Text = text[start:to]
				out = append(out, part)
			}
		}
		for j := range len(text) {
			switch text[j] {
			case '\n':
				col, mark = 0, j+1
			case '\t':
				col += utf8.RuneCountInString(text[mark:j])
				emit(j)
				n := tabColumns - col%tabColumns
				out = append(out, ui.Span{Text: "\uFFFC", ObjectWidth: float32(n) * w, ObjectHeight: 1})
				col += n
				start, mark = j+1, j+1
			}
		}
		col += utf8.RuneCountInString(text[mark:])
		emit(len(text))
	}
	return out
}

func (e *Editor) appendCode(spans []ui.Span, base ui.Span, text string, b *contract.Block) []ui.Span {
	tk := tokenizerOf(b.Lang)
	if tk == nil {
		return append(spans, base)
	}
	var tokens []editor.Token
	tokens, e.codeState = tk.Tokenize(text, e.codeState, nil)
	from := 0
	for _, tok := range tokens {
		end := min(tok.End, len(text))
		if end > from {
			s := base
			s.Text = text[from:end]
			s.Color = tokenColor(e.t, tok.Kind)
			spans = append(spans, s)
			from = end
		}
	}
	if from < len(text) {
		s := base
		s.Text = text[from:]
		spans = append(spans, s)
	}
	return spans
}

func (e *Editor) styled(text string, seg *contract.Segment, m *rowMetrics) ui.Span {
	t, st := e.t, seg.Style
	s := ui.Span{Text: text}
	if m.muted || m.struck {
		s.Color = t.Muted
	}
	if m.struck {
		s.Strikethrough = true
	}
	if st&contract.StyleStrong != 0 {
		s.Weight = 700
	}
	if st&contract.StyleEmphasis != 0 {
		s.Italic = true
	}
	if st&contract.StyleStrike != 0 {
		s.Strikethrough = true
	}
	if st&contract.StyleCode != 0 {
		s.Font = theme.Mono
		s.Size = m.font.Size * .85
		if !m.isCodeBlock() {
			s.Background = t.Surface
			s.Radius = 4
		}
	}
	if st&(contract.StyleLink|contract.StyleWikiLink) != 0 {
		s.Color = t.Accent
		s.Underline = true
		s.DecorationColor = t.Accent.Alpha(.5)
	}
	if st&contract.StyleUnderline != 0 {
		s.Underline = true
	}
	// The inline HTML that takes effect (02 §2.1): <mark>, <kbd>, <sup>, <sub>.
	if st&contract.StyleHighlight != 0 {
		s.Background, s.Radius = t.Mark, 3
	}
	if st&contract.StyleKbd != 0 {
		s.Font = theme.Mono
		s.Size = m.font.Size * .82
		s.Background, s.Radius = t.Surface, 4
	}
	if st&(contract.StyleSup|contract.StyleSub) != 0 {
		size := s.Size
		if size == 0 {
			size = m.font.Size
		}
		s.Size = size * noteScale
		if st&contract.StyleSup != 0 {
			s.BaselineShift = size * noteRaise
		} else {
			s.BaselineShift = -size * .2
		}
	}
	if st&(contract.StyleHTML|contract.StyleFootnoteRef) != 0 {
		s.Color = t.Muted
		s.Font = theme.Mono
		s.Size = m.font.Size * .85
	}
	// Inline math shown as source is purple, its $ grey, as in Typora.
	if st&contract.StyleMath != 0 {
		s.Color = t.SynKeyword
	}
	if st&contract.StyleMarker != 0 {
		s.Color = t.Muted
		s.Background = ui.Color{}
		s.Weight = 400
		s.Italic = false
		s.Underline = false
		s.Strikethrough = false
	}
	return s
}

func (m *rowMetrics) isCodeBlock() bool { return m.font.Family == theme.Mono }
