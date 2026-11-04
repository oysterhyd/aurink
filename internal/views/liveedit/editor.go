package liveedit

import (
	"fmt"
	"maps"
	"time"

	"aurink/internal/contract"
	"aurink/internal/views/editorapi"
	"aurink/internal/views/theme"
	"github.com/egoist/mygo/ui"
)

// Page geometry and how far layouts reach, from 02 §3.
const (
	sidePad        = theme.PadSide
	padTop         = theme.PadTop
	padBottom      = theme.PadBottom
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
	// Renderer renders math and diagrams; nil shows their source.
	Renderer contract.Renderer
	// Theme is the first palette; nil uses Paper.
	Theme *theme.Tokens
	// DocDir returns the folder relative images resolve against, "" for none.
	DocDir func() string
	// Dispatcher brings loaded images back to the main thread.
	Dispatcher contract.Dispatcher
	// KeyLabel names a command's shortcut for the popovers; nil names none.
	KeyLabel func(contract.CommandID) string
	// OpenFile opens a file a link points to, or one dropped on the document; nil
	// leaves them alone.
	OpenFile func(path string)
	// Clip turns copied Markdown into the HTML fragment copied with it; nil
	// copies the Markdown only.
	Clip func(markdown string) (html string)
	// Clipboard reads and writes the clipboard's formats (adapters/clipboard);
	// nil copies and pastes plain text only.
	Clipboard contract.Clipboard
	// Converter turns pasted HTML and RTF into Markdown; nil pastes their text.
	Converter contract.PasteConverter
	// Assets imports pasted and dropped images next to the document; nil inserts
	// dropped images by their own path and ignores pasted pictures.
	Assets contract.Assets
	// DocPath returns the document's file, "" while it has none.
	DocPath func() string
	// SaveFirst saves a document without a file before images are imported next
	// to it; done reports whether it has one now.
	SaveFirst func(done func(ok bool))
	// OnError reports what failed in the background, as an image import.
	OnError func(error)
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
	// RailMarks counts the marks the structure rail placed in the last frame.
	RailMarks int
}

// revealKey says what a row's projection depends on in the selection, relative to
// the row's start so that it survives the row moving. caret is -1 without a caret.
type revealKey struct {
	caret, from, to int
	// A layout bakes in span colors and the lead paragraph's font.
	dark, lead bool
	// draft shows the row as a draft (draft.go).
	draft bool
	// listEnd: the row is the last of its outermost list, which keeps a
	// paragraph's spacing below it.
	listEnd bool
}

func (k revealKey) hidden() bool { return k.caret < 0 && k.from == k.to }

// laid is a row's projection and layout in one display form.
type laid struct {
	key     revealKey
	m       rowMetrics
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
	// A table cell's place: its column, how many columns its row has, and whether
	// that is a header row, all fixed between rebuilds.
	col, cols int
	header    bool
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
	labels        map[labelKey]*ui.TextLayout
	texts         map[textKey]*ui.TextLayout
	renders       map[renderKey]*renderState
	renderOff     bool
	slash         slashMenu
	// The table toolbar's size grid: open, and the size it shows lit.
	tableGrid          bool
	gridRows, gridCols int
	// tableMenuOpen is set while the toolbar's More menu shows: it takes the
	// focus from the document, and the toolbar holding it stays.
	tableMenuOpen bool
	// lineHeights caches lineHeight by font.
	lineHeights map[ui.Font]float32
	// draftAt is the start of the block shown as a draft (draft.go), -1 for
	// none; typedFrom is the caret block's kind before the keystroke being
	// typed, while typingNow.
	draftAt   int
	typedFrom contract.BlockKind
	typingNow bool
	// previews holds each block's last good rendering, which its preview
	// keeps while the source being typed renders.
	previews map[contract.BlockID]*renderState
	// frameNow is the frame's clock, which the work started between frames
	// keeps its times by: the paint compares them with its own.
	frameNow    time.Time
	t           *theme.Tokens
	images      map[string]*imageState
	codeState   uint64
	copiedBlock contract.BlockID
	copiedAt    time.Time
	tickBlock   contract.BlockID
	tickStart   time.Time
	hoverLink   string
	hoverNote   string
	hoverX      float32
	hoverY      float32

	mode       editorapi.Mode
	focusMode  bool
	ty         theme.Typography
	typewriter bool
	// revealMode and softBreak are Settings.Editor's Reveal and SoftBreak.
	revealMode contract.RevealMode
	softBreak  contract.SoftBreakMode
	find       []contract.Range
	findCur    int
	// reduceMotion is the shell's reading of Appearance.ReduceMotion.
	reduceMotion bool
	fmtBar       formatBar
	smoothCaret  bool
	caret        caretMotion
	lb           lightbox
	lastImage    imageHit

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
	e := &Editor{model: o.Model, opts: o, scroll: -padTop, anchorOff: -padTop, labels: map[labelKey]*ui.TextLayout{}, findCur: -1, t: o.Theme,
		ty: theme.DefaultTypography(), draftAt: -1}
	if e.t == nil {
		e.t = theme.For(theme.Options{})
	}
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
		HandleTextInput(e).HandleInput(e.input).Draw(e.paint).PopoverMenu(e.tableMenu)
	e.services, e.hasView = c.Services(), true
	e.frameNow = c.Now()
	// MyGo does not say where files were dropped, and the pointer's last place
	// may be from before the drag: dropped images go in at the caret.
	if files := box.DroppedFiles(); len(files) > 0 {
		e.dropFiles(files, e.head)
	}
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
	if !e.copiedAt.IsZero() {
		const shown = 1500 * time.Millisecond
		if left := shown - c.Now().Sub(e.copiedAt); left > 0 {
			c.After(left)
		} else {
			e.copiedAt, e.copiedBlock = time.Time{}, 0
		}
	}
	return box
}

// Ghost paints the document in another theme and takes no input: the old theme's
// copy during a dissolve (ADR-004). It keeps its own forms, by the key's dark bit.
func (e *Editor) Ghost(c *ui.Context, t *theme.Tokens) ui.Element {
	return ui.Box(c).Fill().Clip().Draw(func(p *ui.Painter, r ui.Rect) {
		live := e.t
		e.t = t
		e.layoutView(r.W, r.H)
		e.paintRows(p, r)
		e.t = live
	})
}

// SetTheme changes the palette. A change other than light and dark drops the forms,
// whose keys do not tell them apart.
func (e *Editor) SetTheme(t *theme.Tokens) {
	if e.t == t {
		return
	}
	if e.t.Dark == t.Dark && *e.t != *t {
		e.dropAllForms()
	}
	e.t = t
	e.invalidate()
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

// rowRange is where a caret can stand in row r: its block's range, or, for an
// empty leaf, the source its projection shows. A lone block marker not yet
// followed by a space (">", "-", "#") is such a leaf: it shows as text, and
// the caret goes after it, and over it, as over any text.
func (e *Editor) rowRange(r int) contract.Range {
	rg := e.block(r).Range
	if !rg.Empty() {
		return rg
	}
	for _, s := range e.form(r).proj.Segments {
		if s.Kind == contract.SegText {
			rg.Start, rg.End = min(rg.Start, s.Src.Start), max(rg.End, s.Src.End)
		}
	}
	return rg
}

func (e *Editor) onEvent(ev contract.DocEvent) {
	switch ev.Kind {
	case contract.DocTextChanged, contract.DocReplaced:
		e.text, e.ast = e.model.Text(), e.model.AST()
		e.Stats.Edits++
		e.syncRows(ev)
		e.refreshSelection()
		e.noteDraft()
		e.preLaid = nil
	case contract.DocSelectionChanged:
		e.refreshSelection()
		e.noteDraft()
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
		if b.Kind == contract.BlockTableCell {
			parent := e.ast.Block(b.Parent)
			v.cols, v.header = 0, parent.Header
			for range e.ast.Children(b.Parent) {
				v.cols++
			}
			if n := len(rows); n > 0 && rows[n-1].cols > 0 && e.ast.Block(rows[n-1].leaf).Parent == b.Parent {
				v.col = rows[n-1].col + 1
			}
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
	k := revealKey{caret: -1, dark: e.t.Dark, lead: e.isLead(r, b), listEnd: e.listEnd(r, b)}
	if e.mode == editorapi.ModeRead {
		return k
	}
	base := b.Lead.Start
	if r == e.caretRow {
		k.caret = clampTo(e.head, b.Range) - base
		k.draft = e.isDraft(r, b)
	}
	a, z := min(e.anchor, e.head), max(e.anchor, e.head)
	if from, to := max(a, b.Range.Start), min(z, b.Range.End); from < to {
		k.from, k.to = from-base, to-base
	}
	return k
}

// isLead reports the document's first paragraph, or the one right after its opening
// H1: it sets in the lead's larger size (§4.2).
func (e *Editor) isLead(r int, b *contract.Block) bool {
	if b.Kind != contract.BlockParagraph || r > 1 || e.rows[r].col != 0 {
		return false
	}
	if r == 0 {
		return e.ast.Block(b.Parent).Kind == contract.BlockRoot
	}
	prev := e.block(0)
	return prev.Kind == contract.BlockHeading && prev.Level == 1 && e.ast.Block(b.Parent).Kind == contract.BlockRoot
}

func (e *Editor) project(b *contract.Block, leaf int, k revealKey) contract.BlockProjection {
	e.Stats.Projects++
	rv := contract.Reveal{Mode: e.revealMode, Caret: -1, SoftBreak: e.softBreak, ObjectLangs: e.opts.ObjectLangs, Exts: e.opts.Exts}
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
	if k.draft {
		rv.Mode = contract.RevealBlock
		return e.draftOf(b, e.opts.Projector.Project(e.ast, e.text, leaf, rv))
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
		if l.proj.Sig == p.Sig && l.key.dark == k.dark && l.key.lead == k.lead {
			l.proj, l.key, l.base = p, k, b.Lead.Start
			v.cur = l
			return l
		}
	}
	l := &laid{key: k, proj: p, base: b.Lead.Start, display: e.opts.Projector.DisplayText(&p)}
	l.m = e.metricsFor(r, b, &l.proj, k.lead, k.listEnd)
	if k.draft && b.Kind == contract.BlockHeading {
		l.m.font.Weight = 700 // Typora's draft heading: body size, bold
	}
	e.shape(l, b, "", 0)
	if len(v.forms) >= maxForms {
		victim := 0
		for i, f := range v.forms {
			if !f.key.hidden() && f != v.cur {
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
	font := l.m.font
	l.layout = ui.ShapeRichTextWith(e.spansOf(l, b, preedit, at), font, ui.TextShape{Width: l.m.width})
	e.Stats.Shapes++
	e.Stats.FrameShapes++
	_, l.h = l.layout.Size()
	if l.h <= 0 {
		// An empty row is as tall as a line of its text will be, so the
		// first character typed moves neither the caret nor the rows below.
		l.h = e.lineHeight(font, l.m.width)
	}
}

// lineHeight is the height the shaper gives a line of text in font: more
// than Size times LineHeight where the face's own extent is taller.
func (e *Editor) lineHeight(font ui.Font, width float32) float32 {
	if h, ok := e.lineHeights[font]; ok {
		return h
	}
	_, h := ui.ShapeText("x", font, width).Size()
	if h <= 0 {
		h = font.Size * font.LineHeight
	}
	if e.lineHeights == nil {
		e.lineHeights = map[ui.Font]float32{}
	}
	e.lineHeights[font] = h
	return h
}

// current returns row r as it shows now, and keeps its height up to date.
func (e *Editor) current(r int) *laid {
	var l *laid
	if e.preedit != "" && r == e.caretRow {
		l = e.composed()
	} else {
		l = e.form(r)
	}
	v := &e.rows[r]
	if v.cols > 0 {
		e.measureCells(r - v.col)
		return l
	}
	e.setHeight(r, l.m.above+l.h+l.m.below)
	return l
}

func (e *Editor) setHeight(r int, h float32) {
	v := &e.rows[r]
	if v.h != h {
		var du int32
		if v.h == 0 {
			du = -1
		}
		e.hs.add(r, float64(max(h, 0)-max(v.h, 0)), du)
		v.h = h
	}
}

// measureCells gives a table row's band to its first cell. The others are measured
// at nothing, which -1 says, since 0 means not measured.
func (e *Editor) measureCells(first int) {
	var band float32
	n := e.rows[first].cols
	for k := first; k < first+n && k < len(e.rows); k++ {
		l := e.form(k)
		if k == e.caretRow && e.preLaid != nil {
			l = e.preLaid
		}
		band = max(band, l.m.above+l.h+l.m.below)
		if k > first {
			e.setHeight(k, -1)
		}
	}
	e.setHeight(first, band)
}

// composed returns the caret's row with the preedit spliced in. It has its own
// layout, made once per composition change, and nothing of it reaches the source.
func (e *Editor) composed() *laid {
	if e.preLaid != nil {
		return e.preLaid
	}
	base := e.form(e.caretRow)
	at := e.opts.Projector.ToDisplay(&base.proj, clampTo(e.head, e.rowRange(e.caretRow)), contract.Downstream)
	l := &laid{key: base.key, m: base.m, proj: base.proj, base: base.base}
	u := utf16ByteOffset(base.display, at)
	l.display = base.display[:u] + e.preedit + base.display[u:]
	e.shape(l, e.block(e.caretRow), e.preedit, at)
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

// relayAll drops every row's form and height, as a new column or type needs.
func (e *Editor) relayAll() {
	e.dropAllForms()
	for i := range e.rows {
		e.rows[i].h = 0
	}
	e.preLaid = nil
	e.resetHeights()
}

func (e *Editor) maxScroll() float64 {
	return max(-padTop, e.hs.top(len(e.rows))+padBottom-float64(e.viewH))
}

// layoutView lays out the rows in view of a w×h box. The anchor row and its offset
// decide the scroll: a row above changing height moves nothing on screen.
func (e *Editor) layoutView(w, h float32) {
	column := max(min(w-2*sidePad, e.ty.Column), 1)
	if column != e.width {
		e.relayAll()
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
			if e.typewriter {
				d = y + ch/2 - h*typewriterLine
			}
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
	e.first -= e.rows[e.first].col
	e.scroll = max(-padTop, e.hs.top(e.anchorIdx)+e.anchorOff)
	p := e.first
	for ; p < n; p++ {
		e.current(p)
		if e.hs.top(p+1) >= e.scroll+float64(e.viewH) {
			break
		}
	}
	e.last = min(p, n-1)
	// A table row shows whole: its cells share one band.
	if v := e.rows[e.last]; v.cols > 0 {
		for k := e.last + 1; k < min(e.last-v.col+v.cols, n); k++ {
			e.current(k)
			e.last = k
		}
	}
}

// rowTop returns a row's top in the document: a cell sits in its table row's band.
func (e *Editor) rowTop(r int) float64 { return e.hs.top(r - e.rows[r].col) }

// RowText returns where a row's text starts in the element, for tests.
func (e *Editor) RowText(r int) ui.Point {
	return ui.Point{X: e.contentX() + e.form(r).m.x, Y: e.rowTextY(r)}
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
	idx := e.opts.Projector.ToDisplay(&l.proj, clampTo(e.head, e.rowRange(e.caretRow)), contract.Downstream)
	if e.preedit != "" {
		idx += e.preSelected.End
	}
	return e.boundsAt(e.caretRow, l, idx)
}

func (e *Editor) boundsAt(i int, l *laid, idx int) (x, y, h float32) {
	r := l.layout.CaretAt(ui.TextCaretPosition{Index: idx, Affinity: e.aff})
	if l.display == "" {
		r.H = e.lineHeight(l.m.font, l.m.width)
	}
	return e.contentX() + l.m.x + r.X, e.rowTextY(i) + r.Y, r.H
}

func (e *Editor) contentX() float32 { return (e.viewW - e.width) / 2 }

// rowTextY returns the element y of row i's first line.
func (e *Editor) rowTextY(i int) float32 {
	return float32(e.rowTop(i)-e.scroll) + e.form(i).m.above
}

// RowHeight returns a row's measured height, for tests.
func (e *Editor) RowHeight(r int) float32 { return e.rows[r].h }

// ColumnWidth returns the text column's width, for tests.
func (e *Editor) ColumnWidth() float32 { return e.width }

// LinkHint returns the link target shown under the pointer, for tests.
func (e *Editor) LinkHint() string { return e.hoverLink }

// ImageFailed reports an image known not to load, for tests.
func (e *Editor) ImageFailed(source string) bool {
	st := e.images[source]
	return st != nil && st.failed
}

// TopOffset returns the source offset of the row at the top of the view.
func (e *Editor) TopOffset() int {
	if e.anchorIdx >= len(e.rows) {
		return 0
	}
	return e.block(e.anchorIdx).Lead.Start
}

// ScrollToOffset puts the row of a source offset at the top, leaving the caret.
func (e *Editor) ScrollToOffset(off int) {
	if len(e.rows) == 0 {
		return
	}
	e.anchorIdx, e.anchorOff = e.rowAt(off), 0
	if e.anchorIdx == 0 {
		e.anchorOff = -padTop
	}
	e.revealPending = false
	e.invalidate()
}
