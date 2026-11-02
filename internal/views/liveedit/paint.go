package liveedit

import (
	"sort"
	"time"

	"aurink/internal/contract"
	"aurink/internal/views/theme"
	"aurink/locales"
	"github.com/egoist/mygo/ui"
)

// label returns a retained layout of a short text: margin tags and line numbers
// never cost a row reshape.
func (e *Editor) label(text string, size float32) *ui.TextLayout {
	key := labelKey{text, size}
	l := e.labels[key]
	if l == nil {
		if len(e.labels) > 2048 {
			clear(e.labels)
		}
		l = ui.ShapeText(text, ui.Font{Family: theme.Mono, Size: size}, 0)
		e.labels[key] = l
	}
	return l
}

type labelKey struct {
	text string
	size float32
}

func (e *Editor) serifMark() *ui.TextLayout {
	if e.quoteMark == nil {
		e.quoteMark = ui.ShapeText("“", ui.Font{Family: theme.Serif, Size: 34}, 0)
	}
	return e.quoteMark
}

func (e *Editor) copyLabel(b *contract.Block) string {
	if e.copiedBlock == b.ID {
		return locales.LiveeditCopied()
	}
	return locales.LiveeditCopy()
}

func (e *Editor) fillRange(p *ui.Painter, l *laid, b *contract.Block, x, y float32, from, to int, c ui.Color) {
	pr := e.opts.Projector
	span := contract.Range{Start: b.Lead.Start, End: b.Trailer.End}
	a := pr.ToDisplay(&l.proj, clampTo(from, span), contract.Upstream)
	z := pr.ToDisplay(&l.proj, clampTo(to, span), contract.Downstream)
	for _, s := range l.layout.SelectionRects(ui.TextInputRange{Start: a, End: z}) {
		p.Fill(ui.Rect{X: x + s.X, Y: y + s.Y, W: s.W, H: s.H}, c, 0)
	}
}

// paint lays out the rows in view and paints them.
func (e *Editor) paint(p *ui.Painter, r ui.Rect) {
	e.orgX, e.orgY = r.X, r.Y
	e.layoutView(r.W, r.H)
	e.paintRows(p, r)
	e.paintLinkHint(p, r)
	e.paintNotePreview(p, r)
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

func (e *Editor) paintRows(p *ui.Painter, r ui.Rect) {
	t := e.t
	p.Fill(r, t.Sheet, 0)
	selFrom, selTo := min(e.anchor, e.head), max(e.anchor, e.head)
	ox := r.X + e.contentX()
	for i := e.first; i <= e.last; i++ {
		l := e.current(i)
		b := e.block(i)
		top := r.Y + float32(e.rowTop(i)-e.scroll)
		rc := rowContext{e: e, p: p, t: t, row: i, b: b, l: l, ox: ox, top: top, height: e.rows[i].h}
		w := widgetOf(b)
		w.back(&rc)
		x, y := rc.textX(), rc.textY()
		e.paintFind(p, l, b, x, y)
		if selFrom != selTo && selFrom < b.Trailer.End && selTo > b.Lead.Start {
			e.fillRange(p, l, b, x, y, selFrom, selTo, t.Selection)
		}
		text := t.Text
		if e.focusMode && i != e.caretRow {
			text = text.Alpha(0.35)
		}
		p.TextLayout(l.layout, x, y, text)
		rc.paintObjects()
		rc.paintContainers()
		w.front(&rc)
		if i == e.caretRow && e.focused {
			p.TextLayout(e.label(marginTag(b), theme.UIBadge), ox-theme.MarginLabel, y+2, t.Muted)
		}
	}
	if e.focused && selFrom == selTo && !e.readOnly() {
		const blink = 530 * time.Millisecond
		if phase := p.Now().Sub(e.blinkStart); (phase/blink)%2 == 0 || phase < 0 {
			cx, cy, ch := e.caretRect()
			p.Fill(ui.Rect{X: r.X + cx, Y: r.Y + cy, W: max(1/p.Scale(), 1.5), H: ch}, t.Accent, 0)
		}
	}
}

func marginTag(b *contract.Block) string {
	if b.Kind == contract.BlockHeading {
		return "H" + itoa(b.Level)
	}
	return "¶"
}

func (e *Editor) paintFind(p *ui.Painter, l *laid, b *contract.Block, x, y float32) {
	start := sort.Search(len(e.find), func(k int) bool { return e.find[k].End > b.Lead.Start })
	for k := start; k < len(e.find) && e.find[k].Start < b.Trailer.End; k++ {
		c := e.t.Mark
		if k == e.findCur {
			c = e.t.Warning.Alpha(.6)
		}
		e.fillRange(p, l, b, x, y, e.find[k].Start, e.find[k].End, c)
	}
}

// paintCheckbox draws a task's box. Toggling draws the tick along its path, driven
// by the frame clock, and asks for no frames once it has finished.
func (e *Editor) paintCheckbox(p *ui.Painter, t *theme.Tokens, c *contract.ContainerDecor, x, y float32) {
	box := ui.Rect{X: x, Y: y, W: 18, H: 18}
	checked := c.Task == contract.TaskDone
	if !checked {
		p.Stroke(box, t.Muted, 5, 1.4)
		return
	}
	progress := float32(1)
	if !e.tickStart.IsZero() && e.tickBlock == c.Block {
		const d = 260 * time.Millisecond
		if el := p.Now().Sub(e.tickStart); el < d {
			progress = float32(el) / float32(d)
			p.AnimationFrame()
		} else {
			e.tickStart = time.Time{}
		}
	}
	p.Fill(box, t.Accent, 5)
	path := new(ui.Path).MoveTo(x+4.5, y+9.5).LineTo(x+7.8, y+12.6).LineTo(x+13.5, y+5.8).Trim(0, progress)
	p.StrokePath(path, 2, t.OnAccent)
}

// paintLinkHint shows the target of the link under the pointer.
func (e *Editor) paintLinkHint(p *ui.Painter, r ui.Rect) {
	if e.hoverLink == "" {
		return
	}
	text := e.label(e.hoverLink, theme.UIStatus)
	w, h := text.Size()
	x, y := r.X+min(e.hoverX, r.W-w-24), r.Y+e.hoverY+18
	p.Fill(ui.Rect{X: x, Y: y, W: w + 16, H: h + 10}, e.t.Solid, 7)
	p.Stroke(ui.Rect{X: x, Y: y, W: w + 16, H: h + 10}, e.t.Border, 7, 1)
	p.TextLayout(text, x+8, y+5, e.t.Muted)
}

// paintRail draws the slider of the structure rail (02 §3) from the Fenwick tree
// alone, without shaping any row. The heading marks are Rail's.
func (e *Editor) paintRail(p *ui.Painter, r ui.Rect) {
	total := e.hs.top(len(e.rows)) + theme.PadTop + theme.PadBottom
	if total <= float64(r.H) {
		return
	}
	scale := float64(r.H) / total
	thumb := max(float32(float64(r.H)*scale), 24)
	y := r.Y + float32((e.scroll+theme.PadTop)*scale)
	p.Fill(ui.Rect{X: r.X + r.W - 8, Y: y, W: 6, H: thumb}, e.t.Text.Alpha(0.25), 3)
}
