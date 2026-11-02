package ui_test

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aurink/internal/contract"
	"aurink/internal/core/document"
	"aurink/internal/core/editing"
	"aurink/internal/core/projection"
	"aurink/internal/views/liveedit"
	"aurink/internal/views/theme"
	"github.com/egoist/mygo/ui"
)

const boxLeft = 26 - 9 // from a list item's text back to the middle of its checkbox

func (h *harness) clickBox(r int) {
	h.t.Helper()
	at := h.e.RowText(r)
	h.tt.ClickAt(at.X-boxLeft, at.Y+9)
	h.tt.Frame()
}

func TestTaskToggleChangesOneByte(t *testing.T) {
	src := "- [ ] one\n- [x] two\n"
	h := open(t, src)
	h.caret(len(src))
	before := h.m.Selection().Primary().Head

	h.clickBox(0)
	h.wantSrc("- [x] one\n- [x] two\n")
	if got := h.m.Selection().Primary().Head; got != before {
		t.Fatalf("the toggle moved the caret from %d to %d", before, got)
	}
	h.clickBox(1)
	h.wantSrc("- [x] one\n- [ ] two\n")
	h.clickBox(0)
	h.wantSrc("- [ ] one\n- [ ] two\n")
	h.tt.Advance(time.Second)
	if h.tt.FramePending() && h.e.Stats.Frames > 1000 {
		t.Fatal("the tick keeps asking for frames")
	}
}

func TestTaskToggleUndoes(t *testing.T) {
	h := open(t, "- [ ] one\n")
	h.clickBox(0)
	h.wantSrc("- [x] one\n")
	h.mod(ui.Cmd, ui.KeyZ)
	h.wantSrc("- [ ] one\n")
}

func TestPressBesideABoxPlacesTheCaret(t *testing.T) {
	src := "- [ ] one\n"
	h := open(t, src)
	at := h.e.RowText(0)
	h.tt.ClickAt(at.X+40, at.Y+5)
	h.tt.Frame()
	h.wantSrc(src)
}

func TestContainersIndent(t *testing.T) {
	h := open(t, "para\n\n- a\n  - b\n\n> q\n")
	var para, a, b, q float32
	for r := range h.e.RowCount() {
		switch h.e.Display(r) {
		case "para":
			para = h.e.RowText(r).X
		case "a":
			a = h.e.RowText(r).X
		case "b":
			b = h.e.RowText(r).X
		case "q":
			q = h.e.RowText(r).X
		}
	}
	if !(para < a && a < b && para < q) {
		t.Fatalf("indents: para %v, a %v, b %v, q %v", para, a, b, q)
	}
	if err := h.e.CheckRows(); err != nil {
		t.Fatal(err)
	}
}

func TestLeadParagraphSetsLarger(t *testing.T) {
	src := "lead words\n\nlead words\n"
	h := open(t, src)
	h.caret(len(src))
	var first, second float32
	row := 0
	for r := range h.e.RowCount() {
		if h.e.Display(r) == "lead words" {
			if row == 0 {
				first = h.e.RowHeight(r)
			} else {
				second = h.e.RowHeight(r)
			}
			row++
		}
	}
	if !(first > second && second > 0) {
		t.Fatalf("first paragraph %v, later one %v", first, second)
	}
}

func cellRows(h *harness) map[string]int {
	out := map[string]int{}
	for r := range h.e.RowCount() {
		out[h.e.Display(r)] = r
	}
	return out
}

func TestTableCellsShareABand(t *testing.T) {
	src := "|aa|bb|\n|--|--|\n|cc|dd|\n\nend\n"
	h := open(t, src)
	h.caret(len(src))
	c := cellRows(h)
	aa, bb, cc, dd := h.e.RowText(c["aa"]), h.e.RowText(c["bb"]), h.e.RowText(c["cc"]), h.e.RowText(c["dd"])
	if aa.Y != bb.Y || cc.Y != dd.Y {
		t.Fatalf("rows are not bands: %v %v %v %v", aa, bb, cc, dd)
	}
	if !(aa.Y < cc.Y && aa.X == cc.X && bb.X == dd.X && aa.X < bb.X) {
		t.Fatalf("grid is off: %v %v %v %v", aa, bb, cc, dd)
	}
	if end := h.e.RowText(c["end"]); end.Y <= cc.Y {
		t.Fatalf("the paragraph after the table is at %v, the last band at %v", end.Y, cc.Y)
	}
	if err := h.e.CheckRows(); err != nil {
		t.Fatal(err)
	}
}

func TestTableBandBelowTheFold(t *testing.T) {
	src := ""
	for range 60 {
		src += "|aa|bb|\n"
		if src == "|aa|bb|\n" {
			src += "|--|--|\n"
		}
	}
	h := openSized(t, src, 800, 300)
	h.e.ScrollBy(500)
	h.tt.Frame()
	first, last := h.e.VisibleRows()
	if first%2 != 0 || last%2 != 1 {
		t.Fatalf("rows %d to %d split a band", first, last)
	}
	if h.e.RowText(first).Y != h.e.RowText(first+1).Y {
		t.Fatalf("band at %v and %v", h.e.RowText(first), h.e.RowText(first+1))
	}
}

func TestTableClickAndTypeInACell(t *testing.T) {
	src := "|aa|bb|\n|--|--|\n|cc|dd|\n\nend\n"
	h := open(t, src)
	h.caret(len(src))
	c := cellRows(h)
	at := h.e.RowText(c["dd"])
	h.tt.ClickAt(at.X+60, at.Y+4)
	h.tt.Frame()
	h.typed("x")
	h.wantSrc("|aa|bb|\n|--|--|\n|cc|ddx|\n\nend\n")
	if err := h.e.CheckRows(); err != nil {
		t.Fatal(err)
	}
}

func TestTableUpDownKeepsTheColumn(t *testing.T) {
	src := "|aa|bb|\n|--|--|\n|cc|dd|\n"
	h := open(t, src)
	h.caret(4) // in bb
	h.key(ui.KeyDown)
	h.typed("!")
	h.wantSrc("|aa|bb|\n|--|--|\n|cc|!dd|\n")
}

func TestTableAddingAColumnKeepsRowsConsistent(t *testing.T) {
	h := open(t, "|aa|bb|\n|--|--|\n|cc|dd|\n")
	h.caret(len("|aa|bb"))
	h.typed("|")
	if err := h.e.CheckRows(); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
}

func TestCodeCopyButton(t *testing.T) {
	src := "```go\nfunc f() {}\n```\n\nend\n"
	h := open(t, src)
	h.caret(len(src))
	at := h.e.RowText(0)
	// The button sits at the card's top right.
	h.tt.ClickAt(at.X+h.e.ColumnWidth()-60, at.Y-20)
	h.tt.Frame()
	if got := h.tt.Clipboard(); got != "func f() {}\n" && got != "func f() {}" {
		t.Fatalf("clipboard %q", got)
	}
	h.wantSrc(src)
}

func TestCodeIsHighlighted(t *testing.T) {
	code := func(lang string) []byte {
		src := "```" + lang + "\nfunc f() { return }\n```\n\nend\n"
		h := open(t, src)
		h.caret(len(src))
		img := h.tt.Image()
		from := int(h.e.RowText(0).Y) * img.Stride
		return img.Pix[from : from+24*img.Stride]
	}
	if bytes.Equal(code("go"), code("zz")) {
		t.Fatal("Go looks the same as no language")
	}
	if !bytes.Equal(code("zy"), code("zz")) {
		t.Fatal("two unknown languages look different")
	}
}

func TestLinkHintAndOpening(t *testing.T) {
	src := "[word](https://example.com/x) tail\n\nend\n"
	h := open(t, src)
	h.caret(len(src))
	at := h.e.RowText(0)
	h.tt.Move(at.X+10, at.Y+8)
	h.tt.Frame()
	if got := h.e.LinkHint(); got != "https://example.com/x" {
		t.Fatalf("hint %q", got)
	}
	h.tt.Move(at.X+10, at.Y+300)
	h.tt.Frame()
	if got := h.e.LinkHint(); got != "" {
		t.Fatalf("the hint stayed: %q", got)
	}
	// A plain click edits.
	h.tt.ClickAt(at.X+10, at.Y+8)
	h.tt.Frame()
	if h.m.Selection().Primary().Head == len(src) {
		t.Fatal("a plain click on a link did not place the caret")
	}
}

type waitDispatcher chan func()

func (d waitDispatcher) Post(f func()) { d <- f }

func TestImageLoadsAndGrowsItsRow(t *testing.T) {
	dir := t.TempDir()
	var png64 bytes.Buffer
	if err := png.Encode(&png64, image.NewRGBA(image.Rect(0, 0, 64, 300))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "p.png"), png64.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	src := "end\n\n![alt](p.png)\n\n![gone](missing.png)\n\nend\n"
	m := document.Factory{}.New("doc", src, contract.DocMeta{Dominant: "\n", Encoding: "utf-8"}, 0)
	d := make(waitDispatcher, 4)
	e := liveedit.New(liveedit.Options{Model: m, Projector: projection.New(), Editing: editing.New(), Dispatcher: d, DocDir: func() string { return dir }})
	t.Cleanup(e.Close)
	tt := ui.NewTester(func(c *ui.Context) { e.View(c) }, 1000, 900)
	tt.Frame()
	before := e.RowHeight(1)
	for range 2 {
		select {
		case f := <-d:
			f()
		case <-time.After(10 * time.Second):
			t.Fatal("an image never reported")
		}
	}
	tt.Frame()
	if after := e.RowHeight(1); after <= before || after < 300 {
		t.Fatalf("row height %v, then %v", before, after)
	}
	if !e.ImageFailed("missing.png") || e.ImageFailed("p.png") {
		t.Fatal("the images' states are wrong")
	}
	if err := e.CheckRows(); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteImageNeverLoads(t *testing.T) {
	h := open(t, "text\n\n![alt](https://example.com/p.png)\n")
	if !h.e.ImageFailed("https://example.com/p.png") {
		t.Fatal("a remote image was not refused")
	}
}

func TestThemeKeepsBothForms(t *testing.T) {
	src := "# title\n\nbody\n"
	h := open(t, src)
	dark, light := theme.For(theme.Options{Dark: true}), theme.For(theme.Options{})
	h.e.ResetStats()
	h.e.SetTheme(dark)
	h.tt.Frame()
	if h.e.Stats.Shapes == 0 {
		t.Fatal("the dark theme reused layouts with light colors")
	}
	h.e.SetTheme(light)
	h.tt.Frame()
	h.e.ResetStats()
	h.e.SetTheme(dark)
	h.tt.Frame()
	h.e.SetTheme(light)
	h.tt.Frame()
	if h.e.Stats.Shapes != 0 {
		t.Fatalf("switching back reshaped %d rows", h.e.Stats.Shapes)
	}
	h.wantSrc(src)
}

func TestGhostPaintsAndTakesNoInput(t *testing.T) {
	src := "ghost words\n"
	m := document.Factory{}.New("doc", src, contract.DocMeta{Dominant: "\n", Encoding: "utf-8"}, 0)
	e := liveedit.New(liveedit.Options{Model: m, Projector: projection.New(), Editing: editing.New()})
	t.Cleanup(e.Close)
	dark := theme.For(theme.Options{Dark: true})
	tt := ui.NewTester(func(c *ui.Context) { e.Ghost(c, dark) }, 800, 500)
	tt.Frame()
	img := tt.Image()
	first, seen := img.RGBAAt(0, 0), false
	for i := 0; i+3 < len(img.Pix) && !seen; i += 4 {
		seen = img.Pix[i] != first.R
	}
	if !seen || first.R > 128 {
		t.Fatalf("the ghost is not painted dark: %v, text %v", first, seen)
	}
	tt.ClickAt(200, 100)
	tt.Type("x")
	tt.Frame()
	if got := m.Text().String(); got != src {
		t.Fatalf("the ghost took input: %q", got)
	}
}

func TestEveryBlockKindPaints(t *testing.T) {
	src := "---\na: 1\n---\n\n# H1\n\nlead\n\n## H2\n\n###### H6\n\n- a\n- [x] t\n  1. n\n\n> q\n> r\n\n---\n\n```go\nvar x = 1\n```\n\n<div>h</div>\n\n[^1]: note\n\n[d]: http://e\n\ntext[^1] <b>i</b> **s** *e* ~~k~~ `c` [l](http://e) http://auto\n\n|a|b|\n|-|-|\n|c|d|\n\n![i](none.png)\n"
	h := open(t, src)
	for off := 0; off <= len(src); off += 7 {
		h.caret(off)
		if err := h.e.CheckRows(); err != nil {
			t.Fatalf("offset %d: %v", off, err)
		}
	}
	h.wantSrc(src)
}

// A tab in a code block takes up to the next stop and stays one byte in the source.
func TestCodeTabHasWidthAndStaysInSource(t *testing.T) {
	src := "```go\n\tx\n00\ty\n```\n"
	h := open(t, src)
	h.caret(strings.Index(src, "x"))
	d := h.e.Display(0)
	xAt := func(i int) float32 {
		t.Helper()
		r, _, ok := h.e.BoundsForRange(ui.TextInputRange{Start: i, End: i})
		if !ok {
			t.Fatalf("no bounds at %d of %q", i, d)
		}
		return r.X
	}
	tab := strings.Index(d, "\tx")
	if tab < 0 {
		t.Fatalf("display %q lost the tab", d)
	}
	zeros := strings.Index(d, "00")
	col := xAt(zeros+1) - xAt(zeros)
	if got := xAt(tab+1) - xAt(tab); got < 3.9*col || got > 4.1*col {
		t.Fatalf("leading tab is %v wide, a column is %v", got, col)
	}
	if got := xAt(zeros+3) - xAt(zeros); got < 3.9*col || got > 4.1*col {
		t.Fatalf("00 and a tab end at %v, want the stop at 4 columns of %v", got, col)
	}
	h.tt.Type("q")
	h.tt.Frame()
	h.wantSrc("```go\n\tqx\n00\ty\n```\n")
}

// Back at the first row, the page keeps its top margin.
func TestScrollToFirstRowKeepsTopMargin(t *testing.T) {
	h := open(t, strings.Repeat("a line\n\n", 200))
	h.e.ScrollBy(900)
	h.tt.Frame()
	h.e.ScrollToOffset(0)
	h.tt.Frame()
	if got := h.e.ScrollOffset(); got != -float32(theme.PadTop) {
		t.Fatalf("scroll %v, want the top margin %v", got, -float32(theme.PadTop))
	}
}

// TestQuoteIsABar: a quote is body text beside a bar that runs unbroken down
// all its paragraphs and stops at its end, as Typora shows it (user's ruling,
// 2026-11-02).
func TestQuoteIsABar(t *testing.T) {
	src := "lead\n\nplain words\n\n> quoted words\n>\n> second\n\nafter\n"
	h := open(t, src)
	h.caret(0)
	rows := cellRows(h)
	plain, quoted, second, after := rows["plain words"], rows["quoted words"], rows["second"], rows["after"]
	if h.e.RowHeight(quoted) != h.e.RowHeight(plain) {
		t.Fatalf("a quote line is %v high, a body line %v: not body text", h.e.RowHeight(quoted), h.e.RowHeight(plain))
	}
	img := h.tt.Image()
	bg := img.RGBAAt(2, 2)
	x := int(h.e.RowText(plain).X) + 2 // the bar's middle: the quote's indent starts at the column's left
	top, bottom := int(h.e.RowText(quoted).Y)+2, int(h.e.RowText(second).Y)+10
	for y := top; y < bottom; y++ {
		if img.RGBAAt(x, y) == bg {
			t.Fatalf("the bar breaks at y=%d (from %d to %d)", y, top, bottom)
		}
	}
	// Just above a line's text, clear of its glyphs whatever the font: the
	// bar stops at the quote's last line and does not reach "after".
	for _, r := range []int{plain, after} {
		if y := int(h.e.RowText(r).Y) - 3; img.RGBAAt(x, y) != bg {
			t.Fatalf("a bar beside %q", h.e.Display(r))
		}
	}
}

// TestLoneMarkerWaitsForItsSpace: typing "#", "+", "1." or ">" alone leaves a
// plain line showing the marker at body size; the space turns it into the
// block, as in Typora (user's report, 2026-11-02).
func TestLoneMarkerWaitsForItsSpace(t *testing.T) {
	for _, tc := range []struct {
		marker string
		kind   string
	}{{"#", "heading"}, {"+", "list"}, {"1.", "list"}, {">", "quote"}} {
		h := open(t, "lead\n\nplain\n\n")
		h.caret(len("lead\n\nplain\n\n"))
		body := h.e.RowHeight(cellRows(h)["plain"])
		h.typed(tc.marker)
		r := h.e.RowCount() - 1
		if d := h.e.Display(r); d != tc.marker {
			t.Fatalf("%q alone shows %q", tc.marker, d)
		}
		if h.e.RowHeight(r) != body || h.e.RowText(r).X != h.e.RowText(cellRows(h)["plain"]).X {
			t.Fatalf("%q alone is not a plain line: height %v (body %v), x %v", tc.marker, h.e.RowHeight(r), body, h.e.RowText(r).X)
		}
		h.typed("a")
		h.wantSrc("lead\n\nplain\n\n" + tc.marker + "a")
		h.key(ui.KeyBackspace)
		h.typed(" x")
		r = h.e.RowCount() - 1
		if d := h.e.Display(r); d != "x" {
			t.Fatalf("%q then a space shows %q, want the %s's text alone", tc.marker, d, tc.kind)
		}
		switch tc.kind {
		case "heading":
			if h.e.RowHeight(r) <= body {
				t.Fatal("\"# x\" is not a heading")
			}
		default:
			if h.e.RowText(r).X <= h.e.RowText(cellRows(h)["plain"]).X {
				t.Fatalf("%q is not indented as a %s", tc.marker+" x", tc.kind)
			}
		}
	}
}
