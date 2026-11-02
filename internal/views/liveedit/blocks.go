package liveedit

import (
	"strings"

	"aurink/internal/contract"
	"aurink/internal/views/theme"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// rowMetrics is a row's typography and geometry, fixed when it is shaped.
type rowMetrics struct {
	font          ui.Font
	above, below  float32 // room above and below the lines, inside the row
	x             float32 // the text's left, from the column's left
	width         float32 // the width the text wraps at
	muted, struck bool
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

func widgetOf(b *contract.Block) widget {
	if w, ok := widgets[b.Kind]; ok {
		return w
	}
	return plainWidget{}
}

const (
	listIndent  = 26
	quoteIndent = 36
	codePadX    = 14
	codeGutter  = 30
	codeHeader  = 30
	cellPad     = 10
)

// metricsFor sizes a row: its containers first, then the block's own widget.
func (e *Editor) metricsFor(r int, b *contract.Block, p *contract.BlockProjection, lead bool) rowMetrics {
	m := rowMetrics{font: theme.BodyFont(), below: theme.Body * .85, width: e.width}
	if lead {
		m.font = theme.LeadFont()
	}
	for i := range p.Containers {
		switch c := &p.Containers[i]; c.Kind {
		case contract.BlockQuote:
			m.x += quoteIndent
			m.font = theme.QuoteFont()
			m.below = theme.Body * 1.5
		case contract.BlockItem:
			m.x += listIndent
			m.below = theme.Body * .4
			if c.Task == contract.TaskDone {
				m.struck = true
			}
		case contract.BlockFootnoteDef:
			m.muted = true
		}
	}
	widgetOf(b).metrics(e, r, b, &m)
	m.width = max(m.width-m.x, 1)
	return m
}

type headingWidget struct{ plainWidget }

func (headingWidget) metrics(_ *Editor, _ int, b *contract.Block, m *rowMetrics) {
	m.font = theme.HeadingFont(b.Level)
	switch b.Level {
	case 1:
		m.above, m.below = 8, 12
	case 2:
		m.above, m.below = theme.Body*1.9, theme.Body*.5
	default:
		m.above, m.below = theme.Body*1.5, theme.Body*.4
	}
	m.muted = b.Level == 6
}

type blankWidget struct{ plainWidget }

func (blankWidget) metrics(_ *Editor, _ int, _ *contract.Block, m *rowMetrics) { m.below = 0 }

// sourceWidget shows syntax M1 does not render as pale source (06 §2.1).
type sourceWidget struct{ plainWidget }

func (sourceWidget) metrics(_ *Editor, _ int, _ *contract.Block, m *rowMetrics) {
	m.font = theme.CodeFont()
	m.muted = true
}

// breakWidget is the section break "···"; its source shows with the caret on it.
type breakWidget struct{ plainWidget }

func (breakWidget) metrics(_ *Editor, _ int, _ *contract.Block, m *rowMetrics) {
	m.above, m.below = theme.Body, theme.Body*1.5
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

func (codeWidget) metrics(_ *Editor, _ int, _ *contract.Block, m *rowMetrics) {
	m.font = theme.CodeFont()
	m.above, m.below = codeHeader, theme.Body*1.4
	m.x = codePadX + codeGutter
	m.width -= codePadX
}

func (codeWidget) back(rc *rowContext) {
	rc.p.Fill(ui.Rect{X: rc.ox, Y: rc.top, W: rc.e.width, H: rc.height - theme.Body + 12}, rc.t.CodeBg, 10)
	rc.p.Stroke(ui.Rect{X: rc.ox, Y: rc.top, W: rc.e.width, H: rc.height - theme.Body + 12}, rc.t.Border, 10, 1)
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
	if v.header {
		m.font.Weight = 600
	}
}

func (cellWidget) back(rc *rowContext) {
	if v := &rc.e.rows[rc.row]; v.header {
		rc.p.Fill(rc.cell(), rc.t.Surface, 0)
	}
}

func (cellWidget) front(rc *rowContext) { rc.p.Stroke(rc.cell(), rc.t.Border, 0, 1) }

func (rc *rowContext) cell() ui.Rect {
	e, v := rc.e, &rc.e.rows[rc.row]
	col := e.width / float32(max(v.cols, 1))
	return ui.Rect{X: rc.ox + float32(v.col)*col, Y: rc.top, W: col, H: e.rows[rc.row-v.col].h}
}

// paintContainers draws what a row's containers show: list markers, checkboxes,
// the hanging quote mark and footnote labels.
func (rc *rowContext) paintContainers() {
	e, p, t := rc.e, rc.p, rc.t
	x := rc.ox
	lineH := rc.l.m.font.Size * rc.l.m.font.LineHeight
	y := rc.textY()
	for i := range rc.l.proj.Containers {
		c := &rc.l.proj.Containers[i]
		switch c.Kind {
		case contract.BlockQuote:
			if c.First {
				p.TextLayout(e.serifMark(), x+2, y-4, t.Accent.Alpha(.5))
			}
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
	return spans
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
	if st&contract.StyleLink != 0 {
		s.Color = t.Accent
		s.Underline = true
		s.DecorationColor = t.Accent.Alpha(.5)
	}
	if st&contract.StyleUnderline != 0 {
		s.Underline = true
	}
	if st&(contract.StyleHTML|contract.StyleFootnoteRef) != 0 {
		s.Color = t.Muted
		s.Font = theme.Mono
		s.Size = m.font.Size * .85
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
