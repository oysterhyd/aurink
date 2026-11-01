package contracttest

import (
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"sync"
	"testing"
	"unicode"
	"unicode/utf8"

	"aurink/internal/contract"
	"aurink/tests/corpus"
)

// ASTFixture parses documents for the AST suite.
type ASTFixture struct {
	// Parse returns the AST of src.
	Parse func(src string, exts contract.Extensions) contract.AST
	// Edit applies ordered edits of the source of prev incrementally. It returns
	// the AST of the next version and the new source; prev stays valid.
	Edit func(prev contract.AST, src string, edits []contract.TextEdit) (contract.AST, string)
}

var leafKinds = map[contract.BlockKind]bool{
	contract.BlockParagraph:     true,
	contract.BlockHeading:       true,
	contract.BlockThematicBreak: true,
	contract.BlockCode:          true,
	contract.BlockHTML:          true,
	contract.BlockMath:          true,
	contract.BlockTableCell:     true,
	contract.BlockFrontMatter:   true,
	contract.BlockTOC:           true,
	contract.BlockBlank:         true,
	contract.BlockLinkDef:       true,
}

var astSamples = []string{
	"",
	"a",
	"\n",
	"\n\n\n",
	"# 标题\n\n正文 **粗** *斜* `码` [链](u \"t\") \\* &amp;\n",
	"a\r\n\r\n\r\nb\r\n",
	"> 引\n> 用\n>\n> - 项\n>   续\n",
	"- [ ] 待办\n- [x] 完成\n  - 嵌套\n\n1. 一\n2. 二\n",
	"```go\nfunc f() {}\n```\n\n~~~\n未闭合\n",
	"    缩进\n\tTab\n",
	"| a | b |\n|---|:-:|\n| 1 | 2 |\n",
	"| a |\n|---|\n| 1 | 多余 |\n",
	"> [ref]: /u\n",
	"正文[^1]\n\n[^1]: 脚注\n\n[ref]: /u \"t\"\n",
	"---\ntitle: x\n---\n\n# h\n",
	"$$\nx\n$$\n",
	"<div>\nhtml\n</div>\n",
	"---\n",
	"-\n\n>\n",
}

// astCorpus returns astSamples and the Markdown of the corpus.
func astCorpus(t *testing.T) []string {
	t.Helper()
	entries, err := corpus.Index()
	if err != nil {
		t.Fatal(err)
	}
	docs := append([]string(nil), astSamples...)
	for _, e := range entries {
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

// TestAST checks docs/04 §4.2 for an AST. The expected results of a parse are
// checked as invariants over the corpus; golden trees belong to the implementation.
func TestAST(t *testing.T, f ASTFixture) {
	t.Run("invariants", func(t *testing.T) {
		for _, src := range astCorpus(t) {
			checkAST(t, src, f.Parse(src, contract.ExtFrontMatter))
		}
	})
	t.Run("hidden text", func(t *testing.T) {
		for _, src := range astCorpus(t) {
			checkNoHiddenText(t, src, f.Parse(src, contract.ExtFrontMatter))
		}
	})
	t.Run("leaves for hidden definitions and cells", func(t *testing.T) {
		if a := f.Parse("> [ref]: /u\n", 0); !hasLeaf(a, contract.BlockLinkDef) {
			t.Error("a link definition in a quote has no BlockLinkDef leaf")
		}
		src := "| a |\n|---|\n| 1 | 多余 |\n"
		a := f.Parse(src, 0)
		found := false
		for i := range a.Leaves() {
			b := a.Block(i)
			found = found || (b.Kind == contract.BlockTableCell && strings.Contains(src[b.Range.Start:b.Range.End], "多余"))
		}
		if !found {
			t.Error("the extra table cell has no leaf")
		}
	})
	t.Run("blank lines", func(t *testing.T) {
		a := f.Parse("", 0)
		if a.LeafCount() != 1 || a.Block(a.LeafAt(0)).Kind != contract.BlockBlank {
			t.Errorf("the empty document has %d leaves", a.LeafCount())
		}
		a = f.Parse("a\n\n\n\nb\n", 0)
		if n := countLeaves(a, contract.BlockBlank); n != 2 { // the first blank line is a's Trailer
			t.Errorf("%d blank leaves, want 2", n)
		}
	})
	t.Run("front matter", func(t *testing.T) {
		src := "---\ntitle: x\n---\n# h\n"
		a := f.Parse(src, contract.ExtFrontMatter)
		if b, ok := a.FrontMatter(); !ok || b.Range != (contract.Range{Start: 0, End: 17}) {
			t.Errorf("front matter %+v, %v", b, ok)
		}
		if _, ok := f.Parse(src, 0).FrontMatter(); ok {
			t.Error("front matter without the extension")
		}
		if _, ok := f.Parse("\n"+src, contract.ExtFrontMatter).FrontMatter(); ok {
			t.Error("front matter not on the first line")
		}
	})
	t.Run("inlines", func(t *testing.T) {
		src := "**粗** \\* &amp; [链](u)\n"
		a := f.Parse(src, 0)
		got := map[contract.InlineKind]contract.Inline{}
		for _, n := range a.Inlines(a.LeafAt(0)) {
			got[n.Kind] = n
		}
		if n := got[contract.InlineStrong]; src[n.Open.Start:n.Open.End] != "**" || src[n.Close.Start:n.Close.End] != "**" {
			t.Errorf("strong %+v", n)
		}
		if n := got[contract.InlineEscape]; n.Text != "*" || src[n.Open.Start:n.Open.End] != "\\" {
			t.Errorf("escape %+v", n)
		}
		if n := got[contract.InlineEntity]; n.Text != "&" || src[n.Range.Start:n.Range.End] != "&amp;" {
			t.Errorf("entity %+v", n)
		}
		if n := got[contract.InlineLink]; n.Dest != "u" || src[n.Close.Start:n.Close.End] != "](u)" {
			t.Errorf("link %+v", n)
		}
	})
	t.Run("outline and tables", func(t *testing.T) {
		a := f.Parse("# A b\n\n## A b\n\n# 中文 标题!\n\n[r]: /u \"t\"\n\nx[^n]\n\n[^n]: y\n", 0)
		var slugs []string
		for _, h := range a.Outline() {
			slugs = append(slugs, h.Slug)
		}
		if got := strings.Join(slugs, " "); got != "a-b a-b-1 中文-标题" {
			t.Errorf("slugs %q", got)
		}
		if d, ok := a.LinkDefs()["r"]; !ok || d.Dest != "/u" || d.Title != "t" {
			t.Errorf("link definitions %+v", a.LinkDefs())
		}
		if _, ok := a.Footnotes()["n"]; !ok {
			t.Errorf("footnotes %+v", a.Footnotes())
		}
	})
	t.Run("incremental equals full", func(t *testing.T) {
		for _, seed := range []int64{1331, 20261101, 73} {
			randomEdits(t, f, seed, 1500, func(a contract.AST, src string) {
				if got, want := dumpAST(a), dumpAST(f.Parse(src, contract.ExtFrontMatter)); got != want {
					t.Fatalf("seed %d, source %q:\nincremental %s\nfull %s", seed, src, got, want)
				}
				checkAST(t, src, a)
			})
		}
	})
	t.Run("identity", func(t *testing.T) {
		src := "第一段\n\n第二段\n\n第三段\n"
		a := f.Parse(src, 0)
		ids := leafIDs(a)
		// A block before an edit, the edited block and a shifted block all survive.
		at := strings.Index(src, "第二段")
		b, next := f.Edit(a, src, []contract.TextEdit{{Range: contract.Range{Start: at, End: at}, Text: "新"}})
		if b.Version() != a.Version()+1 {
			t.Errorf("version %d after %d", b.Version(), a.Version())
		}
		if got := leafIDs(b); fmt.Sprint(got) != fmt.Sprint(ids) {
			t.Errorf("ids %v after typing, were %v", got, ids)
		}
		// An open fence swallows what follows: those blocks do not keep an identity
		// and the new block's ID is never one used before.
		c, _ := f.Edit(b, next, []contract.TextEdit{{Range: contract.Range{}, Text: "```\n"}})
		seen := map[contract.BlockID]bool{}
		for _, id := range ids {
			seen[id] = true
		}
		if c.LeafCount() >= b.LeafCount() {
			t.Errorf("%d leaves after the fence", c.LeafCount())
		}
		for _, id := range leafIDs(c) {
			if seen[id] {
				t.Errorf("id %d survived being swallowed or was reused", id)
			}
		}
	})
	t.Run("ids are unique and never reused", func(t *testing.T) {
		retired := map[contract.BlockID]bool{}
		var prev map[contract.BlockID]bool
		randomEdits(t, f, 7, 1500, func(a contract.AST, _ string) {
			now := map[contract.BlockID]bool{}
			for i := 0; i < a.Len(); i++ {
				id := a.Block(i).ID
				if id == 0 || now[id] {
					t.Fatalf("id %d is zero or repeated", id)
				}
				if retired[id] {
					t.Fatalf("retired id %d came back", id)
				}
				if j, ok := a.Index(id); !ok || j != i {
					t.Fatalf("Index(%d) = %d, %v, want %d", id, j, ok, i)
				}
				now[id] = true
			}
			for id := range prev {
				if !now[id] {
					retired[id] = true
				}
			}
			prev = now
		})
	})
	t.Run("old snapshots do not change", func(t *testing.T) {
		src := corpus.Generate(16<<10, 5)
		a := f.Parse(src, 0)
		want := dumpAST(a)
		cur, next := a, src
		rng := rand.New(rand.NewSource(11))
		var wg sync.WaitGroup
		for range 4 {
			wg.Go(func() {
				for i := 0; i < a.Len(); i += 17 {
					_ = a.Block(i).Range
					if leafKinds[a.Block(i).Kind] {
						_ = a.Inlines(i)
					}
				}
			})
		}
		for range 200 {
			cur, next = f.Edit(cur, next, []contract.TextEdit{randomEdit(rng, next)})
		}
		wg.Wait()
		if got := dumpAST(a); got != want {
			t.Fatal("the first snapshot changed after later edits")
		}
	})
}

var editFragments = []string{
	"a", "中", "🐈", " ", "\n", "\n\n", "\r\n", "\t", "**", "*", "`", "```\n", "> ", "- ", "1. ", "# ",
	"| a |\n|---|\n", "[r]: /u\n", "[^x]: y\n", "[r]", "---\n", "$$\n", "<div>\n", "\\*", "&amp;",
}

// randomEdit returns an edit on UTF-8 boundaries of src.
func randomEdit(rng *rand.Rand, src string) contract.TextEdit {
	boundary := func(i int) int {
		for i > 0 && i < len(src) && !utf8.RuneStart(src[i]) {
			i--
		}
		return i
	}
	start := boundary(rng.Intn(len(src) + 1))
	end := start
	if rng.Intn(2) == 0 {
		end = boundary(min(len(src), start+rng.Intn(12)))
		end = max(end, start)
	}
	text := ""
	if end == start || rng.Intn(2) == 0 {
		text = editFragments[rng.Intn(len(editFragments))]
	}
	return contract.TextEdit{Range: contract.Range{Start: start, End: end}, Text: text}
}

func randomEdits(t *testing.T, f ASTFixture, seed int64, steps int, check func(contract.AST, string)) {
	t.Helper()
	rng := rand.New(rand.NewSource(seed))
	src := "# 标题\n\n正文 **粗** [r]\n\n> 引用\n\n- 项\n\n```\n码\n```\n\n[r]: /u\n"
	a := f.Parse(src, contract.ExtFrontMatter)
	for range steps {
		edit := randomEdit(rng, src)
		if edit.Range.Empty() && edit.Text == "" {
			continue
		}
		a, src = f.Edit(a, src, []contract.TextEdit{edit})
		check(a, src)
	}
}

func checkAST(t *testing.T, src string, a contract.AST) {
	t.Helper()
	fail := func(format string, args ...any) {
		t.Helper()
		t.Fatalf("source %q: %s", src, fmt.Sprintf(format, args...))
	}
	if a.Len() == 0 || a.Block(0).Kind != contract.BlockRoot || a.Block(0).Parent != -1 || a.Block(0).Next != a.Len() {
		fail("bad root")
	}
	for i := 1; i < a.Len(); i++ {
		b, p := a.Block(i), a.Block(a.Block(i).Parent)
		if b.Parent < 0 || b.Parent >= i || b.Next <= i || b.Next > p.Next {
			fail("block %d: parent %d, next %d", i, b.Parent, b.Next)
		}
		if b.Range.Start < p.Range.Start && p.Kind != contract.BlockRoot {
			fail("block %d starts before its parent", i)
		}
		for _, l := range b.Lines {
			if l.Padding > 3 || l.Source.Start < b.Range.Start || l.Source.End > b.Range.End {
				fail("block %d: line %+v, range %+v", i, l, b.Range)
			}
		}
	}
	// docs/04 §4.2.8: the leaves cover the document without a gap or an overlap.
	at, n := 0, 0
	for i := range a.Leaves() {
		b := a.Block(i)
		if !leafKinds[b.Kind] {
			fail("leaf %d has kind %d", i, b.Kind)
		}
		if b.Lead.Start != at || b.Lead.End != b.Range.Start || b.Range.End != b.Trailer.Start || b.Trailer.End < b.Trailer.Start {
			fail("leaf %d at %d: lead %+v, range %+v, trailer %+v", i, at, b.Lead, b.Range, b.Trailer)
		}
		for off := b.Lead.Start; off < b.Trailer.End; off++ {
			if a.LeafAt(off) != i {
				fail("LeafAt(%d) = %d, want %d", off, a.LeafAt(off), i)
			}
		}
		at = b.Trailer.End
		n++
		for j, in := range a.Inlines(i) {
			if in.Range.Start < b.Lead.Start || in.Range.End > b.Trailer.End || in.Next <= j {
				fail("leaf %d: inline %d %+v", i, j, in)
			}
			if !in.Open.Empty() && !in.Close.Empty() && in.Open.End > in.Close.Start {
				fail("leaf %d: inline %d delimiters %+v", i, j, in)
			}
		}
	}
	if at != len(src) || n != a.LeafCount() || n == 0 {
		fail("the leaves end at %d of %d; %d leaves, LeafCount %d", at, len(src), n, a.LeafCount())
	}
	if p := a.Path(len(src)); len(p) == 0 || p[0] != 0 || p[len(p)-1] != a.LeafAt(len(src)) {
		fail("Path(end) = %v", p)
	}
}

var footnoteLabel = regexp.MustCompile(`\[\^[^\]]*\]:`)

// checkNoHiddenText asserts that Lead and Trailer hold only container syntax,
// white space and line endings, so that live mode shows all the text.
func checkNoHiddenText(t *testing.T, src string, a contract.AST) {
	t.Helper()
	for i := range a.Leaves() {
		b := a.Block(i)
		for _, r := range []contract.Range{b.Lead, b.Trailer} {
			hidden := footnoteLabel.ReplaceAllString(src[r.Start:r.End], "")
			for _, c := range hidden {
				if unicode.IsLetter(c) && c != 'x' && c != 'X' {
					t.Fatalf("source %q: text in the hidden part %q of leaf %d", src, src[r.Start:r.End], i)
				}
			}
		}
	}
}

func hasLeaf(a contract.AST, k contract.BlockKind) bool { return countLeaves(a, k) > 0 }

func countLeaves(a contract.AST, k contract.BlockKind) int {
	n := 0
	for i := range a.Leaves() {
		if a.Block(i).Kind == k {
			n++
		}
	}
	return n
}

func leafIDs(a contract.AST) []contract.BlockID {
	var ids []contract.BlockID
	for i := range a.Leaves() {
		ids = append(ids, a.Block(i).ID)
	}
	return ids
}

// dumpAST prints everything of an AST except the IDs.
func dumpAST(a contract.AST) string {
	var s strings.Builder
	for i := 0; i < a.Len(); i++ {
		b := *a.Block(i)
		b.ID = 0
		fmt.Fprintf(&s, "%+v\n", b)
		if leafKinds[b.Kind] {
			fmt.Fprintf(&s, "%+v\n", a.Inlines(i))
		}
	}
	return s.String()
}
