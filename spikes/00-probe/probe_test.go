package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown/parser"
)

// ---------- 1. Custom TextInputClient receives typing + IME ----------

type client struct {
	text     []rune
	sel      int
	marked   *ui.TextInputRange
	replaces []string
	marks    []string
}

func (c *client) u16() []uint16 { return utf16.Encode(c.text) }
func (c *client) TextForRange(r ui.TextInputRange) (string, ui.TextInputRange) {
	u := c.u16()
	s, e := max(0, r.Start), min(len(u), r.End)
	if s > e {
		s = e
	}
	return string(utf16.Decode(u[s:e])), ui.TextInputRange{Start: s, End: e}
}
func (c *client) Selection() ui.TextInputSelection {
	var s ui.TextInputSelection
	s.Range = ui.TextInputRange{Start: c.sel, End: c.sel}
	return s
}
func (c *client) MarkedRange() (ui.TextInputRange, bool) {
	if c.marked == nil {
		return ui.TextInputRange{}, false
	}
	return *c.marked, true
}
func (c *client) ReplaceText(r *ui.TextInputRange, s string) {
	c.replaces = append(c.replaces, s)
	var rr ui.TextInputRange
	if r != nil {
		rr = *r
	} else if c.marked != nil {
		rr = *c.marked
	} else {
		rr = ui.TextInputRange{Start: c.sel, End: c.sel}
	}
	u := c.u16()
	nu := append(append(append([]uint16{}, u[:rr.Start]...), utf16.Encode([]rune(s))...), u[rr.End:]...)
	c.text = utf16.Decode(nu)
	c.sel = rr.Start + len(utf16.Encode([]rune(s)))
	c.marked = nil
}
func (c *client) SetMarkedText(r *ui.TextInputRange, s string, _ ui.TextInputRange) {
	c.marks = append(c.marks, s)
	var rr ui.TextInputRange
	if r != nil {
		rr = *r
	} else if c.marked != nil {
		rr = *c.marked
	} else {
		rr = ui.TextInputRange{Start: c.sel, End: c.sel}
	}
	u := c.u16()
	enc := utf16.Encode([]rune(s))
	nu := append(append(append([]uint16{}, u[:rr.Start]...), enc...), u[rr.End:]...)
	c.text = utf16.Decode(nu)
	c.marked = &ui.TextInputRange{Start: rr.Start, End: rr.Start + len(enc)}
	c.sel = rr.Start + len(enc)
}
func (c *client) UnmarkText() { c.marked = nil }
func (c *client) BoundsForRange(r ui.TextInputRange) (ui.Rect, ui.TextInputRange, bool) {
	return ui.Rect{X: float32(r.Start) * 8, Y: 0, W: 1, H: 16}, r, true
}
func (c *client) IndexForPoint(p ui.Point) (int, bool) { return int(p.X / 8), true }

type probeApp struct{ cl *client }

func (a *probeApp) view(c *ui.Context) {
	ui.Box(c).Size(400, 200).HandleTextInput(a.cl).Label("Doc").Role(ui.RoleTextField)
}

func TestCustomClientTypingAndIME(t *testing.T) {
	a := &probeApp{cl: &client{}}
	tt := ui.NewTester(a.view, 480, 360)
	if err := tt.Click("Doc"); err != nil {
		t.Fatal(err)
	}
	tt.Type("# Hi")
	t.Logf("after Type: %q replaces=%q", string(a.cl.text), a.cl.replaces)
	tt.Compose("nihao", 5)
	t.Logf("mid-compose: %q marked=%v", string(a.cl.text), a.cl.marked)
	tt.Type("你好")
	t.Logf("after Compose: %q marks=%q marked=%v", string(a.cl.text), a.cl.marks, a.cl.marked)
	if string(a.cl.text) == "" {
		t.Fatal("custom client received nothing")
	}
}

// ---------- 2. Rich layout: mixed sizes, hit testing, objects ----------

func TestRichLayout(t *testing.T) {
	spans := []ui.Span{
		{Text: "Heading ", Size: 28, Weight: 700},
		{Text: "body text 中文混排 ", Size: 15},
		{Text: "code", Font: "monospace", Size: 13, Background: ui.RGB(240, 240, 240), Radius: 3},
		{Text: "￼", ObjectWidth: 40, ObjectHeight: 20},
		{Text: " tail", Size: 15},
	}
	l := ui.ShapeRichTextWith(spans, ui.Font{Size: 15, LineHeight: 1.6}, ui.TextShape{Width: 200})
	w, h := l.Size()
	t.Logf("size %.1fx%.1f lines=%d objects=%d", w, h, len(l.Lines()), len(l.Objects()))
	for i, ln := range l.Lines() {
		t.Logf(" line %d rect=%+v start=%d end=%d", i, ln.Rect, ln.Start, ln.End)
	}
	idx := l.IndexAt(ui.Point{X: 30, Y: 10})
	t.Logf("IndexAt(30,10)=%d caret(3)=%+v", idx, l.Caret(3))
	if len(l.Objects()) != 1 {
		t.Errorf("objects=%d", len(l.Objects()))
	}
}

// ---------- 3. Parser byte ranges, CRLF, front matter ----------

func dump(d *parser.Document, t *testing.T) {
	src := d.Source()
	for i := 0; i < d.Len(); i++ {
		b := d.Block(i)
		t.Logf("blk %d %-14s [%d,%d) %q", i, b.Kind, b.Start, b.End, clip(src[b.Start:b.End]))
		if b.Kind == parser.Paragraph || b.Kind == parser.Heading || b.Kind == parser.TableCell {
			for j, in := range d.Inlines(i) {
				t.Logf("    inl %d kind=%d [%d,%d) next=%d text=%q src=%q", j, in.Kind, in.Start, in.End, in.Next, in.Text, clip(src[in.Start:in.End]))
			}
		}
	}
}

func clip(s string) string {
	if len(s) > 40 {
		return s[:40] + "…"
	}
	return s
}

func TestParserRanges(t *testing.T) {
	src := "# Title **bold** \\* &amp;\n\nPara with *em* and [link](http://x.y \"t\") and `code`.\n"
	dump(parser.Parse(src), t)
}

func TestParserCRLF(t *testing.T) {
	src := "# A\r\n\r\npara **b**\r\nline2\r\n"
	dump(parser.Parse(src), t)
}

func TestParserFrontMatter(t *testing.T) {
	src := "---\ntitle: x\ntags: [a]\n---\n\n# Body\n"
	dump(parser.Parse(src), t)
}

func TestParserExtras(t *testing.T) {
	src := "==mark== ~sub~ ^sup^ :smile: [[wiki]] [TOC]\n\n| a | b |\n|:-|-:|\n| 1 | 2 |\n\n$$\nx^2\n$$\n\n- [x] done\n\n[^1]: note\n"
	dump(parser.New(parser.Default|parser.ExtDollarMath), t)
	d := parser.New(parser.Default | parser.ExtDollarMath)
	d.Set(src)
	dump(d, t)
}

// ---------- 4. Incremental edit equivalence & perf ----------

func bigDoc(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "## Section %d\n\nSome *text* with **bold** and `code` and a [link](http://e.com/%d).\n\n- item a\n- item b\n  - nested\n\n```go\nfunc f%d() {}\n```\n\n| a | b |\n|---|---|\n| %d | x |\n\n", i, i, i, i)
	}
	return b.String()
}

func TestParserIncremental(t *testing.T) {
	src := bigDoc(5000) // ~1MB
	t.Logf("doc bytes=%d", len(src))
	t0 := time.Now()
	d := parser.Parse(src)
	for i := 0; i < d.Len(); i++ {
		if k := d.Block(i).Kind; k == parser.Paragraph || k == parser.Heading {
			d.Inlines(i)
		}
	}
	t.Logf("full parse+inlines: %v blocks=%d", time.Since(t0), d.Len())
	mid := len(src) / 2
	mid = strings.Index(src[mid:], "Some ") + mid + 5
	t1 := time.Now()
	const N = 200
	for i := 0; i < N; i++ {
		d.Edit(mid, mid, "x")
		mid++
	}
	t.Logf("incremental edit avg: %v", time.Since(t1)/N)
	full := parser.Parse(d.Source())
	if full.Len() != d.Len() {
		t.Errorf("block count mismatch inc=%d full=%d", d.Len(), full.Len())
	}
	if full.HTML() != d.HTML() {
		t.Errorf("HTML mismatch after incremental edits")
	}
}

// ---------- 5. editor.Buffer fidelity ----------

func TestBufferFidelity(t *testing.T) {
	cases := map[string]string{
		"lf":    "a\nb\n",
		"crlf":  "a\r\nb\r\n",
		"mixed": "a\r\nb\nc\r\n",
		"bom":   "\xef\xbb\xbf# t\n",
		"noeol": "a\nb",
	}
	for name, src := range cases {
		buf := editor.NewBuffer(src)
		var out bytes.Buffer
		buf.WriteTo(&out)
		t.Logf("%-6s len=%d lineEnding=%v roundtrip=%v out=%q", name, buf.Len(), buf.LineEnding(), out.String() == src, out.String())
	}
}

// ---------- 6. TextBuffer ----------

func TestTextBuffer(t *testing.T) {
	src := "a\r\nb\r\n中文😀"
	tb := ui.NewTextBuffer(src)
	t.Logf("TextBuffer roundtrip=%v len=%v", tb.String() == src, len(tb.String()))
}
