package markdown

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"aurink/internal/contract"
)

func TestTyporaBlankRunsAndLeafOwners(t *testing.T) {
	for _, nl := range []string{"\n", "\r\n"} {
		for n := 0; n <= 10; n++ {
			for _, end := range []bool{false, true} {
				src := "a" + nl + strings.Repeat(nl, n)
				if !end {
					src += "b"
				}
				s := mustNew(t, src, 1, contract.ExtDefault).Snapshot()
				checkCoverage(t, s)
				var got, want []int
				for i := range s.Leaves() {
					if b := s.Block(i); b.Kind == contract.BlockBlank {
						got = append(got, b.Range.Start)
					}
				}
				for line := 1; line <= n && (end || line < n-1); line += 2 {
					want = append(want, 1+(line+1)*len(nl))
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("%q end=%v: blank starts %v, want %v", src, end, got, want)
				}
				for offset := 0; offset <= len(src); offset++ {
					b := s.Block(s.LeafAt(offset))
					if offset < b.Lead.Start || offset > b.Trailer.End || offset == b.Trailer.End && offset != len(src) {
						t.Fatalf("%q at %d owned by %+v", src, offset, b)
					}
				}
			}
		}
		src := "a" + nl + nl + nl + "b"
		s := mustNew(t, src, 1, 0).Snapshot()
		for _, at := range []int{1 + len(nl), 1 + 2*len(nl)} {
			b := s.Block(s.LeafAt(at))
			if b.Kind != contract.BlockParagraph || b.Range.Start != 0 {
				t.Fatalf("suppressed separator at %d maps to %+v", at, b)
			}
		}
	}
}

func TestTyporaContainerBlankLeavesRemain(t *testing.T) {
	for _, tc := range []struct {
		src        string
		start, end int
		parent     contract.BlockKind
	}{
		{"> a\n>\n>\n> b", 7, 7, contract.BlockQuote},
		{"- a\n  \n  \n  b", 7, 9, contract.BlockItem},
		{"[^n]: a\n    \n    \n    b", 13, 17, contract.BlockFootnoteDef},
		{"- > a\n  >\n  >\n  > b", 13, 13, contract.BlockQuote},
	} {
		for _, nl := range []string{"\n", "\r\n"} {
			src := strings.ReplaceAll(tc.src, "\n", nl)
			at := func(p int) int { return p + strings.Count(tc.src[:p], "\n")*(len(nl)-1) }
			s := mustNew(t, src, 1, contract.ExtDefault).Snapshot()
			checkCoverage(t, s)
			b := s.Block(s.LeafAt(at(tc.end)))
			if b.Kind != contract.BlockBlank || b.Range != (contract.Range{Start: at(tc.start), End: at(tc.end)}) || s.Block(b.Parent).Kind != tc.parent {
				t.Fatalf("%q: blank=%+v parent=%+v", src, b, s.Block(b.Parent))
			}
		}
	}
}

func TestTyporaBlankRunEditsMatchFullParse(t *testing.T) {
	for _, nl := range []string{"\n", "\r\n"} {
		for _, wrap := range [][2]string{{"a" + nl, "b"}, {"", "b"}, {"a" + nl, ""}, {"> a" + nl + "> ", nl + "> b"}, {"- a" + nl + "  ", nl + "  b"}} {
			t.Run(fmt.Sprintf("%q/%q/%q", nl, wrap[0], wrap[1]), func(t *testing.T) {
				src := wrap[0] + wrap[1]
				p := mustNew(t, src, 1, contract.ExtDefault)
				at := len(wrap[0])
				for step := 0; step < 20; step++ {
					edit := contract.TextEdit{Range: contract.Range{Start: at, End: at}, Text: nl}
					if step >= 10 {
						edit = contract.TextEdit{Range: contract.Range{Start: at - len(nl), End: at}}
					}
					u, err := p.Apply([]contract.TextEdit{edit})
					if err != nil {
						t.Fatal(err)
					}
					src = src[:edit.Range.Start] + edit.Text + src[edit.Range.End:]
					at += len(edit.Text) - edit.Range.Len()
					full := mustNew(t, src, u.Snapshot.Version(), contract.ExtDefault).Snapshot()
					if got, want := differentialDump(u.Snapshot), differentialDump(full); got != want {
						t.Fatalf("step %d source %q:\nincremental %s\nfull %s", step, src, got, want)
					}
					checkCoverage(t, u.Snapshot)
				}
			})
		}
	}
}

func TestTyporaUnclosedMathInContainers(t *testing.T) {
	for _, nl := range []string{"\n", "\r\n"} {
		for _, pair := range [][2]string{
			{"$$\nwords\n\n# after", "$$\nwords\n$$\n\n# after"},
			{"> $$\n> words\n>\n> # after", "> $$\n> words\n> $$\n>\n> # after"},
			{"- $$\n  words\n\n  # after", "- $$\n  words\n  $$\n\n  # after"},
			{"> $$\n\n# after", "> $$\n> $$\n\n# after"},
			{"- $$\n\n# after", "- $$\n  $$\n\n# after"},
		} {
			for closed, input := range pair {
				src := strings.ReplaceAll(input, "\n", nl)
				for _, ext := range []contract.Extensions{0, contract.ExtDefault} {
					s := mustNew(t, src, 1, ext).Snapshot()
					checkCoverage(t, s)
					math, headings := 0, 0
					for i := range s.Leaves() {
						switch s.Block(i).Kind {
						case contract.BlockMath:
							math++
						case contract.BlockHeading:
							headings++
						}
					}
					if math != closed || headings != 1 {
						t.Errorf("%q ext=%v: math=%d headings=%d", src, ext, math, headings)
					}
				}
			}
		}
		s := mustNew(t, "$$x$$"+nl, 1, contract.ExtDefault).Snapshot()
		i := s.LeafAt(0)
		if s.Block(i).Kind != contract.BlockParagraph || len(s.Inlines(i)) != 1 || s.Inlines(i)[0].Kind != contract.InlineMath {
			t.Fatalf("one-line math changed: %+v %+v", s.Block(i), s.Inlines(i))
		}
	}
}

func TestTyporaMathDelimiterEditsMatchFullParse(t *testing.T) {
	for _, nl := range []string{"\n", "\r\n"} {
		for _, wrap := range [][2]string{{"", ""}, {"> ", "> "}, {"- ", "  "}} {
			src := wrap[0] + "$$" + nl + wrap[1] + "x" + nl + wrap[1] + "$$" + nl + nl + "# after"
			p := mustNew(t, src, 1, contract.ExtDefault)
			at := strings.LastIndex(src, "$$")
			for _, remove := range []bool{true, false, true, false} {
				edit := contract.TextEdit{Range: contract.Range{Start: at, End: at}, Text: "$$"}
				if remove {
					edit = contract.TextEdit{Range: contract.Range{Start: at, End: at + 2}}
				}
				u, err := p.Apply([]contract.TextEdit{edit})
				if err != nil {
					t.Fatal(err)
				}
				src = src[:edit.Range.Start] + edit.Text + src[edit.Range.End:]
				full := mustNew(t, src, u.Snapshot.Version(), contract.ExtDefault).Snapshot()
				if got, want := differentialDump(u.Snapshot), differentialDump(full); got != want {
					t.Fatalf("source %q:\nincremental %s\nfull %s", src, got, want)
				}
				checkCoverage(t, u.Snapshot)
			}
		}
	}
}

func TestTyporaDistantMathCloserEdits(t *testing.T) {
	for _, nl := range []string{"\n", "\r\n"} {
		src := "$$" + nl + nl + "# visible" + nl + nl + strings.Repeat("paragraph"+nl+nl, 8) + "close$"
		p := mustNew(t, src, 1, contract.ExtDefault)
		for _, insert := range []bool{true, false} {
			at := len(src)
			edit := contract.TextEdit{Range: contract.Range{Start: at, End: at}, Text: "$"}
			if !insert {
				edit = contract.TextEdit{Range: contract.Range{Start: at - 1, End: at}}
			}
			u, err := p.Apply([]contract.TextEdit{edit})
			if err != nil {
				t.Fatal(err)
			}
			src = src[:edit.Range.Start] + edit.Text + src[edit.Range.End:]
			full := mustNew(t, src, u.Snapshot.Version(), contract.ExtDefault).Snapshot()
			if got, want := differentialDump(u.Snapshot), differentialDump(full); got != want {
				t.Fatalf("source %q:\nincremental %s\nfull %s", src, got, want)
			}
			if got := u.Snapshot.Block(u.Snapshot.LeafAt(0)).Kind; insert && got != contract.BlockMath || !insert && got == contract.BlockMath {
				t.Fatalf("insert=%v first block kind=%v", insert, got)
			}
		}
	}
}

func TestTyporaMathCloserInAnotherContainer(t *testing.T) {
	for _, nl := range []string{"\n", "\r\n"} {
		src := strings.ReplaceAll("> $$\n> unfinished\n\n$$\nclosed\n$$\n\n# after", "\n", nl)
		s := mustNew(t, src, 1, contract.ExtDefault).Snapshot()
		checkCoverage(t, s)
		first := s.Block(s.LeafAt(strings.Index(src, "unfinished")))
		second := s.Block(s.LeafAt(strings.Index(src, "closed")))
		if first.Kind != contract.BlockParagraph || second.Kind != contract.BlockMath {
			t.Fatalf("different container: first=%+v second=%+v", first, second)
		}
	}
}

func TestUnclosedMathAndEmptyTaskTogether(t *testing.T) {
	for _, input := range []string{
		"$$\nwords\n\n- [ ] \n\n# after",
		"- [x] \n\n$$\nwords\n\n# after",
		"- [ ] \n  continuation\n\n$$\nwords\n\n# after",
	} {
		for _, nl := range []string{"\n", "\r\n"} {
			for _, ext := range []contract.Extensions{0, contract.ExtDefault} {
				src := strings.ReplaceAll(input, "\n", nl)
				s := mustNew(t, src, 1, ext).Snapshot()
				checkCoverage(t, s)
				var task, math, heading int
				for i := 0; i < s.Len(); i++ {
					b := s.Block(i)
					if b.Kind == contract.BlockItem && b.Task != contract.TaskNone {
						task++
					}
					if b.Kind == contract.BlockMath {
						math++
					}
					if b.Kind == contract.BlockHeading {
						heading++
					}
				}
				if task != 1 || math != 0 || heading != 1 || s.text.String() != src {
					t.Fatalf("%q ext=%v: task=%d math=%d heading=%d", src, ext, task, math, heading)
				}
			}
		}
	}
}
