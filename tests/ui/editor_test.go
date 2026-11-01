package ui_test

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

	"aurink/internal/contract"
	"aurink/internal/views/editorapi"
	"github.com/egoist/mygo/ui"
)

func TestTyping(t *testing.T) {
	cases := []struct {
		name, src string
		at        int
		in, want  string
		caret     int
	}{
		{"into empty", "", 0, "abc", "abc", 3},
		{"middle", "ad", 1, "bc", "abcd", 3},
		{"han", "", 0, "汉字", "汉字", 6},
		{"emoji", "a", 1, "😀", "a😀", 5},
		{"in strong", "**ab**", 3, "x", "**axb**", 4},
		{"heading", "# a", 3, "b", "# ab", 4},
		{"in list", "- a", 3, "b", "- ab", 4},
		{"crlf kept", "a\r\n\r\nb", 6, "c", "a\r\n\r\nbc", 7},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := open(t, c.src)
			h.caret(c.at)
			h.typed(c.in)
			h.wantSrc(c.want)
			h.wantCaret(c.caret)
		})
	}
}

func TestTypingReplacesSelection(t *testing.T) {
	h := open(t, "hello world")
	h.sel(0, 5)
	h.typed("x")
	h.wantSrc("x world")
	h.wantCaret(1)
}

func TestEnterGoesThroughEditing(t *testing.T) {
	h := open(t, "- a")
	h.caret(3)
	h.key(ui.KeyEnter)
	h.wantSrc("- a\n- ")
	h.wantCaret(6)
	h.typed("b")
	h.wantSrc("- a\n- b")
}

func TestShiftEnterIsAHardBreak(t *testing.T) {
	h := open(t, "ab")
	h.caret(1)
	h.mod(ui.Shift, ui.KeyEnter)
	if got := h.m.Text().String(); got == "ab" || !strings.Contains(got, "\n") {
		t.Fatalf("source %q has no line break", got)
	}
	if h.e.RowCount() != 1 {
		t.Fatalf("%d rows, a hard break stays in the paragraph", h.e.RowCount())
	}
}

func TestTabIndentsAList(t *testing.T) {
	h := open(t, "- a\n- b")
	h.caret(7)
	h.key(ui.KeyTab)
	h.wantSrc("- a\n  - b")
	h.mod(ui.Shift, ui.KeyTab)
	h.wantSrc("- a\n- b")
}

func TestInputRuleIsItsOwnUndoStep(t *testing.T) {
	h := open(t, "")
	h.typed("[")
	h.typed(" ")
	h.typed("]")
	h.typed(" ")
	h.wantSrc("- [ ] ")
	h.wantCaret(6)
	h.mod(ui.Cmd, ui.KeyZ)
	h.wantSrc("[ ] ")
}

func TestBackspaceAndDelete(t *testing.T) {
	cases := []struct {
		name, src string
		at        int
		key       ui.Key
		want      string
		caret     int
	}{
		{"back", "abc", 2, ui.KeyBackspace, "ac", 1},
		{"back han", "汉字", 6, ui.KeyBackspace, "汉", 3},
		{"back emoji", "a😀", 5, ui.KeyBackspace, "a", 1},
		{"back zwj", "👨‍👩‍👧", 18, ui.KeyBackspace, "", 0},
		{"back start", "abc", 0, ui.KeyBackspace, "abc", 0},
		{"forward", "abc", 1, ui.KeyDelete, "ac", 1},
		{"forward end", "abc", 3, ui.KeyDelete, "abc", 3},
		{"forward han", "汉字", 0, ui.KeyDelete, "字", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := open(t, c.src)
			h.caret(c.at)
			h.key(c.key)
			h.wantSrc(c.want)
			h.wantCaret(c.caret)
		})
	}
}

func TestBackspaceJoinsParagraphs(t *testing.T) {
	h := open(t, "a\n\nb")
	h.caret(3)
	h.key(ui.KeyBackspace)
	if got := h.m.Text().String(); got == "a\n\nb" {
		t.Fatal("nothing deleted")
	}
	if err := h.e.CheckRows(); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteSelectionAcrossRows(t *testing.T) {
	h := open(t, "aa\n\nbb\n\ncc")
	h.sel(1, 9)
	h.key(ui.KeyBackspace)
	h.wantSrc("ac")
	h.wantCaret(1)
}

func TestWordDelete(t *testing.T) {
	h := open(t, "one two")
	h.caret(7)
	h.mod(ui.Ctrl, ui.KeyBackspace)
	h.wantSrc("one ")
}

func TestArrowsWalkEverySourcePosition(t *testing.T) {
	// The markers show around the caret, so a walk stops at each visible position,
	// and in the end reaches both ends of the row.
	for _, src := range []string{"# 标题", "**ab** c", "a *b* `c`", "[x](y) z", "- 项"} {
		t.Run(src, func(t *testing.T) {
			h := open(t, src)
			h.mod(ui.Ctrl, ui.KeyEnd)
			last := h.m.Selection().Primary().Head
			seen := []int{last}
			for range len(src) + 2 {
				h.key(ui.KeyLeft)
				off := h.m.Selection().Primary().Head
				if off > seen[len(seen)-1] {
					t.Fatalf("left moved right: %v then %d", seen, off)
				}
				if off != seen[len(seen)-1] {
					seen = append(seen, off)
				}
			}
			first := seen[len(seen)-1]
			if src[0] == '-' {
				if first != 2 {
					t.Fatalf("list row starts at %d, walked %v", first, seen)
				}
			} else if first != 0 {
				t.Fatalf("left walk stops at %d: %v", first, seen)
			}
			back := []int{first}
			for range len(src) + 2 {
				h.key(ui.KeyRight)
				off := h.m.Selection().Primary().Head
				if off != back[len(back)-1] {
					back = append(back, off)
				}
			}
			if back[len(back)-1] != last {
				t.Fatalf("right walk ends at %d, want %d: %v", back[len(back)-1], last, back)
			}
		})
	}
}

func TestLeftEntersMarkersOfHiddenStrong(t *testing.T) {
	h := open(t, "a **b** c")
	h.caret(9)
	if d := h.e.Display(0); d != "a b c" {
		t.Fatalf("away from it the markers hide: %q", d)
	}
	h.keys(ui.KeyLeft, 2)
	h.wantCaret(7)
	if d := h.e.Display(0); d != "a **b** c" {
		t.Fatalf("at its edge the markers show: %q", d)
	}
}

func TestArrowsCrossRows(t *testing.T) {
	h := open(t, "ab\n\ncd")
	h.caret(2)
	h.key(ui.KeyRight)
	h.wantCaret(4)
	h.key(ui.KeyLeft)
	h.wantCaret(2)
}

func TestArrowsAtDocumentEdges(t *testing.T) {
	h := open(t, "ab")
	h.caret(0)
	h.key(ui.KeyLeft)
	h.wantCaret(0)
	h.caret(2)
	h.key(ui.KeyRight)
	h.wantCaret(2)
}

func TestArrowCollapsesSelection(t *testing.T) {
	h := open(t, "abcd")
	h.sel(1, 3)
	h.key(ui.KeyLeft)
	h.wantCaret(1)
	h.sel(3, 1)
	h.key(ui.KeyRight)
	h.wantCaret(3)
}

func TestShiftArrowExtends(t *testing.T) {
	h := open(t, "abcd")
	h.caret(1)
	h.mod(ui.Shift, ui.KeyRight)
	h.mod(ui.Shift, ui.KeyRight)
	h.wantSel(1, 3)
	h.mod(ui.Shift, ui.KeyLeft)
	h.wantSel(1, 2)
}

func TestWordMovement(t *testing.T) {
	h := open(t, "one two  three")
	h.caret(0)
	h.mod(ui.Ctrl, ui.KeyRight)
	h.wantCaret(3)
	h.mod(ui.Ctrl, ui.KeyRight)
	h.wantCaret(7)
	h.mod(ui.Ctrl, ui.KeyRight)
	h.wantCaret(14)
	h.mod(ui.Ctrl, ui.KeyLeft)
	h.wantCaret(9)
	h.mod(ui.Ctrl, ui.KeyLeft)
	h.wantCaret(4)
}

func TestWordMovementCrossesRows(t *testing.T) {
	h := open(t, "ab\n\ncd")
	h.caret(2)
	h.mod(ui.Ctrl, ui.KeyRight)
	h.wantCaret(4)
	h.mod(ui.Ctrl, ui.KeyLeft)
	h.wantCaret(2)
}

func TestWordMovementHanAndLatin(t *testing.T) {
	h := open(t, "abc汉字def")
	h.caret(0)
	h.mod(ui.Ctrl, ui.KeyRight)
	h.wantCaret(3)
	h.mod(ui.Ctrl, ui.KeyRight)
	h.wantCaret(9)
}

func TestUpDownKeepsX(t *testing.T) {
	h := open(t, "abcdefgh\n\nab\n\nabcdefgh")
	h.caret(6)
	h.key(ui.KeyDown)
	h.wantCaret(12) // the short row's end
	h.key(ui.KeyDown)
	off := h.m.Selection().Primary().Head
	if off < 18 {
		t.Fatalf("sticky x lost: caret at %d", off)
	}
}

func TestUpDownAtEdges(t *testing.T) {
	h := open(t, "abc")
	h.caret(1)
	h.key(ui.KeyUp)
	h.wantCaret(0)
	h.caret(1)
	h.key(ui.KeyDown)
	h.wantCaret(3)
}

func TestHomeEnd(t *testing.T) {
	h := open(t, "abc\n\ndef")
	h.caret(6)
	h.key(ui.KeyHome)
	h.wantCaret(5)
	h.key(ui.KeyEnd)
	h.wantCaret(8)
	h.mod(ui.Ctrl, ui.KeyHome)
	h.wantCaret(0)
	h.mod(ui.Ctrl, ui.KeyEnd)
	h.wantCaret(8)
}

func TestHomeOnWrappedLineStaysInLine(t *testing.T) {
	h := openSized(t, strings.Repeat("word ", 400), 500, 700)
	h.caret(1002)
	h.key(ui.KeyHome)
	off := h.m.Selection().Primary().Head
	if off == 0 || off >= 1002 {
		t.Fatalf("home went to %d", off)
	}
}

func TestSelectAll(t *testing.T) {
	src := "a\n\nb\n"
	h := open(t, src)
	h.mod(ui.Cmd, ui.KeyA)
	h.wantSel(0, len(src))
	h.key(ui.KeyBackspace)
	h.wantSrc("")
}

func TestClickPlacesCaret(t *testing.T) {
	h := open(t, "abcdef")
	x, y, _ := caretWindow(h)
	h.tt.ClickAt(x, y+4)
	h.tt.Frame()
	h.wantCaret(0)
	h.tt.ClickAt(990, y+4)
	h.tt.Frame()
	h.wantCaret(6)
}

func caretWindow(h *harness) (x, y, height float32) {
	h.t.Helper()
	r, ok := h.e.SelectionRect()
	if !ok {
		h.t.Fatal("no caret rect")
	}
	return r.X, r.Y, r.H
}

func TestShiftClickExtends(t *testing.T) {
	h := open(t, "abcdef")
	h.caret(0)
	_, y, _ := caretWindow(h)
	h.tt.ClickAtWith(ui.Shift, 990, y+4)
	h.tt.Frame()
	h.wantSel(0, 6)
}

func TestDoubleClickSelectsWord(t *testing.T) {
	h := open(t, "one two three")
	h.caret(5)
	x, y, _ := caretWindow(h)
	h.tt.ClickAt(x, y+4)
	h.tt.ClickAt(x, y+4)
	h.tt.Frame()
	h.wantSel(4, 7)
}

func TestDoubleClickInStrongLeavesMarkersOut(t *testing.T) {
	h := open(t, "x **bold** y")
	h.caret(5)
	x, y, _ := caretWindow(h)
	h.tt.ClickAt(x, y+4)
	h.tt.ClickAt(x, y+4)
	h.tt.Frame()
	s := h.m.Selection().Primary().Range()
	if got := h.m.Text().Slice(s); got != "bold" {
		t.Fatalf("selected %q", got)
	}
}

func TestTripleClickSelectsRow(t *testing.T) {
	h := open(t, "one two\n\nthree")
	h.caret(2)
	x, y, _ := caretWindow(h)
	h.tt.ClickAt(x, y+4)
	h.tt.ClickAt(x, y+4)
	h.tt.ClickAt(x, y+4)
	h.tt.Frame()
	h.wantSel(0, 7)
}

func TestDragSelects(t *testing.T) {
	h := open(t, "abcdef")
	h.caret(0)
	x, y, _ := caretWindow(h)
	h.tt.Press(x, y+4)
	h.tt.Move(x+1000, y+4)
	h.tt.Release(x+1000, y+4)
	h.tt.Frame()
	h.wantSel(0, 6)
}

func TestDragOutOfViewScrolls(t *testing.T) {
	h := open(t, longDoc(500))
	x, y, _ := caretWindow(h)
	h.tt.Press(x, y+4)
	h.tt.Move(x, 900) // 200 below the window
	before := h.e.ScrollOffset()
	for range 30 {
		h.tt.Advance(16 * time.Millisecond)
	}
	if h.e.ScrollOffset() <= before {
		t.Fatalf("scroll stayed at %v", before)
	}
	head := h.m.Selection().Primary().Head
	for range 30 {
		h.tt.Advance(16 * time.Millisecond)
	}
	if got := h.m.Selection().Primary().Head; got <= head {
		t.Fatalf("selection stopped at %d, was %d", got, head)
	}
	h.tt.Release(x, 900)
	stopped := h.e.ScrollOffset()
	h.tt.Advance(200 * time.Millisecond)
	if h.e.ScrollOffset() != stopped {
		t.Fatal("kept scrolling after release")
	}
}

func TestUndoRedo(t *testing.T) {
	h := open(t, "x")
	h.caret(1)
	h.typed("a")
	h.typed("b")
	h.mod(ui.Cmd, ui.KeyZ)
	h.wantSrc("x")
	h.mod(ui.Cmd, ui.KeyY)
	h.wantSrc("xab")
	h.mod(ui.Cmd, ui.KeyZ)
	h.wantSrc("x")
	h.mod(ui.Cmd|ui.Shift, ui.KeyZ)
	h.wantSrc("xab")
}

func TestCaretMoveBreaksCoalescing(t *testing.T) {
	h := open(t, "xy")
	h.caret(1)
	h.typed("a")
	h.key(ui.KeyRight)
	h.key(ui.KeyLeft)
	h.typed("b")
	h.mod(ui.Cmd, ui.KeyZ)
	h.wantSrc("xay")
}

func TestCopyIsMarkdownText(t *testing.T) {
	h := open(t, "a **b** c")
	h.sel(0, 7)
	h.mod(ui.Cmd, ui.KeyC)
	if got := h.tt.Clipboard(); got != "a **b**" {
		t.Fatalf("clipboard %q", got)
	}
	h.wantSrc("a **b** c")
}

func TestCutAndUndo(t *testing.T) {
	h := open(t, "hello")
	h.sel(1, 4)
	h.mod(ui.Cmd, ui.KeyX)
	h.wantSrc("ho")
	if h.tt.Clipboard() != "ell" {
		t.Fatalf("clipboard %q", h.tt.Clipboard())
	}
	h.mod(ui.Cmd, ui.KeyZ)
	h.wantSrc("hello")
}

func TestPaste(t *testing.T) {
	h := open(t, "ab")
	h.caret(1)
	h.tt.SetClipboard("# x\n\ny")
	h.mod(ui.Cmd, ui.KeyV)
	h.wantSrc("a# x\n\nyb")
	h.mod(ui.Cmd, ui.KeyZ)
	h.wantSrc("ab")
}

func TestCopyWithoutSelectionKeepsClipboard(t *testing.T) {
	h := open(t, "ab")
	h.tt.SetClipboard("old")
	h.mod(ui.Cmd, ui.KeyC)
	if h.tt.Clipboard() != "old" {
		t.Fatal("clipboard changed")
	}
}

func TestIMEPreeditNeverReachesSource(t *testing.T) {
	h := open(t, "ab")
	h.caret(1)
	h.tt.Compose("ni", 2)
	h.tt.Frame()
	h.wantSrc("ab")
	if h.e.Preedit() != "ni" {
		t.Fatalf("preedit %q", h.e.Preedit())
	}
	h.tt.Compose("nihao", 5)
	h.tt.Frame()
	h.wantSrc("ab")
}

func TestIMECommitIsOneUndoStep(t *testing.T) {
	h := open(t, "ab")
	h.caret(1)
	h.tt.Compose("ni", 2)
	h.tt.Compose("nihao", 5)
	h.tt.Compose("", 0)
	h.tt.Type("你好")
	h.tt.Frame()
	h.wantSrc("a你好b")
	h.wantCaret(7)
	if h.e.Preedit() != "" {
		t.Fatal("preedit left over")
	}
	h.mod(ui.Cmd, ui.KeyZ)
	h.wantSrc("ab")
}

func TestIMECancelChangesNothing(t *testing.T) {
	h := open(t, "abcd")
	h.sel(1, 3)
	h.tt.Compose("x", 1)
	h.tt.Frame()
	h.tt.Compose("", 0)
	h.tt.Frame()
	h.wantSrc("abcd")
	h.wantSel(1, 3)
	if h.m.CanUndo() {
		t.Fatal("a cancelled composition left a history step")
	}
}

func TestIMEKeysGoToInputMethod(t *testing.T) {
	h := open(t, "ab")
	h.caret(1)
	h.tt.Compose("n", 1)
	h.tt.Key(0, ui.KeyLeft)
	h.tt.Frame()
	h.wantCaret(1)
	h.wantSrc("ab")
}

func TestIMEClientProtocol(t *testing.T) {
	h := open(t, "ab")
	h.caret(1)
	h.e.SetMarkedText(nil, "你好", ui.TextInputRange{Start: 2, End: 2})
	h.tt.Frame()
	got, rg := h.e.TextForRange(ui.TextInputRange{Start: 0, End: 100})
	if got != "a你好b" || rg.End != 4 {
		t.Fatalf("text %q %v", got, rg)
	}
	if m, ok := h.e.MarkedRange(); !ok || m.Start != 1 || m.End != 3 {
		t.Fatalf("marked %v %v", m, ok)
	}
	if s := h.e.Selection(); s.Range.Start != 3 || s.Range.End != 3 {
		t.Fatalf("selection %v", s)
	}
	r, _, visible := h.e.BoundsForRange(ui.TextInputRange{Start: 1, End: 3})
	if !visible || r.W <= 1 || r.H <= 0 {
		t.Fatalf("bounds %v %v", r, visible)
	}
	if _, ok := h.e.IndexForPoint(ui.Point{X: r.X, Y: r.Y + 2}); !ok {
		t.Fatal("no index")
	}
	h.e.ReplaceText(&ui.TextInputRange{Start: 1, End: 3}, "好")
	h.tt.Frame()
	h.wantSrc("a好b")
}

func TestIMEUnmarkKeepsText(t *testing.T) {
	h := open(t, "ab")
	h.caret(1)
	h.e.SetMarkedText(nil, "x", ui.TextInputRange{})
	h.e.UnmarkText()
	h.tt.Frame()
	h.wantSrc("axb")
}

func TestIMECommitReplacesSelectionInOneStep(t *testing.T) {
	h := open(t, "abcd")
	h.sel(1, 3)
	h.e.SetMarkedText(nil, "x", ui.TextInputRange{})
	h.tt.Frame()
	h.e.ReplaceText(nil, "你")
	h.tt.Frame()
	h.wantSrc("a你d")
	h.mod(ui.Cmd, ui.KeyZ)
	h.wantSrc("abcd")
}

func TestClickDuringCompositionCommits(t *testing.T) {
	h := open(t, "ab\n\ncd")
	h.caret(1)
	h.e.SetMarkedText(nil, "x", ui.TextInputRange{})
	h.tt.Frame()
	h.tt.ClickAt(600, 600)
	h.tt.Frame()
	if !strings.Contains(h.m.Text().String(), "x") {
		t.Fatalf("composition lost: %q", h.m.Text().String())
	}
}

func TestBlurCommitsComposition(t *testing.T) {
	h := open(t, "ab")
	h.caret(1)
	h.e.SetMarkedText(nil, "x", ui.TextInputRange{})
	h.tt.Frame()
	h.tt.SetFocused(false)
	h.tt.Frame()
	if h.e.Focused() {
		t.Skip("the tester keeps element focus while the window blurs")
	}
	h.wantSrc("axb")
}

func TestReadOnlyRejectsEverything(t *testing.T) {
	h := open(t, "abc")
	h.m.SetReadOnly(true)
	h.tt.Frame()
	h.caret(1)
	h.typed("x")
	h.key(ui.KeyBackspace)
	h.key(ui.KeyEnter)
	h.tt.Compose("n", 1)
	h.tt.Frame()
	h.mod(ui.Cmd, ui.KeyZ)
	h.wantSrc("abc")
	h.key(ui.KeyRight)
	h.wantCaret(2)
	h.sel(0, 1)
	h.mod(ui.Cmd, ui.KeyC)
	if h.tt.Clipboard() != "a" {
		t.Fatal("copy must work when read-only")
	}
	h.mod(ui.Cmd, ui.KeyX)
	h.wantSrc("abc")
}

func TestReadModeHidesMarkers(t *testing.T) {
	h := open(t, "**a**")
	h.caret(2)
	if h.e.Display(0) != "**a**" {
		t.Fatalf("display %q", h.e.Display(0))
	}
	h.e.SetMode(editorapi.ModeRead)
	h.tt.Frame()
	if h.e.Display(0) != "a" {
		t.Fatalf("read display %q", h.e.Display(0))
	}
	h.typed("x")
	h.wantSrc("**a**")
	h.e.SetMode(editorapi.ModeLive)
	h.tt.Frame()
	if h.e.Display(0) != "**a**" {
		t.Fatalf("back %q", h.e.Display(0))
	}
}

func TestSelectionRevealsCoveredMarkers(t *testing.T) {
	h := open(t, "a **b** c\n\nd *e*")
	h.sel(0, h.m.Text().Len())
	if h.e.Display(0) != "a **b** c" || h.e.Display(1) != "d *e*" {
		t.Fatalf("display %q %q", h.e.Display(0), h.e.Display(1))
	}
	h.caret(0)
	if h.e.Display(1) != "d e" {
		t.Fatalf("display %q", h.e.Display(1))
	}
}

func TestFocusRequestWaitsForBuild(t *testing.T) {
	h := open(t, "a")
	if !h.e.Focused() {
		t.Fatal("first Focus did not take")
	}
}

func TestExternalEditFollows(t *testing.T) {
	h := open(t, "aa\n\nbb")
	h.caret(5)
	h.m.Apply(contract.Transaction{Edits: []contract.TextEdit{{Range: contract.Range{Start: 0, End: 0}, Text: "zz"}}, Origin: contract.OriginAPI})
	h.tt.Frame()
	h.wantSrc("zzaa\n\nbb")
	h.wantCaret(7)
	if h.e.Display(1) != "bb" {
		t.Fatalf("row 1 %q", h.e.Display(1))
	}
}

func TestReplaceRebuilds(t *testing.T) {
	h := open(t, "aa")
	h.m.Replace("# x\n\ny", contract.OriginReload)
	h.tt.Frame()
	h.wantSrc("# x\n\ny")
	if h.e.RowCount() != 2 {
		t.Fatalf("%d rows", h.e.RowCount())
	}
}

func TestRowOffsetsStayRightAfterEditAbove(t *testing.T) {
	h := open(t, "aa\n\n**b**")
	h.tt.Frame()
	h.caret(0)
	h.typed("xxx")
	h.caret(5 + 2 + 2)
	h.typed("y")
	h.wantSrc("xxxaa\n\n**yb**")
}

func TestLinkDefChangeReprojects(t *testing.T) {
	h := open(t, "[a][r] z\n\n[r]: /u")
	h.caret(h.m.Text().Len())
	if d := h.e.Display(0); d != "a z" {
		t.Fatalf("resolved %q", d)
	}
	h.m.Apply(contract.Transaction{Edits: []contract.TextEdit{{Range: contract.Range{Start: 10, End: 13}, Text: "[q]"}}, Origin: contract.OriginAPI})
	h.tt.Frame()
	if d := h.e.Display(0); d == "a z" {
		t.Fatal("the row kept the projection of a definition that is gone")
	}
}

func TestLastTableCellCaret(t *testing.T) {
	src := "| a | b |\n|---|---|\n| c | d |\n"
	h := open(t, src)
	off := strings.Index(src, "c") + 1
	h.caret(off)
	h.typed("x")
	want := src[:off] + "x" + src[off:]
	h.wantSrc(want)
	h.wantCaret(off + 1)
}

func TestWheelScrolls(t *testing.T) {
	h := open(t, longDoc(500))
	h.tt.Scroll(500, 300, 0, 400)
	h.tt.Frame()
	if h.e.ScrollOffset() <= 0 {
		t.Fatalf("scroll %v", h.e.ScrollOffset())
	}
}

func TestScrollDoesNotMoveCaret(t *testing.T) {
	h := open(t, longDoc(500))
	h.caret(3)
	h.tt.Scroll(500, 300, 0, 4000)
	h.tt.Frame()
	h.wantCaret(3)
	if first, _ := h.e.VisibleRows(); first == 0 {
		t.Fatal("the frame took the view back to the caret")
	}
}

func TestTypingBringsCaretBack(t *testing.T) {
	h := open(t, longDoc(500))
	h.caret(3)
	h.tt.Scroll(500, 300, 0, 4000)
	h.tt.Frame()
	h.typed("x")
	first, _ := h.e.VisibleRows()
	if first != 0 {
		t.Fatalf("view stayed at row %d", first)
	}
}

func TestPageDownMovesCaretAndView(t *testing.T) {
	h := open(t, longDoc(500))
	h.caret(0)
	h.key(ui.KeyPageDown)
	if h.m.Selection().Primary().Head == 0 || h.e.ScrollOffset() <= 0 {
		t.Fatalf("caret %d scroll %v", h.m.Selection().Primary().Head, h.e.ScrollOffset())
	}
	h.key(ui.KeyPageUp)
	if h.e.ScrollOffset() > 0 {
		t.Fatalf("scroll %v", h.e.ScrollOffset())
	}
}

func TestCtrlEndRevealsFarCaret(t *testing.T) {
	h := open(t, longDoc(5000))
	h.mod(ui.Ctrl, ui.KeyEnd)
	h.tt.Frame()
	r, ok := h.e.SelectionRect()
	if !ok || r.Y < 0 || r.Y+r.H > 700 {
		t.Fatalf("caret at %v, visible %v", r, ok)
	}
}

func TestRevealOffsetAligns(t *testing.T) {
	src := longDoc(2000)
	h := open(t, src)
	off := len(src) / 2
	for _, al := range []editorapi.ScrollAlign{editorapi.AlignTop, editorapi.AlignCenter, editorapi.AlignNearest} {
		h.e.RevealOffset(off, al)
		h.tt.Frame()
		r, ok := h.e.SelectionRect()
		if !ok {
			t.Fatalf("align %d: not visible", al)
		}
		if al == editorapi.AlignCenter && (r.Y < 300 || r.Y > 400) {
			t.Fatalf("center at %v", r)
		}
		if al == editorapi.AlignTop && (r.Y < 60 || r.Y > 90) {
			t.Fatalf("top at %v", r)
		}
	}
}

func TestTypewriterCenters(t *testing.T) {
	h := open(t, longDoc(500))
	h.e.SetTypewriter(true)
	h.caret(len(longDoc(100)))
	r, _ := h.e.SelectionRect()
	if r.Y < 300 || r.Y > 400 {
		t.Fatalf("caret at %v", r)
	}
}

func TestControllerReports(t *testing.T) {
	h := open(t, "# one\n\ntext\n\n## two\n\nmore")
	h.caret(7)
	if i, n := h.e.CaretBlock(); i != 2 || n != 4 {
		t.Fatalf("caret block %d of %d", i, n)
	}
	if id, ok := h.e.VisibleHeading(); !ok || id != h.m.AST().Outline()[0].Block {
		t.Fatalf("heading %d %v", id, ok)
	}
	if f := h.e.Formatting(); f.Block != contract.BlockParagraph {
		t.Fatalf("format %+v", f)
	}
	h.sel(7, 11)
	if err := h.e.RunEdit(contract.EditCommand{ID: contract.CmdFormatStrong}); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	h.wantSrc("# one\n\n**text**\n\n## two\n\nmore")
	if r, ok := h.e.SelectionRect(); !ok || r.W <= 0 {
		t.Fatalf("rect %v %v", r, ok)
	}
	h.e.SetFocusMode(true)
	h.e.SetFindHighlights([]contract.Range{{Start: 0, End: 1}, {Start: 9, End: 12}}, 1)
	h.tt.Frame()
	h.e.ClearFindHighlights()
	h.tt.Frame()
	if h.e.ScrollFraction() != 0 {
		t.Fatalf("fraction %v", h.e.ScrollFraction())
	}
}

func longDoc(rows int) string {
	var b strings.Builder
	for i := range rows {
		switch i % 10 {
		case 0:
			fmt.Fprintf(&b, "# 标题 %d\n\n", i)
		case 5:
			fmt.Fprintf(&b, "- 项 %d\n- 另一项 **粗** `码`\n\n", i)
		default:
			fmt.Fprintf(&b, "段落 %d 有 **粗体** 与 *斜体*，also some latin words that wrap on narrow windows and go on for a while.\n\n", i)
		}
	}
	return b.String()
}

func TestTypingShapesOnlyItsRow(t *testing.T) {
	h := open(t, longDoc(2000))
	h.caret(20)
	for range 10 {
		h.tt.Advance(time.Millisecond)
	}
	h.e.ResetStats()
	for range 50 {
		h.typed("x")
	}
	s := h.e.Stats
	if s.RowRebuilds != 0 {
		t.Fatalf("%d walks over all rows while typing", s.RowRebuilds)
	}
	if s.MaxFrameShapes > 3 {
		t.Fatalf("up to %d shapes per frame", s.MaxFrameShapes)
	}
	// Rows below the edit move but keep their projections.
	if s.Projects > 50*3 {
		t.Fatalf("%d projections in 50 keystrokes", s.Projects)
	}
}

func TestCaretWalkDoesNotReshapeWhenDisplaySame(t *testing.T) {
	h := open(t, longDoc(50))
	h.caret(30)
	h.e.ResetStats()
	for range 5 {
		h.key(ui.KeyRight)
	}
	if h.e.Stats.Shapes > 2 {
		t.Fatalf("%d shapes", h.e.Stats.Shapes)
	}
}

func TestScrollShapesAtMostTwiceVisible(t *testing.T) {
	h := open(t, longDoc(10000))
	if h.e.RowCount() < 10000 {
		t.Fatalf("%d rows", h.e.RowCount())
	}
	h.e.ResetStats()
	maxVis := 0
	for range 200 {
		h.tt.Scroll(500, 300, 0, 120)
		h.tt.Frame()
		f, l := h.e.VisibleRows()
		maxVis = max(maxVis, l-f+1)
	}
	if got := h.e.Stats.MaxFrameShapes; got > maxVis*2 {
		t.Fatalf("%d shapes in a frame, %d rows visible", got, maxVis)
	}
}

func TestRandomKeysAgainstString(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	h := open(t, longDoc(30))
	ks := []ui.Key{ui.KeyLeft, ui.KeyRight, ui.KeyUp, ui.KeyDown, ui.KeyHome, ui.KeyEnd}
	for i := range 600 {
		switch r.Intn(8) {
		case 0, 1, 2:
			h.key(ks[r.Intn(len(ks))])
		case 3:
			h.typed(string(rune('a' + r.Intn(26))))
		case 4:
			h.typed("汉")
		case 5:
			h.key(ui.KeyBackspace)
		case 6:
			h.key(ui.KeyEnter)
		case 7:
			if r.Intn(3) == 0 {
				h.mod(ui.Cmd, ui.KeyZ)
			} else {
				h.mod(ui.Shift, ui.KeyRight)
			}
		}
		if err := h.e.CheckRows(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if s := h.m.Selection().Primary(); s.Head < 0 || s.Head > h.m.Text().Len() {
			t.Fatalf("step %d: head %d of %d", i, s.Head, h.m.Text().Len())
		}
	}
	for h.m.CanUndo() {
		h.m.Undo()
	}
	h.tt.Frame()
	h.wantSrc(longDoc(30))
}

func TestWidthChangeRelays(t *testing.T) {
	h := open(t, longDoc(100))
	h.tt.SetSize(420, 700)
	h.tt.Frame()
	h.caret(40)
	h.typed("x")
	r, ok := h.e.SelectionRect()
	if !ok || r.X >= 420 {
		t.Fatalf("caret %v", r)
	}
}

// A row that changes height as its markers show must not move what is above it.
func TestRevealKeepsRowsAboveInPlace(t *testing.T) {
	long := strings.Repeat("**word** ", 60)
	h := openSized(t, longDoc(200)+long+"\n\nend", 500, 700)
	row := h.e.RowCount() - 2
	off := strings.Index(h.m.Text().String(), long)
	h.e.RevealOffset(off+len(long)-1, editorapi.AlignCenter)
	h.tt.Frame()
	h.caret(h.m.Text().Len())
	h.tt.Frame()
	y0 := h.e.ScrollOffset()
	hidden := h.e.Display(row)
	h.caret(off + 2)
	if h.e.Display(row) == hidden {
		t.Fatal("expected the row to show a marker at the caret")
	}
	if got := h.e.ScrollOffset(); got > y0+1 || got < y0-200 {
		t.Fatalf("scroll went from %v to %v", y0, got)
	}
}

func TestCandidateWindowFollowsPreedit(t *testing.T) {
	h := open(t, "ab")
	h.caret(1)
	h.e.SetMarkedText(nil, "n", ui.TextInputRange{Start: 1, End: 1})
	h.tt.Frame()
	short, _, _ := h.e.BoundsForRange(ui.TextInputRange{Start: 2, End: 2})
	h.e.SetMarkedText(nil, "nihaoma", ui.TextInputRange{Start: 7, End: 7})
	h.tt.Frame()
	long, _, _ := h.e.BoundsForRange(ui.TextInputRange{Start: 8, End: 8})
	if long.X <= short.X {
		t.Fatalf("caret bounds %v then %v", short, long)
	}
	if r, ok := h.tt.TextCaret(); !ok || r.H <= 0 {
		t.Fatalf("window caret %v %v", r, ok)
	}
}

func TestCompositionInHiddenMarkerRowKeepsMapping(t *testing.T) {
	h := open(t, "a **b** c")
	h.caret(9)
	h.e.SetMarkedText(nil, "中", ui.TextInputRange{Start: 1, End: 1})
	h.tt.Frame()
	h.e.ReplaceText(nil, "中")
	h.tt.Frame()
	h.wantSrc("a **b** c中")
	h.wantCaret(12)
}

func TestCaretAtCellEndStaysInCell(t *testing.T) {
	src := "|a|b|\n|-|-|\n|c|d|\n"
	h := open(t, src)
	first := strings.Index(src, "c")
	h.caret(first)
	at, _ := h.e.CaretBlock()
	h.caret(first + 1)
	if got, _ := h.e.CaretBlock(); got != at {
		t.Fatalf("caret after c is in row %d, c is in row %d", got, at)
	}
}

// Editing a definition changes rows that were not edited.
func TestDefinitionEditDoesNotLeaveStaleLink(t *testing.T) {
	h := open(t, "[a][r] z\n\n[r]: /u\n\ny")
	h.caret(h.m.Text().Len())
	if got := h.e.LinkTarget(0); got != "/u" {
		t.Fatalf("link target %q", got)
	}
	walks := h.e.Stats.RowRebuilds
	h.m.Apply(contract.Transaction{Edits: []contract.TextEdit{{Range: contract.Range{Start: 16, End: 17}, Text: "v"}}, Origin: contract.OriginAPI})
	h.tt.Frame()
	h.wantSrc("[a][r] z\n\n[r]: /v\n\ny")
	if got := h.e.LinkTarget(0); got != "/v" {
		t.Fatalf("link target %q, walks %d", got, h.e.Stats.RowRebuilds-walks)
	}
}
