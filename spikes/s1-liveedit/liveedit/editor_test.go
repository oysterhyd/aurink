package liveedit

import (
	"math/rand"
	"strings"
	"testing"
	"time"

	"aurink.local/spikes/contract-rc/contract"
	"github.com/egoist/mygo/ui"
)

// harness runs an editor in a Tester.
type harness struct {
	t  *testing.T
	e  *Editor
	tt *ui.Tester
}

func open(t *testing.T, src string) *harness {
	t.Helper()
	e := New(src)
	tt := ui.NewTester(func(c *ui.Context) { e.View(c) }, 1000, 700)
	tt.Frame()
	return &harness{t, e, tt}
}

func (h *harness) caretAt(off int) {
	h.t.Helper()
	h.e.setSelection(off, off)
	h.tt.Frame()
}

func (h *harness) wantSource(want string) {
	h.t.Helper()
	if got := h.e.Source(); got != want {
		h.t.Fatalf("source\n got %q\nwant %q", got, want)
	}
}

func (h *harness) wantCaret(want int) {
	h.t.Helper()
	if a, c := h.e.SourceSelection(); a != want || c != want {
		h.t.Fatalf("selection %d..%d, want caret %d, in %q", a, c, want, h.e.Source())
	}
}

func (h *harness) wantSel(anchor, caret int) {
	h.t.Helper()
	if a, c := h.e.SourceSelection(); a != anchor || c != caret {
		h.t.Fatalf("selection %d..%d, want %d..%d", a, c, anchor, caret)
	}
}

// ---------- Typing ----------

func TestTypeAppendsToSource(t *testing.T) {
	h := open(t, "hello")
	h.caretAt(5)
	h.tt.Type(" world")
	h.wantSource("hello world")
	h.wantCaret(11)
}

func TestTypeCJKAndEmojiInMiddle(t *testing.T) {
	h := open(t, "ab")
	h.caretAt(1)
	h.tt.Type("中😀")
	h.wantSource("a中😀b")
	h.wantCaret(1 + len("中😀"))
}

func TestTypeReplacesSelection(t *testing.T) {
	h := open(t, "hello world")
	h.e.setSelection(0, 5)
	h.tt.Frame()
	h.tt.Type("X")
	h.wantSource("X world")
	h.wantCaret(1)
}

func TestEnterSplitsBlockAndTypingContinues(t *testing.T) {
	h := open(t, "abcd\n\nnext\n")
	h.caretAt(2)
	h.tt.Key(0, ui.KeyEnter)
	h.wantSource("ab\n\ncd\n\nnext\n")
	h.wantCaret(4)
	h.tt.Type("X")
	h.wantSource("ab\n\nXcd\n\nnext\n")
}

func TestEnterOnEmptyBlockAndAtEnd(t *testing.T) {
	h := open(t, "a")
	h.caretAt(1)
	h.tt.Key(0, ui.KeyEnter)
	h.wantSource("a\n\n")
	h.wantCaret(3)
	if h.e.BlockCount() != 2 {
		t.Fatalf("blocks %d, want a and an empty one", h.e.BlockCount())
	}
	h.tt.Key(0, ui.KeyEnter)
	h.wantSource("a\n\n\n\n")
	h.wantCaret(5)
	h.tt.Type("z")
	h.wantSource("a\n\n\n\nz")
}

func TestEnterKeepsDocumentsLineEnding(t *testing.T) {
	h := open(t, "ab\r\n\r\ncd\r\n")
	h.caretAt(1)
	h.tt.Key(0, ui.KeyEnter)
	h.wantSource("a\r\n\r\nb\r\n\r\ncd\r\n")
}

func TestBackspaceWithinAndAcrossBlocks(t *testing.T) {
	h := open(t, "ab\n\ncd\n")
	h.caretAt(5)
	h.tt.Key(0, ui.KeyBackspace)
	h.wantSource("ab\n\nd\n")
	h.wantCaret(4)
	h.tt.Key(0, ui.KeyBackspace)
	h.wantSource("abd\n") // the terminator and separator go at once
	h.wantCaret(2)
}

func TestBackspaceDeletesWholeSurrogatePair(t *testing.T) {
	h := open(t, "a😀b")
	h.caretAt(5)
	h.tt.Key(0, ui.KeyBackspace)
	h.wantSource("ab")
	h.wantCaret(1)
}

func TestDeleteForwardJoinsBlocks(t *testing.T) {
	h := open(t, "ab\n\ncd")
	h.caretAt(1)
	h.tt.Key(0, ui.KeyDelete)
	h.wantSource("a\n\ncd")
	h.tt.Key(0, ui.KeyDelete)
	h.wantSource("acd")
}

func TestUndoRestoresTextAndSelectionAndCoalesces(t *testing.T) {
	h := open(t, "x")
	h.caretAt(1)
	h.tt.Type("a")
	h.tt.Type("b")
	h.tt.Type("c")
	h.tt.Key(ui.Cmd, ui.KeyZ)
	h.wantSource("x")
	h.wantCaret(1)
	h.tt.Key(ui.Cmd|ui.Shift, ui.KeyZ)
	h.wantSource("xabc")
	h.wantCaret(4)
}

// ---------- IME: preedit never reaches the source ----------

func TestComposeDoesNotWriteSource(t *testing.T) {
	h := open(t, "ab")
	h.caretAt(1)
	h.tt.Compose("ni", 2)
	h.wantSource("ab")
	if h.e.Preedit() != "ni" {
		t.Fatalf("preedit %q", h.e.Preedit())
	}
	edits := h.e.Stats.Edits
	h.tt.Compose("nihao", 5)
	h.wantSource("ab")
	if h.e.Stats.Edits != edits {
		t.Fatal("composing made an edit")
	}
}

func TestComposeCommitIsOneEdit(t *testing.T) {
	h := open(t, "ab")
	h.caretAt(1)
	h.tt.Compose("ni", 2)
	h.tt.Compose("nihao", 5)
	edits := h.e.Stats.Edits
	h.tt.Type("你好")
	h.wantSource("a你好b")
	h.wantCaret(1 + len("你好"))
	if h.e.Stats.Edits != edits+1 || h.e.Preedit() != "" {
		t.Fatalf("edits +%d preedit %q", h.e.Stats.Edits-edits, h.e.Preedit())
	}
}

func TestComposeCancelLeavesNothing(t *testing.T) {
	h := open(t, "ab")
	h.caretAt(1)
	h.tt.Compose("nihao", 5)
	h.tt.Compose("", 0)
	h.wantSource("ab")
	h.wantCaret(1)
	if h.e.Preedit() != "" {
		t.Fatal("preedit stayed")
	}
	if len(h.e.undo) != 0 {
		t.Fatal("a cancelled composition left a history step")
	}
}

func TestCommitUndoIsOneStepAndSeparateFromTyping(t *testing.T) {
	h := open(t, "")
	h.tt.Type("a")
	h.tt.Compose("ni", 2)
	h.tt.Compose("nihao", 5)
	h.tt.Type("你好")
	h.wantSource("a你好")
	h.tt.Key(ui.Cmd, ui.KeyZ)
	h.wantSource("a")
	h.tt.Key(ui.Cmd, ui.KeyZ)
	h.wantSource("")
}

func TestComposeOverSelectionUndoesAsOneStep(t *testing.T) {
	h := open(t, "one\n\ntwo\n\nthree")
	h.e.setSelection(1, 6) // across blocks, to just after the t
	h.tt.Frame()
	h.tt.Compose("ni", 2)
	h.wantSource("owo\n\nthree")
	h.tt.Type("你")
	h.wantSource("o你wo\n\nthree")
	h.tt.Key(ui.Cmd, ui.KeyZ)
	h.wantSource("one\n\ntwo\n\nthree")
	h.wantSel(1, 6)
}

func TestComposeInBlockWithHiddenMarkers(t *testing.T) {
	h := open(t, "first\n\nsay **bold** now\n")
	// The second block hides its markers until the caret enters it.
	if l := h.e.current(1); l.display != "say bold now" {
		t.Fatalf("hidden form %q", l.display)
	}
	off := strings.Index(h.e.Source(), "bold") + 2
	h.caretAt(off)
	l := h.e.current(1)
	if l.display != "say **bold** now" {
		t.Fatalf("revealed form %q", l.display)
	}
	h.tt.Compose("zi", 2)
	if l := h.e.current(1); l.display != "say **bozild** now" {
		t.Fatalf("composed form %q", l.display)
	}
	h.wantSource("first\n\nsay **bold** now\n")
	h.tt.Type("字")
	h.wantSource("first\n\nsay **bo字ld** now\n")
}

func TestIMEDocumentIsActiveBlockDisplay(t *testing.T) {
	h := open(t, "first\n\nsecond 中\n\nthird")
	h.caretAt(7 + 3)
	s, r := h.e.TextForRange(ui.TextInputRange{Start: 0, End: 1000})
	if s != "second 中" || r.Start != 0 || r.End != 8 {
		t.Fatalf("doc %q %v", s, r)
	}
	sel := h.e.Selection()
	if sel.Range.Start != 3 || sel.Range.End != 3 {
		t.Fatalf("selection %v", sel)
	}
	h.tt.Compose("ni", 2)
	s, _ = h.e.TextForRange(ui.TextInputRange{Start: 0, End: 1000})
	if s != "secniond 中" {
		t.Fatalf("doc with preedit %q", s)
	}
	m, ok := h.e.MarkedRange()
	if !ok || m.Start != 3 || m.End != 5 {
		t.Fatalf("marked %v %v", m, ok)
	}
}

func TestCandidateFollowsCaretAcrossBlocks(t *testing.T) {
	h := open(t, "first block\n\nsecond block\n")
	h.caretAt(3)
	r1, ok := h.tt.TextCaret()
	if !ok {
		t.Fatal("no IME caret")
	}
	h.caretAt(strings.Index(h.e.Source(), "second") + 3)
	r2, _ := h.tt.TextCaret()
	if r2.Y <= r1.Y {
		t.Fatalf("IME caret did not move down: %v -> %v", r1, r2)
	}
	h.tt.Compose("nihao", 5)
	r3, _ := h.tt.TextCaret()
	if r3.Y < r2.Y-1 || r3.Y > r2.Y+1 {
		t.Fatalf("IME caret left the line: %v -> %v", r2, r3)
	}
	b, _, vis := h.e.BoundsForRange(ui.TextInputRange{Start: 3, End: 5})
	if !vis || b.W <= 1 {
		t.Fatalf("preedit bounds %v %v", b, vis)
	}
}

func TestFocusLossCommitsPreedit(t *testing.T) {
	h := open(t, "ab")
	h.caretAt(1)
	h.tt.Compose("ni", 2)
	h.e.UnmarkText()
	h.wantSource("anib")
	h.wantCaret(3)
}

// ---------- Moving across blocks ----------

func TestArrowsCrossBlocks(t *testing.T) {
	h := open(t, "ab\n\ncd")
	h.caretAt(2)
	h.tt.Key(0, ui.KeyRight)
	h.wantCaret(4)
	h.tt.Key(0, ui.KeyLeft)
	h.wantCaret(2)
	h.tt.Key(0, ui.KeyLeft)
	h.wantCaret(1)
}

func TestArrowsStepOverSurrogatesAndCRLF(t *testing.T) {
	h := open(t, "😀a\r\nb")
	h.caretAt(0)
	h.tt.Key(0, ui.KeyRight)
	h.wantCaret(4)
	h.tt.Key(0, ui.KeyRight)
	h.wantCaret(5)
	h.tt.Key(0, ui.KeyRight)
	h.wantCaret(7) // over the soft break's two bytes
}

func TestUpDownKeepStickyXAcrossBlocks(t *testing.T) {
	long := strings.Repeat("0123456789", 3)
	h := open(t, long+"\n\nab\n\n"+long+"\n")
	h.caretAt(20)
	x0, _, _ := h.e.caretRect()
	h.tt.Key(0, ui.KeyDown) // to the short block, whose end is left of x
	_, c := h.e.SourceSelection()
	if c != len(long)+2+2 {
		t.Fatalf("caret %d not at end of short block", c)
	}
	h.tt.Key(0, ui.KeyDown) // and past it: x comes back
	x2, _, _ := h.e.caretRect()
	if d := x2 - x0; d < -8 || d > 8 {
		t.Fatalf("sticky x lost: %v -> %v", x0, x2)
	}
	h.tt.Key(0, ui.KeyLeft) // a horizontal move forgets it
	if h.e.hasSticky {
		t.Fatal("sticky x kept after a horizontal move")
	}
}

func TestDownMovesThroughWrappedLinesBeforeLeavingBlock(t *testing.T) {
	long := strings.Repeat("word ", 400)
	h := open(t, long+"\n\nnext")
	h.caretAt(0)
	lines := len(h.e.current(0).layout.Lines())
	if lines < 3 {
		t.Fatalf("only %d lines", lines)
	}
	for range lines - 1 {
		h.tt.Key(0, ui.KeyDown)
		if h.e.caretBlock != 0 {
			t.Fatal("left the block early")
		}
	}
	h.tt.Key(0, ui.KeyDown)
	if h.e.caretBlock != 1 {
		t.Fatal("did not reach the next block")
	}
}

func TestUpFromFirstLineGoesToStart(t *testing.T) {
	h := open(t, "abc\n\ndef")
	h.caretAt(2)
	h.tt.Key(0, ui.KeyUp)
	h.wantCaret(0)
}

func TestHomeEnd(t *testing.T) {
	h := open(t, "abc def\n\nx")
	h.caretAt(3)
	h.tt.Key(0, ui.KeyEnd)
	h.wantCaret(7)
	h.tt.Key(0, ui.KeyHome)
	h.wantCaret(0)
}

// ---------- Selection ----------

func TestShiftExtendsAcrossBlocksInSourceBytes(t *testing.T) {
	h := open(t, "ab\n\ncd")
	h.caretAt(1)
	h.tt.Key(ui.Shift, ui.KeyRight)
	h.wantSel(1, 2)
	h.tt.Key(ui.Shift, ui.KeyRight)
	h.wantSel(1, 4)
	h.tt.Key(ui.Shift, ui.KeyDown)
	_, c := h.e.SourceSelection()
	if c < 4 {
		t.Fatalf("caret %d", c)
	}
	h.tt.Key(0, ui.KeyLeft)
	h.wantCaret(1)
}

func TestSelectionAcrossBlocksDeletesAndUndoes(t *testing.T) {
	h := open(t, "one **b** x\n\ntwo\n\nthree")
	a := strings.Index(h.e.Source(), "b**")
	z := strings.Index(h.e.Source(), "ree")
	h.e.setSelection(a, z)
	h.tt.Frame()
	h.tt.Key(0, ui.KeyBackspace)
	h.wantSource("one **ree")
	h.tt.Key(ui.Cmd, ui.KeyZ)
	h.wantSource("one **b** x\n\ntwo\n\nthree")
	h.wantSel(a, z)
}

func TestCopyIsSourceAndPasteIsByteReplacement(t *testing.T) {
	h := open(t, "say **bold** now\n\nend")
	h.e.setSelection(0, 12)
	h.tt.Frame()
	h.tt.Key(ui.Cmd, ui.KeyC)
	if got := h.tt.Clipboard(); got != "say **bold**" {
		t.Fatalf("clipboard %q", got)
	}
	h.caretAt(len(h.e.Source()))
	h.tt.SetClipboard("*p*")
	h.tt.Key(ui.Cmd, ui.KeyV)
	h.wantSource("say **bold** now\n\nend*p*")
}

func TestSelectAll(t *testing.T) {
	src := "ab\n\ncd\n"
	h := open(t, src)
	h.tt.Key(ui.Cmd, ui.KeyA)
	a, c := h.e.SourceSelection()
	if a != 0 || c != 6 { // the final terminator is no caret position
		t.Fatalf("selection %d..%d", a, c)
	}
}

// ---------- Pointer ----------

func TestClickPlacesCaretInClickedBlock(t *testing.T) {
	src := "first line\n\nsecond line\n\nthird line\n"
	h := open(t, src)
	_, y := h.pointIn(2)
	h.tt.ClickAt(h.e.contentX()+1, y)
	if h.e.caretBlock != 2 {
		t.Fatalf("caret block %d", h.e.caretBlock)
	}
	h.wantCaret(strings.Index(src, "third"))
}

func (h *harness) pointIn(block int) (x, y float32) {
	h.t.Helper()
	l := h.e.current(block)
	return h.e.contentX(), h.e.blockTextY(block) + l.h/2
}

func TestClickInHiddenFormLandsOnTheSameText(t *testing.T) {
	src := "active\n\nsay **bold** now\n"
	h := open(t, src)
	l := h.e.current(1)
	r := l.layout.Caret(strings.Index(l.display, "ld"))
	_, y := h.pointIn(1)
	h.tt.ClickAt(h.e.contentX()+r.X+0.5, y)
	h.wantCaret(strings.Index(src, "ld"))
	if !h.e.current(1).proj.Revealed {
		t.Fatal("clicked block did not reveal")
	}
}

func TestShiftClickAndDragSelect(t *testing.T) {
	src := "first\n\nsecond\n"
	h := open(t, src)
	h.caretAt(0)
	_, y := h.pointIn(1)
	h.tt.ClickAtWith(ui.Shift, h.e.contentX()+1, y)
	h.wantSel(0, strings.Index(src, "second"))

	_, y0 := h.pointIn(0)
	h.tt.Press(h.e.contentX()+1, y0)
	h.tt.Move(h.e.contentX()+1, y)
	h.tt.Release(h.e.contentX()+1, y)
	a, c := h.e.SourceSelection()
	if a != 0 || c < 7 {
		t.Fatalf("drag %d..%d", a, c)
	}
}

// ---------- Projection ----------

func TestRevealOnlyTheCaretsBlock(t *testing.T) {
	src := "# Head\n\n**a** *b* `c`\n\nplain **x**\n"
	h := open(t, src)
	if d := h.e.current(0).display; d != "# Head" {
		t.Fatalf("active heading %q", d)
	}
	if d := h.e.current(1).display; d != "a b c" {
		t.Fatalf("inactive %q", d)
	}
	h.caretAt(strings.Index(src, "plain"))
	if d := h.e.current(0).display; d != "Head" {
		t.Fatalf("heading after leaving %q", d)
	}
	if d := h.e.current(2).display; d != "plain **x**" {
		t.Fatalf("active %q", d)
	}
}

func TestProjectionInvariants(t *testing.T) {
	src := "# T **b** ##\n\nsay **bo*x*ld** `c` 中😀\r\nnext *i*\r\n\r\n\n\n  indented\n\n[d]: http://x\n\n- a\n- b\n\n```\ncode\n```\n"
	d := newDocument(src)
	pos := 0
	for i := range d.blocks {
		b := &d.blocks[i]
		if b.Start != pos {
			t.Fatalf("block %d starts at %d, want %d", i, b.Start, pos)
		}
		pos = b.End
		for _, revealed := range []bool{false, true} {
			checkProjection(t, d, i, revealed)
		}
	}
	if pos != len(src) {
		t.Fatalf("partition ends at %d of %d", pos, len(src))
	}
}

func checkProjection(t *testing.T, d *document, i int, revealed bool) {
	t.Helper()
	b := &d.blocks[i]
	p := project(d, i, revealed)
	at := b.Start
	src := d.src()
	for _, s := range p.Segments {
		if s.Src.Start != at {
			t.Fatalf("block %d: gap at %d: %+v", i, at, p.Segments)
		}
		at = s.Src.End
		if transparent(&s) && s.Display != src[s.Src.Start:s.Src.End] {
			t.Fatalf("display %q != source %q", s.Display, src[s.Src.Start:s.Src.End])
		}
		if s.Kind == contract.SegHidden && s.Display != "" {
			t.Fatalf("hidden with display %q", s.Display)
		}
		if !revealed && s.Kind == contract.SegMarker {
			t.Fatalf("marker in hidden form: %+v", s)
		}
	}
	if at != b.End {
		t.Fatalf("block %d: segments end at %d, want %d", i, at, b.End)
	}
	prev := 0
	for off := b.Start; off <= b.End; off++ {
		for _, aff := range []contract.Affinity{contract.Upstream, contract.Downstream} {
			u := toDisplay(&p, off, aff)
			if got := toSource(&p, u, aff); got != snap(&p, off, aff) {
				t.Fatalf("round trip %d: %d != %d", off, got, snap(&p, off, aff))
			}
			if s := snap(&p, off, aff); snap(&p, s, aff) != s {
				t.Fatalf("snap of %d not stable: %d", off, s)
			}
		}
		u := toDisplay(&p, off, contract.Upstream)
		if u < prev {
			t.Fatalf("toDisplay not monotonic at %d", off)
		}
		prev = u
	}
}

func TestSigChangesWithContentAndForm(t *testing.T) {
	d := newDocument("say **b** x\n\nsay **c** x\n")
	if project(d, 0, false).Sig == project(d, 0, true).Sig {
		t.Fatal("form did not change Sig")
	}
	if project(d, 0, false).Sig == project(d, 1, false).Sig {
		t.Fatal("content did not change Sig")
	}
	a, b := project(d, 0, true), project(d, 0, true)
	if a.Sig != b.Sig {
		t.Fatal("Sig not deterministic")
	}
}

func TestBlockIDs(t *testing.T) {
	h := open(t, "one\n\ntwo\n\nthree\n")
	ids := func() []contract.BlockID {
		var out []contract.BlockID
		for _, b := range h.e.d.blocks {
			out = append(out, b.ID)
		}
		return out
	}
	before := ids()
	h.caretAt(6)
	h.tt.Type("x")
	after := ids()
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("typing changed IDs: %v -> %v", before, after)
		}
	}
	h.tt.Key(0, ui.KeyEnter)
	after = ids()
	if after[0] != before[0] || after[len(after)-1] != before[2] || len(after) != 4 {
		t.Fatalf("split: %v -> %v", before, after)
	}
}

// ---------- Lossless by construction ----------

// TestRandomEditsMatchStringModel drives keys, typing and compositions at random
// and checks the source against plain string replacement after every step.
func TestRandomEditsMatchStringModel(t *testing.T) {
	for seed := int64(1); seed <= 3; seed++ {
		r := rand.New(rand.NewSource(seed))
		src := GenerateBlocks(20)
		h := open(t, src)
		model := src
		for step := range 400 {
			a, c := h.e.SourceSelection()
			lo, hi := min(a, c), max(a, c)
			switch r.Intn(7) {
			case 0, 1:
				s := []string{"x", "中", "*", "`", "# ", "😀"}[r.Intn(6)]
				h.tt.Type(s)
				model = model[:lo] + s + model[hi:]
			case 2:
				// Composing removes a selection, and writes nothing else.
				model = model[:lo] + model[hi:]
				h.tt.Compose("pre", 3)
				if h.e.Source() != model {
					t.Fatalf("seed %d step %d: preedit reached source", seed, step)
				}
				h.tt.Type("提")
				model = model[:lo] + "提" + model[lo:]
			case 3:
				h.tt.Key(0, ui.KeyEnter)
				model = model[:lo] + "\n\n" + model[hi:]
			case 4:
				off := r.Intn(len(model) + 1)
				h.e.setSelection(off, off)
				h.tt.Frame()
			case 5:
				k := []ui.Key{ui.KeyLeft, ui.KeyRight, ui.KeyUp, ui.KeyDown, ui.KeyHome, ui.KeyEnd}[r.Intn(6)]
				h.tt.Key(0, k)
			case 6:
				before := h.e.Source()
				h.tt.Key(0, ui.KeyBackspace)
				model = h.e.Source()
				if len(model) > len(before) {
					t.Fatalf("backspace grew the source")
				}
				if lo != hi && model != before[:lo]+before[hi:] {
					t.Fatalf("seed %d step %d: selection delete wrong", seed, step)
				}
			}
			if got := h.e.Source(); got != model {
				t.Fatalf("seed %d step %d:\n got %q\nwant %q", seed, step, got, model)
			}
			checkBlocks(t, h.e)
		}
		// Undoing everything gets back to the original bytes.
		for len(h.e.undo) > 0 {
			h.e.command("undo")
		}
		h.wantSource(src)
	}
}

func checkBlocks(t *testing.T, e *Editor) {
	t.Helper()
	pos := 0
	seen := map[contract.BlockID]bool{}
	for i := range e.d.blocks {
		b := &e.d.blocks[i]
		if b.Start != pos || b.CStart < b.Start || b.CEnd < b.CStart || b.End < b.CEnd {
			t.Fatalf("block %d bad: %+v at %d", i, *b, pos)
		}
		if seen[b.ID] {
			t.Fatalf("duplicate ID %d", b.ID)
		}
		seen[b.ID] = true
		pos = b.End
	}
	if pos != len(e.Source()) {
		t.Fatalf("partition ends at %d of %d", pos, len(e.Source()))
	}
	if len(e.views) != len(e.d.blocks) {
		t.Fatalf("views %d blocks %d", len(e.views), len(e.d.blocks))
	}
	c := &e.d.blocks[e.caretBlock]
	if e.caret < c.CStart || e.caret > c.CEnd {
		t.Fatalf("caret %d outside content of block %+v", e.caret, *c)
	}
}

func TestUntouchedBytesSurviveEditing(t *testing.T) {
	// Mixed endings, odd emphasis styles, no final newline: all untouched.
	src := "__a__\r\n\r\n*  x\n\n+ y\n\nSetext\n===\n\n[l]: /u 'T'\n\ntail  "
	h := open(t, src)
	h.caretAt(len(src))
	h.tt.Type("!")
	h.wantSource(src + "!")
	h.tt.Key(0, ui.KeyBackspace)
	h.wantSource(src)
}

// ---------- Virtual scrolling ----------

func TestFenwick(t *testing.T) {
	var h heights
	h.reset([]float32{10, 0, 30, 0})
	h.est = 5
	if got := h.top(4); got != 50 {
		t.Fatalf("total %v", got)
	}
	if got := h.top(2); got != 15 {
		t.Fatalf("top(2) %v", got)
	}
	if got := h.at(14); got != 1 {
		t.Fatalf("at(14) %d", got)
	}
	if got := h.at(15); got != 2 {
		t.Fatalf("at(15) %d", got)
	}
	if got := h.at(1e9); got != 3 {
		t.Fatalf("at past end %d", got)
	}
	h.add(1, 20, -1)
	if got := h.top(2); got != 30 {
		t.Fatalf("after measure %v", got)
	}
	r := rand.New(rand.NewSource(1))
	hs := make([]float32, 500)
	h.reset(hs)
	h.est = 3
	model := make([]float64, 500)
	for i := range model {
		model[i] = 3
	}
	for range 2000 {
		i := r.Intn(500)
		x := float64(1 + r.Intn(50))
		if hs[i] > 0 {
			h.add(i, x-float64(hs[i]), 0)
		} else {
			h.add(i, x, -1)
		}
		hs[i], model[i] = float32(x), x
		q := r.Intn(501)
		sum := 0.0
		for _, v := range model[:q] {
			sum += v
		}
		if got := h.top(q); got != sum {
			t.Fatalf("top(%d) %v want %v", q, got, sum)
		}
	}
}

func TestScrollDoesNotReshapeWholeScreen(t *testing.T) {
	h := open(t, GenerateBlocks(10000))
	first, last := h.e.VisibleBlocks()
	visible := last - first + 1
	t.Logf("10k blocks: %d bytes, %d visible, %d shaped after open", len(h.e.Source()), visible, h.e.Stats.Shapes)
	if h.e.BlockCount() != 10000 {
		t.Fatalf("blocks %d", h.e.BlockCount())
	}
	h.e.ResetStats()
	const notch = 120
	worst := 0
	for range 200 {
		before := h.e.Stats.Shapes
		h.tt.Scroll(500, 300, 0, notch)
		worst = max(worst, h.e.Stats.Shapes-before)
	}
	s := h.e.Stats
	t.Logf("[实测] 200 notches of %d DIP: scroll=%.0f frames=%d shapes=%d perNotch=%.1f worstNotch=%d worstFrame=%d", notch, h.e.Scroll(), s.Frames, s.Shapes, float64(s.Shapes)/200, worst, s.MaxFrameShapes)
	if s.MaxFrameShapes > overscanBudget+visible/2 {
		t.Fatalf("a frame shaped %d blocks with %d visible", s.MaxFrameShapes, visible)
	}
	if s.Shapes > 200*(overscanBudget+6) {
		t.Fatalf("%d shapes", s.Shapes)
	}
	if h.tt.FramePending() {
		t.Fatal("frames continue after scrolling stopped")
	}
}

func TestScrollBackReshapesNothing(t *testing.T) {
	h := open(t, GenerateBlocks(10000))
	for range 20 {
		h.tt.Scroll(500, 300, 0, 120)
	}
	h.e.ResetStats()
	for range 20 {
		h.tt.Scroll(500, 300, 0, -60)
	}
	if h.e.Stats.Shapes != 0 {
		t.Fatalf("scrolling back shaped %d blocks", h.e.Stats.Shapes)
	}
}

func TestIdleIsZeroFrames(t *testing.T) {
	h := open(t, GenerateBlocks(200))
	h.tt.SetFocused(false)
	h.tt.Frame()
	h.tt.Advance(time.Minute)
	if h.tt.FramePending() {
		t.Fatal("idle window asks for frames")
	}
	frames := h.e.Stats.Frames
	h.tt.Advance(time.Minute)
	if h.e.Stats.Frames != frames {
		t.Fatalf("%d frames while idle", h.e.Stats.Frames-frames)
	}
}

func TestLayoutsAreBounded(t *testing.T) {
	h := open(t, GenerateBlocks(10000))
	for range 400 {
		h.tt.Scroll(500, 300, 0, 700)
	}
	if n := h.e.LaidBlocks(); n > maxLaid+100 {
		t.Fatalf("%d layouts kept", n)
	}
}

func TestKeystrokeShapesOneBlock(t *testing.T) {
	h := open(t, GenerateBlocks(10000))
	h.caretAt(strings.Index(h.e.Source(), "Markdown"))
	h.tt.Frame()
	h.e.ResetStats()
	for range 20 {
		h.tt.Type("x")
	}
	s := h.e.Stats
	if s.Shapes != 20 {
		t.Fatalf("20 keys shaped %d blocks", s.Shapes)
	}
	if s.Edits != 20 {
		t.Fatalf("edits %d", s.Edits)
	}
}

func TestCaretPassingThroughReshapesAtMostOncePerForm(t *testing.T) {
	h := open(t, GenerateBlocks(200))
	h.tt.Frame()
	for range 6 {
		h.tt.Key(0, ui.KeyDown)
	}
	for range 6 {
		h.tt.Key(0, ui.KeyUp)
	}
	h.e.ResetStats()
	for range 6 {
		h.tt.Key(0, ui.KeyDown)
	}
	if h.e.Stats.Shapes != 0 {
		t.Fatalf("walking cached blocks shaped %d", h.e.Stats.Shapes)
	}
}

func TestScrollAnchorHoldsWhenBlockAboveChanges(t *testing.T) {
	h := open(t, GenerateBlocks(2000))
	for range 40 {
		h.tt.Scroll(500, 300, 0, 300)
	}
	id := h.e.d.blocks[h.e.anchorIdx]
	before := h.e.blockTextY(h.e.anchorIdx)
	// An edit far above: text grows by many lines there.
	h.e.applyEdit(0, 0, strings.Repeat("x\n", 50)+"\n")
	h.e.current(0)
	h.tt.Frame()
	if h.e.d.blocks[h.e.anchorIdx].ID != id.ID {
		t.Fatalf("anchor changed block")
	}
	if after := h.e.blockTextY(h.e.anchorIdx); after != before {
		t.Fatalf("anchor moved on screen: %v -> %v", before, after)
	}
}

func TestScrollAnchorHoldsWhileEstimatesAreCorrected(t *testing.T) {
	h := open(t, GenerateBlocks(5000))
	h.tt.Key(ui.Cmd, ui.KeyEnd) // far down, blocks above never measured
	h.tt.Frame()
	h.tt.Scroll(500, 300, 0, -40)
	anchor, y := h.e.anchorIdx, h.e.blockTextY(h.e.anchorIdx)
	h.tt.Frame()
	if h.e.anchorIdx != anchor || h.e.blockTextY(anchor) != y {
		t.Fatalf("view moved in a frame without input: %d/%v -> %d/%v", anchor, y, h.e.anchorIdx, h.e.blockTextY(h.e.anchorIdx))
	}
}

func TestCaretScrollsIntoViewAndLeavesOverscanAlone(t *testing.T) {
	h := open(t, GenerateBlocks(2000))
	h.tt.Key(ui.Cmd, ui.KeyEnd)
	_, y, ch := h.e.caretRect()
	if y < 0 || y+ch > 700 {
		t.Fatalf("caret offscreen at %v", y)
	}
	h.tt.Key(ui.Cmd, ui.KeyHome)
	h.wantCaret(0)
	_, y, _ = h.e.caretRect()
	if y < 0 || y > 700 {
		t.Fatalf("caret offscreen at %v", y)
	}
}
