package liveedit

import (
	"time"
	"unicode/utf8"

	"aurink.local/spikes/contract-rc/contract"
	"github.com/egoist/mygo/ui"
)

// Page geometry and how far layouts reach, from 02 §3.
const (
	maxColumn      = 720
	sidePad        = 28
	padTop         = 72
	padBottom      = 360
	overscanBudget = 6   // blocks shaped ahead of the viewport per frame
	maxLaid        = 512 // layouts kept before far ones are dropped
)

// Stats counts what the engine did, for assertions by count (03 §10).
type Stats struct {
	Frames int
	// Shapes counts block layouts made; FrameShapes those since the last frame
	// ended, input included, and MaxFrameShapes the most in a frame.
	Shapes, FrameShapes, MaxFrameShapes int
	Projects, Edits                     int
}

// laid is a block's projection and layout in one display form.
type laid struct {
	proj    contract.BlockProjection
	base    int // block start the projection's offsets are at
	display string
	layout  *ui.TextLayout
	h       float32 // layout height
}

// blockView is what the editor keeps per block: both display forms, so that a
// caret passing through reshapes nothing, and the height counted in hs.
type blockView struct {
	slots [2]*laid // hidden, revealed
	h     float32
}

// edit is a step of the prototype's history. A real one lives in core/document.
type edit struct {
	from           int
	old, new       string
	a0, c0, a1, c1 int
	join, typing   bool
	origin         contract.Origin
}

// Editor is the engine's state, kept by the app across frames.
type Editor struct {
	d     *document
	views []blockView
	hs    heights

	// The selection is always in source bytes.
	anchor, caret           int
	anchorBlock, caretBlock int
	aff                     ui.TextAffinity
	stickyX                 float32
	hasSticky               bool

	// Preedit is a virtual fragment at the caret: never in the source.
	preedit     string
	preSelected ui.TextInputRange
	preLaid     *laid
	joinNext    bool

	undo, redo []edit

	// The scroll position, kept by anchor: scroll counts from the first block's
	// top, and is negative within the top padding.
	scroll        float64
	anchorIdx     int
	anchorOff     float64
	width         float32
	viewW, viewH  float32
	first, last   int
	revealPending bool
	laidN         int
	moreOverscan  bool

	headings     []int
	headingsFrom *document
	headingsVer  int
	labels       map[string]*ui.TextLayout

	services   ui.Services
	handle     ui.Handle
	focused    bool
	blinkStart time.Time
	dragging   bool
	started    bool

	Stats Stats
	// OnPainted, when set, runs as a frame's painting ends, for measuring.
	OnPainted func()
}

// New returns an editor of src with the caret at its start.
func New(src string) *Editor {
	e := &Editor{d: newDocument(src), scroll: -padTop, anchorOff: -padTop, labels: map[string]*ui.TextLayout{}}
	e.views = make([]blockView, len(e.d.blocks))
	e.resetHeights()
	e.setSelection(0, 0)
	return e
}

// Source returns the document's source, the only truth.
func (e *Editor) Source() string { return e.d.src() }

// SourceSelection returns the selection's anchor and caret in source bytes.
func (e *Editor) SourceSelection() (anchor, caret int) { return e.anchor, e.caret }

// BlockCount returns the number of blocks.
func (e *Editor) BlockCount() int { return len(e.d.blocks) }

// Scroll returns the scroll offset, from the first block's top.
func (e *Editor) Scroll() float64 { return e.scroll }

// Preedit returns the composition shown at the caret.
func (e *Editor) Preedit() string { return e.preedit }

// LaidBlocks returns how many display forms hold a layout.
func (e *Editor) LaidBlocks() int { return e.laidN }

// VisibleBlocks returns the blocks painted in the last frame.
func (e *Editor) VisibleBlocks() (first, last int) { return e.first, e.last }

// SetCaret puts the caret at a source offset.
func (e *Editor) SetCaret(off int) { e.setSelection(off, off) }

// ScrollBy scrolls by dy DIPs, as the wheel does.
func (e *Editor) ScrollBy(dy float64) { e.setScroll(e.scroll + dy) }

// ResetStats clears the counters.
func (e *Editor) ResetStats() { e.Stats = Stats{} }

// View builds the editor's element: it scrolls itself, since the anchor must be
// able to correct the offset in the frame that measures a block.
func (e *Editor) View(c *ui.Context) ui.Element {
	box := ui.Box(c).Fill().Focusable().Clip().Bind(&e.handle).Cursor(ui.CursorText).FocusRing(false).
		Role(ui.RoleTextField).Label("Editor").
		HandleTextInput(e).HandleInput(e.input).Draw(e.paint)
	e.services = c.Services()
	if !e.started {
		e.started = true
		e.handle.Focus(c)
	}
	focused := e.handle.Focused(c)
	if focused != e.focused {
		e.focused = focused
		e.blinkStart = c.Now()
	}
	if focused {
		const blink = 530 * time.Millisecond
		if phase := c.Now().Sub(e.blinkStart); phase < 30*time.Second {
			c.After(blink - phase%blink)
		}
	}
	return box
}

func (e *Editor) revealed(i int) bool { return i == e.caretBlock || i == e.anchorBlock }

// clampCaret keeps an offset out of a block's trailer and off the middle of a rune.
func (e *Editor) clampCaret(off int) int {
	src := e.d.src()
	off = max(0, min(off, len(src)))
	for off > 0 && off < len(src) && !utf8.RuneStart(src[off]) {
		off--
	}
	b := &e.d.blocks[e.d.blockAt(off)]
	return max(b.CStart, min(off, b.CEnd))
}

func (e *Editor) setSelection(anchor, caret int) {
	oldA, oldC := e.anchorBlock, e.caretBlock
	e.anchor, e.caret = e.clampCaret(anchor), e.clampCaret(caret)
	e.anchorBlock, e.caretBlock = e.d.blockAt(e.anchor), e.d.blockAt(e.caret)
	e.preLaid = nil
	e.revealPending = true
	// Blocks that changed form change height now, so that tops stay exact.
	for _, i := range [...]int{oldA, oldC, e.anchorBlock, e.caretBlock} {
		if i < len(e.views) && e.views[i].h > 0 && e.width > 0 {
			e.current(i)
		}
	}
}

// laidFor returns block i in the given form, shaping it only if needed.
func (e *Editor) laidFor(i int, revealed bool) *laid {
	v := &e.views[i]
	k := 0
	if revealed {
		k = 1
	}
	b := &e.d.blocks[i]
	if l := v.slots[k]; l != nil {
		if l.base != b.Start {
			rebase(&l.proj, b.Start-l.base)
			l.base = b.Start
		}
		return l
	}
	l := &laid{proj: project(e.d, i, revealed), base: b.Start}
	e.Stats.Projects++
	l.display = displayText(&l.proj)
	e.shape(l, b, "", 0)
	v.slots[k] = l
	e.laidN++
	return l
}

// shape lays out l's segments, with a preedit at display index at.
func (e *Editor) shape(l *laid, b *block, preedit string, at int) {
	font := blockFont(b)
	spans := spansOf(&l.proj, font, preedit, at)
	l.layout = ui.ShapeRichTextWith(spans, font, ui.TextShape{Width: max(e.width, 1)})
	e.Stats.Shapes++
	e.Stats.FrameShapes++
	_, l.h = l.layout.Size()
	if l.h <= 0 {
		l.h = font.Size * font.LineHeight
	}
}

// current returns block i as it shows now, and keeps its height up to date.
func (e *Editor) current(i int) *laid {
	var l *laid
	if e.preedit != "" && i == e.caretBlock {
		l = e.composed()
	} else {
		l = e.laidFor(i, e.revealed(i))
	}
	b := &e.d.blocks[i]
	above, below := blockSpace(b)
	h := above + l.h + below
	v := &e.views[i]
	if v.h != h {
		if v.h > 0 {
			e.hs.add(i, float64(h-v.h), 0)
		} else {
			e.hs.add(i, float64(h), -1)
		}
		v.h = h
	}
	return l
}

// composed returns the caret's block with the preedit spliced in. It has its own
// layout, made once per composition change, and nothing of it reaches the source.
func (e *Editor) composed() *laid {
	if e.preLaid != nil {
		return e.preLaid
	}
	base := e.laidFor(e.caretBlock, true)
	at := toDisplay(&base.proj, e.caret, contract.Downstream)
	l := &laid{proj: base.proj, base: base.base}
	u := utf16ByteOffset(base.display, at)
	l.display = base.display[:u] + e.preedit + base.display[u:]
	e.shape(l, &e.d.blocks[e.caretBlock], e.preedit, at)
	e.preLaid = l
	return l
}

func (e *Editor) resetHeights() {
	hs := make([]float32, len(e.views))
	for i := range e.views {
		hs[i] = e.views[i].h
	}
	e.hs.reset(hs)
	e.estimate()
}

func (e *Editor) estimate() {
	m, unknown := e.hs.totals()
	if n := len(e.views) - unknown; n > 0 {
		e.hs.est = m / float64(n)
	} else {
		e.hs.est = 16 * 1.75 * 2
	}
}

func (e *Editor) setScroll(y float64) {
	y = max(-padTop, min(y, e.maxScroll()))
	e.scroll = y
	e.anchorIdx = e.hs.at(max(y, 0))
	e.anchorOff = y - e.hs.top(e.anchorIdx)
}

func (e *Editor) maxScroll() float64 {
	return max(-padTop, e.hs.top(len(e.views))+padBottom-float64(e.viewH))
}

// layoutView lays out the blocks in view of a w×h box. The anchor block and its
// offset decide the scroll: a block above changing height moves nothing on screen.
func (e *Editor) layoutView(w, h float32) {
	column := max(min(w-2*sidePad, maxColumn), 1)
	if column != e.width {
		e.dropAll()
		e.width = column
	}
	e.viewW, e.viewH = w, h
	e.estimate()
	e.place()
	if e.revealPending {
		e.revealPending = false
		_, y, ch := e.caretRect()
		if y < 0 || y+ch > h {
			if y < 0 {
				e.setScroll(e.scroll + float64(y-8))
			} else {
				e.setScroll(e.scroll + float64(y+ch-h+8))
			}
			e.place()
			// The estimates between may have been off; one correction settles it.
			if _, y, ch = e.caretRect(); y < 0 || y+ch > h {
				e.setScroll(e.scroll + float64(y+ch/2-h/2))
				e.place()
			}
		}
	}
	if s := max(-padTop, min(e.scroll, e.maxScroll())); s != e.scroll {
		e.setScroll(s)
		e.place()
	}
	e.overscan()
	if e.laidN > maxLaid+e.last-e.first {
		e.forget()
	}
}

// place shapes the blocks in view, from the anchor out.
func (e *Editor) place() {
	n := len(e.views)
	e.anchorIdx = max(0, min(e.anchorIdx, n-1))
	e.current(e.anchorIdx)
	e.first = e.anchorIdx
	for e.first > 0 && e.hs.top(e.anchorIdx)+e.anchorOff-e.hs.top(e.first) < 0 {
		e.first--
		e.current(e.first)
	}
	e.scroll = max(-padTop, e.hs.top(e.anchorIdx)+e.anchorOff)
	p := e.first
	y := e.hs.top(p)
	for ; p < n; p++ {
		e.current(p)
		y = e.hs.top(p + 1)
		if y >= e.scroll+float64(e.viewH) {
			break
		}
	}
	e.last = min(p, n-1)
}

// overscan shapes a few blocks a screen around the view ahead of need, within a
// budget per frame, and asks for another frame while some remain.
func (e *Editor) overscan() {
	budget := overscanBudget
	e.moreOverscan = false
	screen := float64(e.viewH)
	wanted := func(i int) bool {
		if e.views[i].slots[0] != nil {
			return false
		}
		if budget == 0 {
			e.moreOverscan = true
			return false
		}
		budget--
		return true
	}
	for i := e.last + 1; i < len(e.views) && e.hs.top(i) < e.scroll+2*screen; i++ {
		if wanted(i) {
			e.current(i)
		}
	}
	for i := e.first - 1; i >= 0 && e.hs.top(i+1) > e.scroll-screen; i-- {
		if wanted(i) {
			e.current(i)
		}
	}
	e.scroll = max(-padTop, e.hs.top(e.anchorIdx)+e.anchorOff)
}

// forget drops the layouts far from view; heights stay.
func (e *Editor) forget() {
	lo, hi := e.first-maxLaid/4, e.last+maxLaid/4
	for i := range e.views {
		if (i < lo || i > hi) && !e.revealed(i) {
			e.dropSlots(i)
		}
	}
}

func (e *Editor) dropSlots(i int) {
	v := &e.views[i]
	for k := range v.slots {
		if v.slots[k] != nil {
			v.slots[k] = nil
			e.laidN--
		}
	}
}

func (e *Editor) dropAll() {
	for i := range e.views {
		e.dropSlots(i)
		e.views[i].h = 0
	}
	e.preLaid = nil
	e.resetHeights()
}

// applyEdit replaces source bytes and carries the views over to the new blocks:
// only blocks the edit touched lose their layouts.
func (e *Editor) applyEdit(from, to int, text string) {
	remap := e.d.replace(from, to, text)
	e.Stats.Edits++
	views := make([]blockView, len(e.d.blocks))
	identity := len(views) == len(e.views)
	for i, m := range remap {
		if m.to != i {
			identity = false
		}
		if m.touched {
			e.dropSlots(i)
		}
		if m.to >= 0 {
			views[m.to] = e.views[i]
		} else {
			e.dropSlots(i)
		}
	}
	if a := e.anchorIdx; a < len(remap) {
		if m := remap[a]; m.to >= 0 {
			e.anchorIdx = m.to
		} else {
			e.anchorIdx = e.d.blockAt(from)
		}
	}
	e.views = views
	if !identity {
		e.resetHeights()
	}
	e.preLaid = nil
	e.headingsFrom = nil
}

// caretRect returns the caret in element coordinates: x, y and height.
func (e *Editor) caretRect() (x, y, h float32) {
	l := e.current(e.caretBlock)
	idx := toDisplay(&l.proj, e.caret, contract.Downstream)
	if e.preedit != "" {
		idx += e.preSelected.End
	}
	return e.boundsAt(e.caretBlock, l, idx)
}

func (e *Editor) boundsAt(i int, l *laid, idx int) (x, y, h float32) {
	r := l.layout.CaretAt(ui.TextCaretPosition{Index: idx, Affinity: e.aff})
	if l.display == "" {
		font := blockFont(&e.d.blocks[i])
		r.H = font.Size * font.LineHeight
	}
	return e.contentX() + r.X, e.blockTextY(i) + r.Y, r.H
}

func (e *Editor) contentX() float32 { return (e.viewW - e.width) / 2 }

// blockTextY returns the element y of block i's first line.
func (e *Editor) blockTextY(i int) float32 {
	above, _ := blockSpace(&e.d.blocks[i])
	return float32(e.hs.top(i)-e.scroll) + above
}
