package ui_test

import (
	"math/rand/v2"
	"strings"
	"testing"

	"aurink/internal/contract"
	"aurink/internal/core/document"
	"aurink/internal/core/editing"
	"aurink/internal/core/projection"
	"aurink/internal/views/liveedit"
	"aurink/internal/views/source"
	"aurink/internal/views/theme"
	"aurink/tests/corpus"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

type sourceHarness struct {
	t  *testing.T
	m  contract.DocumentModel
	s  *source.Editor
	tt *ui.Tester
}

func openSource(t *testing.T, src string) *sourceHarness {
	t.Helper()
	dominant := "\n"
	if strings.Count(src, "\r\n")*2 > strings.Count(src, "\n") {
		dominant = "\r\n"
	}
	m := document.Factory{}.New("doc", src, contract.DocMeta{Dominant: dominant, Encoding: "utf-8"}, 0)
	s := source.New(m, theme.For(theme.Options{}))
	t.Cleanup(s.Close)
	tt := ui.NewTester(func(c *ui.Context) { s.View(c) }, 900, 600)
	tt.Frame()
	return &sourceHarness{t, m, s, tt}
}

func (h *sourceHarness) text() string { return h.m.Text().String() }

func (h *sourceHarness) want(src string) {
	h.t.Helper()
	h.s.Sync()
	if got := h.text(); got != src {
		h.t.Fatalf("source\n got %q\nwant %q", got, src)
	}
}

func TestSourceNoEditKeepsEveryCorpusByte(t *testing.T) {
	entries, err := corpus.Index()
	if err != nil {
		t.Fatal(err)
	}
	mixed := 0
	for _, entry := range entries {
		data, err := corpus.Read(entry)
		if err != nil {
			t.Fatal(err)
		}
		src := string(data)
		if strings.Contains(src, "\r\n") && strings.Contains(strings.ReplaceAll(src, "\r\n", ""), "\n") {
			mixed++
		}
		h := openSource(t, src)
		for range 3 {
			top := h.s.Leave()
			h.s.Enter(top)
			h.tt.Frame()
		}
		h.want(src)
	}
	if mixed == 0 {
		t.Fatal("the corpus has no document with mixed line endings")
	}
}

func TestSourceEditChangesOnlyEditedBytes(t *testing.T) {
	src := "one\r\ntwo\nthree\r\nfour\n"
	for _, c := range []struct {
		name string
		edit func(b *editor.Buffer)
		want string
	}{
		{"typed in a line", func(b *editor.Buffer) { b.Edit(editor.Edit{Start: 7, End: 7, Text: "X"}) }, "one\r\ntwoX\nthree\r\nfour\n"},
		{"in a CRLF line", func(b *editor.Buffer) { b.Edit(editor.Edit{Start: 8, End: 8, Text: "X"}) }, "one\r\ntwo\nXthree\r\nfour\n"},
		{"at a line's end", func(b *editor.Buffer) { b.Edit(editor.Edit{Start: 3, End: 3, Text: "X"}) }, "oneX\r\ntwo\nthree\r\nfour\n"},
		{"newline joined", func(b *editor.Buffer) { b.Edit(editor.Edit{Start: 3, End: 4, Text: ""}) }, "onetwo\nthree\r\nfour\n"},
		{"two places", func(b *editor.Buffer) {
			b.Edit(editor.Edit{Start: 0, End: 0, Text: "A"}, editor.Edit{Start: 14, End: 14, Text: "B"})
		}, "Aone\r\ntwo\nthree\r\nBfour\n"},
		{"newline typed", func(b *editor.Buffer) { b.Edit(editor.Edit{Start: 7, End: 7, Text: "\n"}) }, "one\r\ntwo\n\nthree\r\nfour\n"},
		{"a whole CRLF line", func(b *editor.Buffer) { b.Edit(editor.Edit{Start: 8, End: 14, Text: ""}) }, "one\r\ntwo\nfour\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := openSource(t, src)
			c.edit(h.s.State().Buffer)
			h.want(c.want)
		})
	}
}

func TestSourceKeysEdit(t *testing.T) {
	h := openSource(t, "a\r\nb\n")
	h.tt.ClickAt(300, 100)
	h.s.State().Select(2, 2)
	h.tt.Type("x")
	h.tt.Frame()
	h.want("a\r\nxb\n")
}

func TestSourceUndoIsTheDocuments(t *testing.T) {
	h := openSource(t, "a\r\nb\n")
	h.s.State().Buffer.Edit(editor.Edit{Start: 2, End: 2, Text: "x"})
	h.want("a\r\nxb\n")
	h.tt.ClickAt(300, 100)
	h.tt.Frame()
	if !h.m.CanUndo() {
		t.Fatal("the document cannot undo a source edit")
	}
	h.tt.Key(wordKey(), ui.KeyZ)
	h.tt.Frame()
	h.want("a\r\nb\n")
	if got := h.s.State().Buffer.String(); got != "a\nb\n" {
		t.Fatalf("buffer %q", got)
	}
	h.tt.Key(wordKey()|ui.Shift, ui.KeyZ)
	h.tt.Frame()
	h.want("a\r\nxb\n")
}

func TestSourceFollowsTheDocument(t *testing.T) {
	h := openSource(t, "a\nb\n")
	if _, err := h.m.Apply(contract.Transaction{Edits: []contract.TextEdit{{Range: contract.Range{Start: 2, End: 3}, Text: "zz"}}, Origin: contract.OriginCommand}); err != nil {
		t.Fatal(err)
	}
	if got := h.s.State().Buffer.String(); got != "a\nzz\n" {
		t.Fatalf("buffer %q", got)
	}
	h.want("a\nzz\n")
}

func TestSourceSelectionCrossesTheViews(t *testing.T) {
	src := "one\r\ntwo\r\nthree\n"
	h := openSource(t, src)
	h.m.SetSelection(contract.Caret(5)) // the start of two
	h.s.Enter(0)
	if got := h.s.State().Cursor(); got != 4 {
		t.Fatalf("mirror caret %d", got)
	}
	h.s.State().Select(8, 8) // the start of three
	h.s.Leave()
	if got := h.m.Selection().Primary().Head; got != 10 {
		t.Fatalf("document caret %d", got)
	}
}

func TestSourceToggleHundredTimes(t *testing.T) {
	src := "# t\r\n\r\ntext **b**\nmore\r\n"
	h := openSource(t, src)
	e := liveedit.New(liveedit.Options{Model: h.m, Projector: projection.New(), Editing: editing.New()})
	t.Cleanup(e.Close)
	h.m.SetSelection(contract.Caret(9))
	for i := range 100 {
		h.s.Enter(0)
		if i%10 == 5 {
			h.s.State().Buffer.Edit(editor.Edit{Start: h.s.State().Cursor(), End: h.s.State().Cursor(), Text: "y"})
		}
		h.tt.Frame()
		h.s.Leave()
	}
	if got := strings.Count(h.text(), "y"); got != 10 {
		t.Fatalf("%d edits landed: %q", got, h.text())
	}
	if !strings.Contains(h.text(), "# t\r\n\r\n") || !strings.HasSuffix(h.text(), "more\r\n") || !strings.Contains(h.text(), "\nmore") {
		t.Fatalf("line endings moved: %q", h.text())
	}
	if err := e.CheckRows(); err != nil {
		t.Fatal(err)
	}
	for range 10 {
		h.m.Undo()
	}
	h.want(src)
}

// Random edits never touch a line ending they did not delete or type.
func TestSourceRandomEditsKeepLineEndings(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 9))
	for range 200 {
		var b strings.Builder
		for i := range 2 + r.IntN(12) {
			b.WriteString(string(rune('a' + i)))
			b.WriteString([]string{"\n", "\r\n"}[r.IntN(2)])
		}
		src := b.String()
		h := openSource(t, src)
		buf := h.s.State().Buffer
		n := buf.Len()
		// Letters only, at the starts of lines: every line ending stays.
		line := r.IntN(buf.LineCount())
		other := r.IntN(buf.LineCount())
		if line > other {
			line, other = other, line
		}
		edits := []editor.Edit{{Start: buf.Offset(editor.Pos{Line: line}), End: buf.Offset(editor.Pos{Line: line}), Text: "X"}}
		if other > line {
			at := buf.Offset(editor.Pos{Line: other})
			edits = append(edits, editor.Edit{Start: at, End: at, Text: "Y"})
		}
		buf.Edit(edits...)
		h.s.Sync()
		got := strings.NewReplacer("X", "", "Y", "").Replace(h.text())
		if got != src {
			t.Fatalf("%d bytes, lines %d and %d: %q became %q", n, line, other, src, h.text())
		}
		if lf := strings.ReplaceAll(h.text(), "\r\n", "\n"); lf != buf.String() {
			t.Fatalf("mirror %q, document %q", buf.String(), h.text())
		}
	}
}

func TestSourceNewLinesTakeTheDocumentsEnding(t *testing.T) {
	h := openSource(t, "a\r\nb\r\n")
	h.s.State().Buffer.Edit(editor.Edit{Start: 1, End: 1, Text: "\nz"})
	h.want("a\r\nz\r\nb\r\n")
}
