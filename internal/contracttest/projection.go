package contracttest

import (
	"strings"
	"testing"
	"unicode/utf8"

	"aurink/internal/contract"
	"aurink/tests/corpus"
)

// ProjectionFixture is a projector and a way to parse the documents it projects.
type ProjectionFixture struct {
	Projector contract.Projector
	// Parse returns a snapshot of src and its AST.
	Parse func(src string) (contract.Text, contract.AST)
}

// projectionCorpus returns astSamples and the small documents of the corpus.
func projectionCorpus(t *testing.T) []string {
	t.Helper()
	entries, err := corpus.Index()
	if err != nil {
		t.Fatal(err)
	}
	docs := append([]string(nil), astSamples...)
	for _, e := range entries {
		if e.Size > 16<<10 {
			continue
		}
		b, err := corpus.Read(e)
		if err != nil {
			t.Fatal(err)
		}
		if utf8.Valid(b) {
			docs = append(docs, string(b))
		}
	}
	return docs
}

// TestProjection checks docs/04 §4.3 for a projector.
func TestProjection(t *testing.T, f ProjectionFixture) {
	pr := f.Projector
	t.Run("invariants", func(t *testing.T) {
		for _, src := range projectionCorpus(t) {
			text, ast := f.Parse(src)
			for leaf := range ast.Leaves() {
				b := ast.Block(leaf)
				carets := []int{-1, b.Range.Start, (b.Range.Start + b.Range.End) / 2, b.Range.End}
				for _, mode := range []contract.RevealMode{contract.RevealInline, contract.RevealBlock, contract.RevealAlways, contract.RevealNone} {
					for _, caret := range carets {
						p := pr.Project(ast, text, leaf, contract.Reveal{Mode: mode, Caret: caret})
						checkProjection(t, pr, src, b, &p, mode)
					}
				}
			}
		}
	})
	t.Run("caret walks through every leaf", func(t *testing.T) {
		for _, src := range projectionCorpus(t) {
			text, ast := f.Parse(src)
			for leaf := range ast.Leaves() {
				walkCaret(t, pr, src, ast, text, leaf)
			}
		}
	})
	t.Run("inline reveal", func(t *testing.T) {
		src := "a **粗** b\n"
		text, ast := f.Parse(src)
		leaf := ast.LeafAt(0)
		display := func(caret int) string {
			p := pr.Project(ast, text, leaf, contract.Reveal{Mode: contract.RevealInline, Caret: caret})
			return pr.DisplayText(&p)
		}
		if got := display(0); got != "a 粗 b" {
			t.Errorf("caret outside: %q", got)
		}
		if got := display(strings.Index(src, "粗")); got != "a **粗** b" {
			t.Errorf("caret inside: %q", got)
		}
		if got := display(-1); got != "a 粗 b" {
			t.Errorf("caret in another block: %q", got)
		}
	})
	t.Run("nested markers show with their ancestors", func(t *testing.T) {
		src := "**a *b* c** d\n"
		text, ast := f.Parse(src)
		p := pr.Project(ast, text, ast.LeafAt(0), contract.Reveal{Mode: contract.RevealInline, Caret: strings.Index(src, "b")})
		if got := pr.DisplayText(&p); got != "**a *b* c** d" {
			t.Errorf("display %q", got)
		}
	})
	t.Run("a selection reveals the nodes it covers", func(t *testing.T) {
		src := "**a** *b* c\n"
		text, ast := f.Parse(src)
		p := pr.Project(ast, text, ast.LeafAt(0), contract.Reveal{
			Mode: contract.RevealInline, Caret: -1, Selection: contract.Range{Start: 0, End: strings.Index(src, " c")},
		})
		if got := pr.DisplayText(&p); got != "**a** *b* c" {
			t.Errorf("display %q", got)
		}
	})
	t.Run("headings show # only at the start of the line", func(t *testing.T) {
		src := "## 标题\n"
		text, ast := f.Parse(src)
		leaf := ast.LeafAt(0)
		show := func(caret int) string {
			p := pr.Project(ast, text, leaf, contract.Reveal{Mode: contract.RevealInline, Caret: caret})
			return pr.DisplayText(&p)
		}
		if got := show(strings.Index(src, "题")); got != "标题" {
			t.Errorf("caret in the text: %q", got)
		}
		if got := show(0); got != "## 标题" {
			t.Errorf("caret at the start: %q", got)
		}
	})
	t.Run("replacements", func(t *testing.T) {
		src := "a &amp; \\* b\n"
		text, ast := f.Parse(src)
		p := pr.Project(ast, text, ast.LeafAt(0), contract.Reveal{Mode: contract.RevealInline, Caret: -1})
		if got := pr.DisplayText(&p); got != "a & * b" {
			t.Errorf("display %q", got)
		}
	})
	t.Run("objects", func(t *testing.T) {
		src := "![图](a.png)\n"
		text, ast := f.Parse(src)
		p := pr.Project(ast, text, ast.LeafAt(0), contract.Reveal{Mode: contract.RevealInline, Caret: -1})
		var obj *contract.ObjectRef
		for _, s := range p.Segments {
			if s.Kind == contract.SegObject {
				obj = s.Object
			}
		}
		if obj == nil || obj.Kind != contract.ObjImage || obj.Source != "a.png" || obj.Alt != "图" {
			t.Errorf("object %+v", obj)
		}
	})
	t.Run("tasks", func(t *testing.T) {
		src := "- [x] 项\n"
		text, ast := f.Parse(src)
		leaf := ast.LeafAt(strings.Index(src, "项"))
		p := pr.Project(ast, text, leaf, contract.Reveal{Mode: contract.RevealInline, Caret: -1})
		if len(p.Containers) == 0 {
			t.Fatal("no containers")
		}
		c := p.Containers[len(p.Containers)-1]
		if c.Task != contract.TaskDone || src[c.TaskSrc.Start:c.TaskSrc.End] != "[x]" {
			t.Errorf("container %+v", c)
		}
		if got := pr.DisplayText(&p); got != "项" {
			t.Errorf("display %q", got)
		}
	})
	t.Run("Sig", func(t *testing.T) {
		src := "x\n\n**a** b\n"
		text, ast := f.Parse(src)
		at := strings.Index(src, "**")
		hidden := pr.Project(ast, text, ast.LeafAt(at), contract.Reveal{Mode: contract.RevealInline, Caret: -1})
		shown := pr.Project(ast, text, ast.LeafAt(at), contract.Reveal{Mode: contract.RevealInline, Caret: at + 2})
		if hidden.Sig == shown.Sig {
			t.Error("a different display has the same Sig")
		}
		// Moving a block does not change its layout key.
		moved := "longer first line\n\n**a** b\n"
		text2, ast2 := f.Parse(moved)
		other := pr.Project(ast2, text2, ast2.LeafAt(strings.Index(moved, "**")), contract.Reveal{Mode: contract.RevealInline, Caret: -1})
		if hidden.Sig != other.Sig {
			t.Error("Sig depends on the offset")
		}
	})
	t.Run("Display does not pin the source", func(t *testing.T) {
		src := strings.Repeat("word ", 100) + "\n"
		text, ast := f.Parse(src)
		p := pr.Project(ast, text, ast.LeafAt(0), contract.Reveal{Mode: contract.RevealInline, Caret: -1})
		for _, s := range p.Segments {
			if s.Display != "" && sameMemory(s.Display, text.String()) {
				t.Fatal("a segment points into the document's string")
			}
		}
	})
}

func checkProjection(t *testing.T, pr contract.Projector, src string, b *contract.Block, p *contract.BlockProjection, mode contract.RevealMode) {
	t.Helper()
	fail := func(format string, args ...any) {
		t.Helper()
		args = append([]any{src, mode}, args...)
		t.Fatalf("source %q, mode %d: "+format, args...)
	}
	at := b.Lead.Start
	var display strings.Builder
	for _, s := range p.Segments {
		if s.Src.Start != at || s.Src.End < s.Src.Start {
			fail("segment %+v starts at %d, want %d", s, s.Src.Start, at)
		}
		at = s.Src.End
		raw := src[s.Src.Start:s.Src.End]
		switch s.Kind {
		case contract.SegText, contract.SegMarker:
			if s.Display != raw {
				fail("segment %+v shows %q, source %q", s, s.Display, raw)
			}
		case contract.SegHidden:
			if s.Display != "" {
				fail("hidden segment %+v shows text", s)
			}
		case contract.SegObject:
			if s.Display != "￼" || s.Object == nil {
				fail("object segment %+v", s)
			}
		}
		if mode == contract.RevealNone && s.Kind == contract.SegMarker {
			fail("a marker in reading mode: %+v", s)
		}
		display.WriteString(s.Display)
	}
	if at != b.Trailer.End {
		fail("the segments end at %d, want %d", at, b.Trailer.End)
	}
	if pr.DisplayText(p) != display.String() {
		fail("DisplayText %q, segments %q", pr.DisplayText(p), display.String())
	}
	// docs/04 §4.3.3 and §4.3.7.
	prev := -1
	for off := b.Lead.Start; off <= b.Trailer.End; off++ {
		for _, aff := range []contract.Affinity{contract.Upstream, contract.Downstream} {
			d := pr.ToDisplay(p, off, aff)
			if d < prev && aff == contract.Downstream {
				fail("ToDisplay(%d) = %d after %d", off, d, prev)
			}
			if aff == contract.Downstream {
				prev = d
			}
			snap := pr.Snap(p, off, aff)
			if snap < b.Range.Start || snap > b.Range.End {
				fail("Snap(%d) = %d outside %+v", off, snap, b.Range)
			}
			if inside(p, snap) {
				fail("Snap(%d) = %d is inside a hidden part or an object", off, snap)
			}
			if got := pr.ToSource(p, d, aff); got != snap {
				fail("ToSource(ToDisplay(%d)) = %d, Snap = %d (affinity %d)", off, got, snap, aff)
			}
		}
	}
}

// inside reports whether off is strictly inside a hidden or object segment.
func inside(p *contract.BlockProjection, off int) bool {
	for _, s := range p.Segments {
		if (s.Kind == contract.SegHidden || s.Kind == contract.SegObject || s.Kind == contract.SegReplaced) && off > s.Src.Start && off < s.Src.End {
			return true
		}
	}
	return false
}

// walkCaret moves the caret one display position at a time through a leaf, both
// ways, projecting again at each step as the editor does with inline reveal.
// The caret must always move, must not stall, and must reach every visible text.
func walkCaret(t *testing.T, pr contract.Projector, src string, ast contract.AST, text contract.Text, leaf int) {
	t.Helper()
	b := ast.Block(leaf)
	project := func(caret int) contract.BlockProjection {
		return pr.Project(ast, text, leaf, contract.Reveal{Mode: contract.RevealInline, Caret: caret})
	}
	start := project(b.Range.Start)
	first := pr.Snap(&start, b.Range.Start, contract.Downstream)
	endP := project(b.Range.End)
	last := pr.Snap(&endP, b.Range.End, contract.Upstream)

	visited := map[int]bool{first: true}
	caret, limit := first, 4*(b.Range.Len()+2)
	for caret < last {
		p := project(caret)
		d := pr.ToDisplay(&p, caret, contract.Downstream)
		next := pr.ToSource(&p, d+displayStep(pr.DisplayText(&p), d), contract.Downstream)
		if next <= caret {
			t.Fatalf("source %q, leaf %d: the caret stalled at %d (display %q)", src, leaf, caret, pr.DisplayText(&p))
		}
		caret = next
		visited[caret] = true
		if limit--; limit < 0 {
			t.Fatalf("source %q, leaf %d: the walk does not end", src, leaf)
		}
	}
	// Every visible character of the resting display can hold the caret.
	rest := pr.Project(ast, text, leaf, contract.Reveal{Mode: contract.RevealInline, Caret: -1})
	for _, s := range rest.Segments {
		if s.Kind != contract.SegText || s.Src.Start < first || s.Src.Start >= last {
			continue
		}
		if !visited[s.Src.Start] && !nearVisited(visited, s.Src.Start) {
			t.Fatalf("source %q, leaf %d: the walk skipped the text at %d", src, leaf, s.Src.Start)
		}
	}
	for caret > first {
		p := project(caret)
		d := pr.ToDisplay(&p, caret, contract.Upstream)
		if d == 0 {
			t.Fatalf("source %q, leaf %d: no way left from %d", src, leaf, caret)
		}
		next := pr.ToSource(&p, d-displayStepBack(pr.DisplayText(&p), d), contract.Upstream)
		if next >= caret {
			t.Fatalf("source %q, leaf %d: the caret stalled going left at %d (display %q)", src, leaf, caret, pr.DisplayText(&p))
		}
		caret = next
		if limit--; limit < -4*(b.Range.Len()+2) {
			t.Fatalf("source %q, leaf %d: the walk back does not end", src, leaf)
		}
	}
}

// nearVisited allows for a position that only differs by zero-width markers.
func nearVisited(visited map[int]bool, off int) bool {
	for d := 1; d <= 8; d++ {
		if visited[off-d] {
			return true
		}
	}
	return false
}

// displayStep returns the UTF-16 length of the character at UTF-16 index d.
func displayStep(s string, d int) int {
	i := 0
	for _, r := range s {
		w := 1
		if r >= 0x10000 {
			w = 2
		}
		if i >= d {
			return w
		}
		i += w
	}
	return 1
}

// displayStepBack returns the UTF-16 length of the character before index d.
func displayStepBack(s string, d int) int {
	i, last := 0, 1
	for _, r := range s {
		if i >= d {
			break
		}
		last = 1
		if r >= 0x10000 {
			last = 2
		}
		i += last
	}
	return last
}
