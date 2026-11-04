package markdown

import (
	"strings"
	"testing"

	"aurink/internal/contract"
)

func TestDoubleDollarInline(t *testing.T) {
	for _, tc := range []struct {
		name, src, math, tex string
		block                contract.BlockKind
	}{
		{"standalone", "$$abc$$\n", "$$abc$$", "abc", contract.BlockParagraph},
		{"spaces", "$$ x + y $$\n", "$$ x + y $$", " x + y ", contract.BlockParagraph},
		{"surrounded", "text $$y$$ text\n", "$$y$$", "y", contract.BlockParagraph},
		{"quote", "> $$x$$\n", "$$x$$", "x", contract.BlockParagraph},
		{"list", "- $$x$$\n", "$$x$$", "x", contract.BlockParagraph},
		{"heading", "# $$x$$\n", "$$x$$", "x", contract.BlockHeading},
		{"table", "| $$x$$ |\n|---|\n", "$$x$$", "x", contract.BlockTableCell},
		{"reference context", "$$x$$ [ref]\n\n[ref]: /url\n", "$$x$$", "x", contract.BlockParagraph},
		{"reference after guarded line", "$$x$$\n\n[ref]: /url\n", "$$x$$", "x", contract.BlockParagraph},
		{"multiline paragraph", "$$x$$\ncontinued **bold**\n", "$$x$$", "x", contract.BlockParagraph},
		{"multiline with references", "$$x$$\ncontinued [ref]\n\n[ref]: /url\n", "$$x$$", "x", contract.BlockParagraph},
		{"nested emphasis", "**$$x$$**\n", "$$x$$", "x", contract.BlockParagraph},
		{"crlf", "> $$甲$$\r\n", "$$甲$$", "甲", contract.BlockParagraph},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := mustNew(t, tc.src, 1, contract.ExtDollarMath).Snapshot()
			if s.text.String() != tc.src {
				t.Fatal("source changed")
			}
			checkCoverage(t, s)
			at := strings.Index(tc.src, tc.math)
			i := s.LeafAt(at)
			if got := s.Block(i).Kind; got != tc.block {
				t.Fatalf("block = %v, want %v", got, tc.block)
			}
			var matches int
			for _, n := range s.Inlines(i) {
				if n.Kind != contract.InlineMath {
					continue
				}
				matches++
				if s.text.Slice(n.Range) != tc.math || s.text.Slice(n.Open) != "$$" || s.text.Slice(n.Close) != "$$" || n.Text != tc.tex || n.Display {
					t.Fatalf("math = %+v", n)
				}
			}
			if matches != 1 {
				t.Fatalf("got %d inline math nodes: %+v", matches, s.Inlines(i))
			}
		})
	}
}

func TestDoubleDollarBlankIsText(t *testing.T) {
	for _, src := range []string{"$$$$\n", "$$ $$\n", "$$\t$$\n", "$$\u2003$$\n", "text $$ $$ tail\n", "> $$$$\n"} {
		t.Run(src, func(t *testing.T) {
			s := mustNew(t, src, 1, contract.ExtDollarMath).Snapshot()
			checkCoverage(t, s)
			for i := range s.Leaves() {
				if b := s.Block(i); b.Kind == contract.BlockMath {
					t.Fatalf("blank math became block: %+v", b)
				}
				for _, n := range s.Inlines(i) {
					if n.Kind == contract.InlineMath {
						t.Fatalf("blank math became inline: %+v", n)
					}
				}
			}
		})
	}
}

func TestDollarOpenerNeedsItsOwnLine(t *testing.T) {
	for _, src := range []string{"$$", "$$  ", "$$unfinished\nnext\n", "$$ x + y\nnext\n"} {
		s := mustNew(t, src, 1, contract.ExtDollarMath).Snapshot()
		checkCoverage(t, s)
		for i := range s.Leaves() {
			if s.Block(i).Kind == contract.BlockMath {
				t.Errorf("%q opened a math block", src)
			}
		}
	}
}

func TestDoubleDollarDoesNotSpanLines(t *testing.T) {
	for _, tc := range []struct{ src, math string }{
		{"prefix $$x *em*\nend$$ suffix\n", ""},
		{"prefix $$x *em*\nend$$good$$ suffix\n", "$$good$$"},
		{"> prefix $$x *em*\n> end$$good$$ suffix\n", "$$good$$"},
		{"prefix $$x *em*\nend$$good$$ [ref]\n\n[ref]: /url\n", "$$good$$"},
	} {
		s := mustNew(t, tc.src, 1, contract.ExtDollarMath).Snapshot()
		checkCoverage(t, s)
		emphasis, math := false, ""
		for _, n := range s.Inlines(s.LeafAt(strings.Index(tc.src, "prefix"))) {
			if n.Kind == contract.InlineMath {
				math = s.text.Slice(n.Range)
			}
			emphasis = emphasis || n.Kind == contract.InlineEmphasis
		}
		if !emphasis || math != tc.math {
			t.Errorf("source %q: emphasis %v math %q, want %q; inlines %+v", tc.src, emphasis, math, tc.math, s.Inlines(s.LeafAt(strings.Index(tc.src, "prefix"))))
		}
	}
}

func TestDollarMathKeepsOtherBlocksAndLiterals(t *testing.T) {
	for _, src := range []string{"`$$x$$`\n", "```\n$$x$$\n```\n", "`a\n$$x$$\nb`\n"} {
		s := mustNew(t, src, 1, contract.ExtDollarMath).Snapshot()
		checkCoverage(t, s)
		for i := range s.Leaves() {
			if s.Block(i).Kind == contract.BlockMath {
				t.Fatalf("math block in code: %q", src)
			}
			for _, n := range s.Inlines(i) {
				if n.Kind == contract.InlineMath {
					t.Fatalf("inline math in code: %q", src)
				}
				if n.Kind == contract.InlineCode && strings.Contains(src, "`a\n") && n.Text != "a $$x$$ b" {
					t.Fatalf("code text changed: %q", n.Text)
				}
			}
		}
	}
	for _, src := range []string{"$$\nx + y $$\nafter\n", "> $$\n> x\nafter\n"} {
		s := mustNew(t, src, 1, contract.ExtDollarMath).Snapshot()
		checkCoverage(t, s)
		if s.Block(s.LeafAt(strings.Index(src, "after"))).Kind != contract.BlockParagraph {
			t.Errorf("math swallowed following paragraph: %q", src)
		}
	}
}

func TestManyUnclosedDollarLines(t *testing.T) {
	src := strings.Repeat("$$unfinished\n", 2000) + "end\n"
	s := mustNew(t, src, 1, contract.ExtDollarMath).Snapshot()
	checkCoverage(t, s)
	if s.Block(s.LeafAt(0)).Kind != contract.BlockParagraph || s.Block(s.LeafAt(len(src)-1)).Kind != contract.BlockParagraph {
		t.Fatal("unclosed inline delimiters opened a block")
	}
	for _, n := range s.Inlines(s.LeafAt(0)) {
		if n.Kind == contract.InlineMath {
			t.Fatalf("math spans lines: %+v", n)
		}
	}
}

func TestMathBlocksAndDisabledDollarExtension(t *testing.T) {
	for _, src := range []string{
		"$$\nx + y\n$$\n", "  $$  \nx + y $$\n", "> $$\n> x\n> $$\n", "- $$\n  x\n  $$\n", "\\[\nx\n\\]\n",
	} {
		for _, ext := range []contract.Extensions{0, contract.ExtDollarMath} {
			s := mustNew(t, src, 1, ext).Snapshot()
			checkCoverage(t, s)
			found := false
			for i := range s.Leaves() {
				found = found || s.Block(i).Kind == contract.BlockMath
			}
			if !found {
				t.Errorf("extension %v: missing block in %q", ext, src)
			}
		}
	}
	for _, src := range []string{"$$abc$$\n", "$$ x $$\n"} {
		s := mustNew(t, src, 1, 0).Snapshot()
		if got := s.Block(s.LeafAt(0)).Kind; got != contract.BlockMath {
			t.Errorf("extension disabled: %q is %v", src, got)
		}
	}
	s := mustNew(t, "$x$\n", 1, 0).Snapshot()
	for _, n := range s.Inlines(s.LeafAt(0)) {
		if n.Kind == contract.InlineMath {
			t.Fatal("single-dollar math enabled with extension off")
		}
	}
}

func TestDoubleDollarTypingMatchesFullParse(t *testing.T) {
	for _, tc := range []struct {
		name string
		ext  contract.Extensions
	}{{"dollar", contract.ExtDollarMath}, {"default", contract.ExtDefault}} {
		t.Run(tc.name, func(t *testing.T) {
			testDoubleDollarTyping(t, tc.ext)
		})
	}
}

func testDoubleDollarTyping(t *testing.T, ext contract.Extensions) {
	t.Helper()
	for _, wrap := range [][2]string{{"", ""}, {"> ", "\n> next\n"}, {"before\n\n", "\n\nafter\n"}} {
		src := wrap[0] + wrap[1]
		p := mustNew(t, src, 1, ext)
		at := len(wrap[0])
		check := func() {
			t.Helper()
			s := p.Snapshot()
			if s.text.String() != src {
				t.Fatal("source changed")
			}
			if err := equivalent(s, mustNew(t, src, s.Version(), ext).Snapshot()); err != nil {
				t.Fatalf("source %q: %v", src, err)
			}
			checkCoverage(t, s)
		}
		for _, r := range "$$abc$$" {
			_, err := p.Apply([]contract.TextEdit{{Range: contract.Range{Start: at, End: at}, Text: string(r)}})
			if err != nil {
				t.Fatal(err)
			}
			src = src[:at] + string(r) + src[at:]
			at++
			check()
		}
		for range "$$abc$$" {
			_, err := p.Apply([]contract.TextEdit{{Range: contract.Range{Start: at - 1, End: at}}})
			if err != nil {
				t.Fatal(err)
			}
			src = src[:at-1] + src[at:]
			at--
			check()
		}
	}
}
