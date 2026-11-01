package liveedit

import (
	"fmt"
	"maps"
	"time"

	"aurink/internal/contract"
	"aurink/internal/views/editorapi"
	"github.com/egoist/mygo/ui"
)

// Page geometry and how far layouts reach, from 02 §3.
const (
	maxColumn      = 720
	sidePad        = 28
	padTop         = 72
	padBottom      = 360
	overscanBudget = 6   // rows shaped ahead of the viewport per frame
	maxLaid        = 512 // layouts kept before far ones are dropped
	maxForms       = 4   // display forms kept per row
)

// Options is what an editor is built from.
type Options struct {
	// Model is the document. It defaults to Session's model.
	Model contract.DocumentModel
	// Session is optional: an editor edits a model, the session only passes through.
	Session   contract.Session
	Projector contract.Projector
	Editing   contract.Editing
	// Prefs returns the editing preferences; nil uses the defaults.
	Prefs       func() contract.EditPrefs
	Exts        contract.Extensions
	ObjectLangs []string
}

// Stats counts what the engine did, for assertions by count (03 §10).
type Stats struct {
	Frames int
	// Shapes counts layouts made; FrameShapes those since the last frame ended,
	// input included, and MaxFrameShapes the most in a frame.
	Shapes, FrameShapes, MaxFrameShapes int
	Projects                            int
	// RowRebuilds counts walks over all rows, which ordinary typing must not do.
	RowRebuilds int
	Edits       int
}

// revealKey says what a row's projection depends on in the selection, relative to
// the row's start so that it survives the row moving. caret is -1 without a caret.
type revealKey struct{ caret, from, to int }

var hiddenKey = revealKey{caret: -1}

// laid is a row's projection and layout in one display form.
type laid struct {
	key     revealKey
	proj    contract.BlockProjection
	base    int // the row start that the projection's offsets are at
	display string
	layout  *ui.TextLayout
	h       float32 // layout height
}

// row is what the editor keeps per leaf block. leaf is the block's index in the
// AST: it holds until a rebuild, and AST.Index would build a map per snapshot.
type row struct {
	id    contract.BlockID
	leaf  int
	kind  contract.BlockKind
	forms []*laid
	cur   *laid
	h     float32
}

type headingMark struct {
	row, level int
}

// Editor is the engine's state, kept by the app across frames.
type Editor struct {
	model contract.DocumentModel
	opts  Options
	text  contract.Text
	ast   contract.AST
	off   func()

	rows   []row
	rowOf  map[contract.BlockID]int
	astLen int
	hs     heights

	// The selection is always in source bytes, as the model has it.
	anchor, head        int
	anchorRow, caretRow int
	touched             []contract.BlockID
	aff                 ui.TextAffinity
	stickyX             float32
	hasSticky           bool

	// Preedit is a virtual fragment at the caret, never in the source; preRange is
	// what a commit replaces, so that a selection goes in one step with it.
	preedit     string
	preSelected ui.TextInputRange
	preRange    contract.Range
	preLaid     *laid

	// The scroll position, kept by anchor: scroll counts from the first row's top,
	// and is negative within the top padding.
	scroll        float64
	anchorIdx     int
	anchorOff     float64
	width         float32
	viewW, viewH  float32
	orgX, orgY    float32
	first, last   int
	revealPending bool
	revealAlign   editorapi.ScrollAlign
	laidN         int
	moreOverscan  bool

	headings      []headingMark
	headingsStale bool
	tocRows       []int
	linkDefs      map[string]contract.LinkDef
	footnotes     map[string]int
	labels        map[string]*ui.TextLayout

	mode       editorapi.Mode
	focusMode  bool
	typewriter bool
	find       []contract.Range
	findCur    int

	services   ui.Services
	hasView    bool
	handle     ui.Handle
	focusReq   bool
	focused    bool
	blinkStart time.Time

	dragging         bool
	dragUnit         int
	dragFrom, dragTo int
	dragX, dragY     float32
	dragSpeed        float32
	dragLast         time.Time
	Stats            Stats
	// OnPainted, when set, runs as a frame's painting ends, for measuring.
	OnPainted func()
}

// New returns an editor of a model. Close ends its subscription.
func New(o Options) *Editor {
	if o.Model == nil && o.Session != nil {
		o.Model = o.Session.Model()
	}
	if o.Prefs == nil {
		o.Prefs = func() contract.EditPrefs {
			return contract.EditPrefs{BulletMarker: '-', StrongDelim: "**", EmphasisDelim: "*", FenceDelim: "```", AutoPair: true, TabSize: 2, Exts: o.Exts}
		}
	}
	e := &Editor{model: o.Model, opts: o, scroll: -padTop, anchorOff: -padTop, labels: map[string]*ui.TextLayout{}, findCur: -1}
	e.text, e.ast = o.Model.Text(), o.Model.AST()
	e.rebuildRows(nil)
	e.linkDefs, e.footnotes = e.ast.LinkDefs(), e.ast.Footnotes()
	e.refreshSelection()
	e.off = o.Model.Subscribe(e.onEvent)
	return e
}

// Close stops following the model.
func (e *Editor) Close() {
	if e.off != nil {
		e.off()
		e.off = nil
	}
}

// RowCount returns the number of rows, one per leaf block.
func (e *Editor) RowCount() int { return len(e.rows) }

// VisibleRows returns the rows painted in the last frame.
func (e *Editor) VisibleRows() (first, last int) { return e.first, e.last }

// Preedit returns the composition shown at the caret.
func (e *Editor) Preedit() string { return e.preedit }

// Display returns a row's display text as it shows now.
func (e *Editor) Display(r int) string { return e.form(r).display }

// LinkTarget returns the first link's target in a row as it shows now.
func (e *Editor) LinkTarget(r int) string {
	for _, s := range e.form(r).proj.Segments {
		if s.Link != "" {
			return s.Link
		}
	}
	return ""
}

// ScrollBy scrolls by dy DIPs, as the wheel does.
func (e *Editor) ScrollBy(dy float64) { e.setScroll(e.scroll + dy) }

// ResetStats clears the counters.
func (e *Editor) ResetStats() { e.Stats = Stats{} }

// CheckRows compares the kept rows with the AST, for tests of the incremental path.
func (e *Editor) CheckRows() error {
	i := 0
	for l := range e.ast.Leaves() {
		if i >= len(e.rows) {
			return fmt.Errorf("more leaves than %d rows", len(e.rows))
		}
		id := e.ast.Block(l).ID
		if e.rows[i].id != id || e.rows[i].leaf != l {
			return fmt.Errorf("row %d is block %d at %d, the AST has %d at %d", i, e.rows[i].id, e.rows[i].leaf, id, l)
		}
		if e.rowOf[id] != i {
			return fmt.Errorf("rowOf[%d] = %d, want %d", id, e.rowOf[id], i)
		}
		i++
	}
	if i != len(e.rows) {
		return fmt.Errorf("%d leaves, %d rows", i, len(e.rows))
	}
	return nil
}

// View builds the editor's element: it scrolls itself, since the anchor must be
// able to correct the offset in the frame that measures a row.
func (e *Editor) View(c *ui.Context) ui.Element {
	box := ui.Box(c).Fill().Focusable().Clip().Bind(&e.handle).Cursor(ui.CursorText).FocusRing(false).
		Role(ui.RoleTextField).Label("Editor").
		HandleTextInput(e).HandleInput(e.input).Draw(e.paint)
	e.services, e.hasView = c.Services(), true
	if e.focusReq {
		e.focusReq = false
		e.handle.Focus(c)
	}
	focused := e.handle.Focused(c)
	if focused != e.focused {
		e.focused = focused
		e.blinkStart = c.Now()
		if !focused {
			e.UnmarkText()
			e.model.BreakCoalescing()
		}
	}
	if focused {
		const blink = 530 * time.Millisecond
		if phase := c.Now().Sub(e.blinkStart); phase < 30*time.Second {
			c.After(blink - phase%blink)
		}
	}
	if e.dragging && e.dragSpeed != 0 {
		e.autoscroll(c.Now())
		c.After(16 * time.Millisecond)
	}
	return box
}

func (e *Editor) invalidate() {
	if e.hasView {
		e.services.Invalidate()
	}
}

func (e *Editor) readOnly() bool { return e.mode == editorapi.ModeRead || e.model.ReadOnly() }

func (e *Editor) block(r int) *contract.Block { return e.ast.Block(e.rows[r].leaf) }

// rowAt returns the row of a source offset. LeafAt gives an offset in a row's Lead
// to that row, but at the end of a cell it belongs to the cell it ends.
func (e *Editor) rowAt(off int) int {
	off = max(0, min(off, e.text.Len()))
	r, ok := e.rowOf[e.ast.Block(e.ast.LeafAt(off)).ID]
	if !ok {
		return 0
	}
	if b := e.block(r); off < b.Range.Start && r > 0 && e.block(r-1).Range.End == off {
		return r - 1
	}
	return r
}

func clampTo(off int, rg contract.Range) int { return max(rg.Start, min(off, rg.End)) }

func (e *Editor) onEvent(ev contract.DocEvent) {
	switch ev.Kind {
	case contract.DocTextChanged, contract.DocReplaced:
		e.text, e.ast = e.model.Text(), e.model.AST()
		e.Stats.Edits++
		e.syncRows(ev)
		e.refreshSelection()
		e.preLaid = nil
	case contract.DocSelectionChanged:
		e.refreshSelection()
	case contract.DocReadOnlyChanged:
		if e.model.ReadOnly() {
			e.cancelPreedit()
		}
	}
	e.invalidate()
}

// syncRows carries the rows over an edit. An edit inside rows that all survive, the
// ordinary keystroke, touches only them; anything else walks the rows once.
func (e *Editor) syncRows(ev contract.DocEvent) {
	fast := ev.Kind == contract.DocTextChanged && e.ast.Len() == e.astLen && e.ast.LeafCount() == len(e.rows)
	var dirty []int
	if fast {
		for _, id := range ev.Dirty {
			r, ok := e.rowOf[id]
			if !ok {
				// Ancestors of the edit are dirty too; they are not rows.
				if !e.ancestorOfChange(id, ev.Changes) {
					fast = false
					break
				}
				continue
			}
			if e.ast.Block(e.rows[r].leaf).ID != id {
				fast = false
				break
			}
			dirty = append(dirty, r)
		}
	}
	tables := !fast
	if fast {
		for _, r := range dirty {
			b := e.block(r)
			was := e.rows[r].kind
			if was == contract.BlockHeading || b.Kind == contract.BlockHeading {
				e.headingsStale = true
				e.dropTOC()
			}
			if was == contract.BlockLinkDef || b.Kind == contract.BlockLinkDef || e.inFootnote(b) {
				tables = true
			}
			e.rows[r].kind = b.Kind
			e.dropForms(r)
		}
	} else {
		e.rebuildRows(ev.Dirty)
		if ev.Kind == contract.DocReplaced {
			e.dropAllForms()
		}
	}
	// A cached projection is good only if no change touches its bytes (04 §4.3.6).
	for _, c := range ev.Changes {
		e.dropForms(e.rowAt(c.NewStart))
		e.dropForms(e.rowAt(c.NewEnd))
	}
	if tables {
		links, notes := e.ast.LinkDefs(), e.ast.Footnotes()
		if !maps.Equal(links, e.linkDefs) || !maps.Equal(notes, e.footnotes) {
			e.dropAllForms()
		}
		e.linkDefs, e.footnotes = links, notes
	}
}

func (e *Editor) ancestorOfChange(id contract.BlockID, changes []contract.Change) bool {
	for _, c := range changes {
		path := e.ast.Path(c.NewStart)
		for _, i := range path[:max(len(path)-1, 0)] { // the last is the leaf
			if e.ast.Block(i).ID == id {
				return true
			}
		}
	}
	return false
}

func (e *Editor) inFootnote(b *contract.Block) bool {
	for _, i := range e.ast.Path(b.Range.Start) {
		if e.ast.Block(i).Kind == contract.BlockFootnoteDef {
			return true
		}
	}
	return false
}

func (e *Editor) dropTOC() {
	for _, r := range e.tocRows {
		e.dropForms(r)
	}
}

// rebuildRows walks the leaves, keeping what is known of those that survive.
func (e *Editor) rebuildRows(dirty []contract.BlockID) {
	e.Stats.RowRebuilds++
	var anchorID contract.BlockID
	if e.anchorIdx < len(e.rows) {
		anchorID = e.rows[e.anchorIdx].id
	}
	old, oldOf := e.rows, e.rowOf
	n := e.ast.LeafCount()
	rows := make([]row, 0, n)
	of := make(map[contract.BlockID]int, n)
	e.tocRows = e.tocRows[:0]
	for l := range e.ast.Leaves() {
		b := e.ast.Block(l)
		v := row{id: b.ID, leaf: l, kind: b.Kind}
		if r, ok := oldOf[b.ID]; ok {
			v.forms, v.cur, v.h = old[r].forms, old[r].cur, old[r].h
			delete(oldOf, b.ID)
		}
		if b.Kind == contract.BlockTOC {
			e.tocRows = append(e.tocRows, len(rows))
		}
		of[b.ID] = len(rows)
		rows = append(rows, v)
	}
	for _, r := range oldOf {
		e.laidN -= len(old[r].forms)
	}
	e.rows, e.rowOf, e.astLen = rows, of, e.ast.Len()
	for _, id := range dirty {
		if r, ok := of[id]; ok {
			e.dropForms(r)
		}
	}
	if r, ok := of[anchorID]; ok {
		e.anchorIdx = r
	} else {
		e.anchorIdx = e.rowAt(e.model.Selection().Primary().Head)
	}
	e.headingsStale = true
	e.resetHeights()
}

func (e *Editor) refreshSelection() {
	for _, r := range [...]int{e.anchorRow, e.caretRow} {
		if r < len(e.rows) {
			e.touched = append(e.touched, e.rows[r].id)
		}
	}
	sel := e.model.Selection().Primary()
	e.anchor, e.head = sel.Anchor, sel.Head
	e.anchorRow, e.caretRow = e.rowAt(e.anchor), e.rowAt(e.head)
	e.preLaid = nil
	e.revealPending = true
	e.revealAlign = editorapi.AlignNearest
}

// keyFor returns what row r's projection depends on now.
func (e *Editor) keyFor(r int, b *contract.Block) revealKey {
	if e.mode == editorapi.ModeRead {
		return hiddenKey
	}
	k := hiddenKey
	base := b.Lead.Start
	if r == e.caretRow {
		k.caret = clampTo(e.head, b.Range) - base
	}
	a, z := min(e.anchor, e.head), max(e.anchor, e.head)
	if from, to := max(a, b.Range.Start), min(z, b.Range.End); from < to {
		k.from, k.to = from-base, to-base
	}
	return k
}

func (e *Editor) project(b *contract.Block, leaf int, k revealKey) contract.BlockProjection {
	e.Stats.Projects++
	rv := contract.Reveal{Mode: contract.RevealInline, Caret: -1, ObjectLangs: e.opts.ObjectLangs, Exts: e.opts.Exts}
	if e.mode == editorapi.ModeRead {
		rv.Mode = contract.RevealNone
	}
	base := b.Lead.Start
	if k.caret >= 0 {
		rv.Caret = k.caret + base
	}
	if k.to > k.from {
		rv.Selection = contract.Range{Start: k.from + base, End: k.to + base}
	}
	return e.opts.Projector.Project(e.ast, e.text, leaf, rv)
}

// rebased moves l's offsets to the row's start and reports whether they still span
// the row. The second part guards the cache, it should never be false.
func rebased(l *laid, b *contract.Block) bool {
	if d := b.Lead.Start - l.base; d != 0 {
		segs := l.proj.Segments
		for i := range segs {
			segs[i].Src.Start += d
			segs[i].Src.End += d
		}
		cs := l.proj.Containers
		for i := range cs {
			cs[i].TaskSrc.Start += d
			cs[i].TaskSrc.End += d
		}
		l.base = b.Lead.Start
	}
	segs := l.proj.Segments
	return len(segs) == 0 || (segs[0].Src.Start == b.Lead.Start && segs[len(segs)-1].Src.End == b.Trailer.End)
}

// form returns row r as the selection shows it, shaping only if the display is new.
func (e *Editor) form(r int) *laid {
	v := &e.rows[r]
	b := e.ast.Block(v.leaf)
	k := e.keyFor(r, b)
	if l := v.cur; l != nil && l.key == k && rebased(l, b) {
		return l
	}
	for _, l := range v.forms {
		if l.key == k && rebased(l, b) {
			v.cur = l
			return l
		}
	}
	p := e.project(b, v.leaf, k)
	for _, l := range v.forms {
		if l.proj.Sig == p.Sig {
			l.proj, l.key, l.base = p, k, b.Lead.Start
			v.cur = l
			return l
		}
	}
	l := &laid{key: k, proj: p, base: b.Lead.Start, display: e.opts.Projector.DisplayText(&p)}
	e.shape(l, b, "", 0)
	if len(v.forms) >= maxForms {
		victim := 0
		for i, f := range v.forms {
			if f.key != hiddenKey && f != v.cur {
				victim = i
				break
			}
		}
		v.forms = append(v.forms[:victim], v.forms[victim+1:]...)
		e.laidN--
	}
	v.forms = append(v.forms, l)
	v.cur = l
	e.laidN++
	return l
}

// shape lays out l's segments, with a preedit at display index at.
func (e *Editor) shape(l *laid, b *contract.Block, preedit string, at int) {
	font := blockFont(b)
	l.layout = ui.ShapeRichTextWith(spansOf(&l.proj, font, preedit, at), font, ui.TextShape{Width: max(e.width, 1)})
	e.Stats.Shapes++
	e.Stats.FrameShapes++
	_, l.h = l.layout.Size()
	if l.h <= 0 {
		l.h = font.Size * font.LineHeight
	}
}

// current returns row r as it shows now, and keeps its height up to date.
func (e *Editor) current(r int) *laid {
	var l *laid
	if e.preedit != "" && r == e.caretRow {
		l = e.composed()
	} else {
		l = e.form(r)
	}
	above, below := blockSpace(e.block(r))
	h := above + l.h + below
	v := &e.rows[r]
	if v.h != h {
		if v.h > 0 {
			e.hs.add(r, float64(h-v.h), 0)
		} else {
			e.hs.add(r, float64(h), -1)
		}
		v.h = h
	}
	return l
}

// composed returns the caret's row with the preedit spliced in. It has its own
// layout, made once per composition change, and nothing of it reaches the source.
func (e *Editor) composed() *laid {
	if e.preLaid != nil {
		return e.preLaid
	}
	base := e.form(e.caretRow)
	b := e.block(e.caretRow)
	at := e.opts.Projector.ToDisplay(&base.proj, clampTo(e.head, b.Range), contract.Downstream)
	l := &laid{key: base.key, proj: base.proj, base: base.base}
	u := utf16ByteOffset(base.display, at)
	l.display = base.display[:u] + e.preedit + base.display[u:]
	e.shape(l, b, e.preedit, at)
	e.preLaid = l
	return l
}

func (e *Editor) resetHeights() {
	hs := make([]float32, len(e.rows))
	for i := range e.rows {
		hs[i] = e.rows[i].h
	}
	e.hs.reset(hs)
	e.estimate()
}

func (e *Editor) estimate() {
	m, unknown := e.hs.totals()
	if n := len(e.rows) - unknown; n > 0 {
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
	return max(-padTop, e.hs.top(len(e.rows))+padBottom-float64(e.viewH))
}

// layoutView lays out the rows in view of a w×h box. The anchor row and its offset
// decide the scroll: a row above changing height moves nothing on screen.
func (e *Editor) layoutView(w, h float32) {
	column := max(min(w-2*sidePad, maxColumn), 1)
	if column != e.width {
		e.dropAllForms()
		for i := range e.rows {
			e.rows[i].h = 0
		}
		e.preLaid = nil
		e.resetHeights()
		e.width = column
	}
	e.viewW, e.viewH = w, h
	e.estimate()
	// Rows that changed form change height now, so that tops stay exact.
	for _, id := range e.touched {
		if r, ok := e.rowOf[id]; ok && e.rows[r].h > 0 {
			e.current(r)
		}
	}
	e.touched = e.touched[:0]
	e.place()
	if e.revealPending {
		e.revealPending = false
		e.reveal(h)
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

// reveal scrolls the caret into view. Estimates between may be off, so it measures
// again after each move; the second pass has the caret's row measured.
func (e *Editor) reveal(h float32) {
	align := e.revealAlign
	if e.typewriter {
		align = editorapi.AlignCenter
	}
	for range 3 {
		_, y, ch := e.caretRect()
		var d float32
		switch align {
		case editorapi.AlignTop:
			d = y - padTop
		case editorapi.AlignCenter:
			d = y + ch/2 - h/2
		default:
			switch {
			case y < 0:
				d = y - 8
			case y+ch > h:
				d = y + ch - h + 8
			}
		}
		if d > -0.5 && d < 0.5 {
			return
		}
		before := e.scroll
		e.setScroll(e.scroll + float64(d))
		e.place()
		if e.scroll == before {
			return
		}
	}
}

// place shapes the rows in view, from the anchor out.
func (e *Editor) place() {
	n := len(e.rows)
	e.anchorIdx = max(0, min(e.anchorIdx, n-1))
	e.current(e.anchorIdx)
	e.first = e.anchorIdx
	for e.first > 0 && e.hs.top(e.anchorIdx)+e.anchorOff-e.hs.top(e.first) < 0 {
		e.first--
		e.current(e.first)
	}
	e.scroll = max(-padTop, e.hs.top(e.anchorIdx)+e.anchorOff)
	p := e.first
	for ; p < n; p++ {
		e.current(p)
		if e.hs.top(p+1) >= e.scroll+float64(e.viewH) {
			break
		}
	}
	e.last = min(p, n-1)
}

// overscan shapes a few rows a screen around the view ahead of need, within a
// budget per frame, and asks for another frame while some remain.
func (e *Editor) overscan() {
	budget := overscanBudget
	e.moreOverscan = false
	screen := float64(e.viewH)
	wanted := func(i int) bool {
		if e.rows[i].cur != nil {
			return false
		}
		if budget == 0 {
			e.moreOverscan = true
			return false
		}
		budget--
		return true
	}
	for i := e.last + 1; i < len(e.rows) && e.hs.top(i) < e.scroll+2*screen; i++ {
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
	for i := range e.rows {
		if (i < lo || i > hi) && i != e.caretRow && i != e.anchorRow {
			e.dropForms(i)
		}
	}
}

func (e *Editor) dropForms(r int) {
	v := &e.rows[r]
	e.laidN -= len(v.forms)
	v.forms, v.cur = nil, nil
}

func (e *Editor) dropAllForms() {
	for i := range e.rows {
		e.dropForms(i)
	}
	e.preLaid = nil
}

// caretRect returns the caret in element coordinates: x, y and height.
func (e *Editor) caretRect() (x, y, h float32) {
	l := e.current(e.caretRow)
	idx := e.opts.Projector.ToDisplay(&l.proj, clampTo(e.head, e.block(e.caretRow).Range), contract.Downstream)
	if e.preedit != "" {
		idx += e.preSelected.End
	}
	return e.boundsAt(e.caretRow, l, idx)
}

func (e *Editor) boundsAt(i int, l *laid, idx int) (x, y, h float32) {
	r := l.layout.CaretAt(ui.TextCaretPosition{Index: idx, Affinity: e.aff})
	if l.display == "" {
		font := blockFont(e.block(i))
		r.H = font.Size * font.LineHeight
	}
	return e.contentX() + r.X, e.rowTextY(i) + r.Y, r.H
}

func (e *Editor) contentX() float32 { return (e.viewW - e.width) / 2 }

// rowTextY returns the element y of row i's first line.
func (e *Editor) rowTextY(i int) float32 {
	above, _ := blockSpace(e.block(i))
	return float32(e.hs.top(i)-e.scroll) + above
}
