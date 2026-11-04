package markdown

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"aurink/internal/contract"
)

func TestTrailingQuoteMarker(t *testing.T) {
	for _, shape := range []struct {
		first, marker string
		quotes, items int
	}{
		{"> a", ">", 1, 0},
		{"> > a", "> >", 2, 0},
		{"- > a", "  >", 1, 1},
		{" 1. > a", "    >", 1, 1},
		{"> - > a", ">   >", 2, 1},
		{"-\t> a", "\t>", 1, 1},
	} {
		for _, eol := range []string{"\n", "\r\n"} {
			for _, space := range []string{"", " "} {
				for _, after := range []string{"", eol + "next", eol + eol + "next"} {
					name := fmt.Sprintf("%q/%q/%q/%q", shape.first, eol, space, after)
					t.Run(name, func(t *testing.T) {
						marker := shape.marker + space
						src := shape.first + eol + marker + after
						start, end := len(shape.first)+len(eol), len(shape.first)+len(eol)+len(marker)
						s := mustNew(t, src, 1, contract.ExtDefault).Snapshot()
						checkCoverage(t, s)
						paragraph := s.Block(s.LeafAt(len(shape.first) - 1))
						if paragraph.Kind != contract.BlockParagraph || paragraph.Range.End != len(shape.first) || paragraph.Trailer != (contract.Range{Start: len(shape.first), End: start}) {
							t.Fatalf("paragraph = %+v", paragraph)
						}
						leaf := s.Block(s.LeafAt(end))
						if leaf.Kind != contract.BlockBlank || leaf.Range != (contract.Range{Start: end, End: end}) || leaf.Lead != (contract.Range{Start: start, End: end}) || leaf.Trailer.Start != end {
							t.Fatalf("blank = %+v", leaf)
						}
						if s.Block(leaf.Parent).Kind != contract.BlockQuote {
							t.Fatalf("blank parent = %+v", s.Block(leaf.Parent))
						}
						quotes, items := 0, 0
						for parent := leaf.Parent; parent > 0; parent = s.Block(parent).Parent {
							b := s.Block(parent)
							switch b.Kind {
							case contract.BlockQuote:
								quotes++
								if b.Range.End < end {
									t.Fatalf("quote does not cover marker: %+v", b)
								}
							case contract.BlockItem:
								items++
							}
						}
						if quotes != shape.quotes || items != shape.items {
							t.Fatalf("ancestor quotes=%d items=%d", quotes, items)
						}
					})
				}
			}
		}
	}
}

func TestQuoteSeparatorStaysInTrailer(t *testing.T) {
	for _, src := range []string{
		"> a\n>\n> b", "> a\r\n> \r\n> b", "> > a\n> >\n> > b", "- > a\n  >\n  > b",
		"> a\n>\n> # b", "> a\n>\n> - b",
		"> > a\n>\n> b", "- > > a\n  >\n  > b",
	} {
		t.Run(src, func(t *testing.T) {
			s := mustNew(t, src, 1, contract.ExtDefault).Snapshot()
			checkCoverage(t, s)
			if s.LeafCount() != 2 {
				for i := 0; i < s.Len(); i++ {
					t.Logf("block %d: %+v", i, s.Block(i))
				}
				t.Fatalf("leaves = %d", s.LeafCount())
			}
			first := s.Block(s.LeafAt(strings.Index(src, "a")))
			_, separatorStart := lineEnd(src, first.Range.End)
			_, separatorEnd := lineEnd(src, separatorStart)
			if first.Trailer.End != separatorEnd {
				t.Fatalf("separator not absorbed: %+v", first)
			}
		})
	}
}

func TestQuoteEndsInRepeatedListContexts(t *testing.T) {
	src := "- intro\n  > > a\n  > >\n  > outer\n  > > b\n  > >\n\nnext"
	s := mustNew(t, src, 1, contract.ExtDefault).Snapshot()
	checkCoverage(t, s)
	var quotes []int
	for _, marker := range []string{"  > >\n  > outer", "  > >\n\nnext"} {
		start := strings.Index(src, marker)
		end, _ := lineEnd(src, start)
		b := s.Block(s.LeafAt(end))
		if b.Kind != contract.BlockBlank || b.Range != (contract.Range{Start: end, End: end}) || s.Block(b.Parent).Kind != contract.BlockQuote {
			t.Fatalf("marker at %d: leaf = %+v", start, b)
		}
		quotes = append(quotes, b.Parent)
	}
	if quotes[0] == quotes[1] || s.Block(quotes[0]).Parent != s.Block(quotes[1]).Parent {
		t.Fatalf("inner quote parents = %v", quotes)
	}
}

func TestQuoteTrailingMarkerBoundaries(t *testing.T) {
	for _, src := range []string{
		"> a\n>\n>", "> a\n> \n> ", "> a\n> \n> \n\nnext", "> \n> ",
		"> a\n> b\n> ", "> # heading\n> ", "> ```\n> code\n> ```\n> ",
		"> [r]: /u\n> ", "> [r]: /u\r\n> \r\n\r\nnext", "> a\n>\n> [r]: /u\n> ",
		"- > [r]: /u\n  > ",
		"- > a\n  > \n  > ",
	} {
		t.Run(src, func(t *testing.T) {
			markerStart := strings.LastIndex(src, ">")
			markerEnd, _ := lineEnd(src, markerStart)
			s := mustNew(t, src, 1, contract.ExtDefault).Snapshot()
			checkCoverage(t, s)
			b := s.Block(s.LeafAt(markerEnd))
			if b.Kind != contract.BlockBlank || b.Range != (contract.Range{Start: markerEnd, End: markerEnd}) || s.Block(b.Parent).Kind != contract.BlockQuote {
				t.Fatalf("last marker leaf = %+v, parent = %+v", b, s.Block(b.Parent))
			}
		})
	}
	for _, src := range []string{"- > a\n> ", "> a\n\n> "} {
		t.Run(src, func(t *testing.T) {
			s := mustNew(t, src, 1, contract.ExtDefault).Snapshot()
			checkCoverage(t, s)
			first := s.Block(s.LeafAt(strings.Index(src, "a")))
			last := s.Block(s.LeafAt(len(src)))
			if s.Block(last.Parent).Kind != contract.BlockQuote || last.Parent == first.Parent {
				t.Fatalf("distinct quotes: first = %+v, last = %+v", first, last)
			}
		})
	}
}

func TestQuoteDefinitionBeforeTrailingMarker(t *testing.T) {
	for _, src := range []string{"> [r]: /u\n> ", "> a\n>\n> [r]: /u\n> "} {
		t.Run(src, func(t *testing.T) {
			s := mustNew(t, src, 1, contract.ExtDefault).Snapshot()
			checkCoverage(t, s)
			if len(s.LinkDefs()) != 1 {
				t.Fatalf("definitions = %+v", s.LinkDefs())
			}
			var kinds []contract.BlockKind
			var parent int
			for i := range s.Leaves() {
				b := s.Block(i)
				kinds = append(kinds, b.Kind)
				if parent == 0 {
					parent = b.Parent
				} else if b.Parent != parent {
					t.Fatalf("leaf has another quote parent: %+v", b)
				}
			}
			want := []contract.BlockKind{contract.BlockLinkDef, contract.BlockBlank}
			if strings.Contains(src, "> a") {
				want = append([]contract.BlockKind{contract.BlockParagraph}, want...)
			}
			if !slices.Equal(kinds, want) {
				t.Fatalf("leaf kinds = %v, want %v", kinds, want)
			}
		})
	}
}

func TestQuoteMarkerIncremental(t *testing.T) {
	for _, eol := range []string{"\n", "\r\n"} {
		for _, suffix := range []string{"", "\n\nnext\n\nlast"} {
			t.Run(fmt.Sprintf("%q/%q", eol, suffix), func(t *testing.T) {
				p := mustNew(t, suffix, 1, contract.ExtDefault)
				at := 0
				apply := func(remove int, insert string) {
					t.Helper()
					old := p.Snapshot()
					u, err := p.Apply([]contract.TextEdit{{Range: contract.Range{Start: at - remove, End: at}, Text: insert}})
					if err != nil {
						t.Fatal(err)
					}
					at += len(insert) - remove
					if err := equivalent(u.Snapshot, mustNew(t, u.Snapshot.text.String(), u.Snapshot.Version(), contract.ExtDefault).Snapshot()); err != nil {
						t.Fatalf("after deleting %d and inserting %q: %v", remove, insert, err)
					}
					checkCoverage(t, u.Snapshot)
					checkDirty(t, old, u)
				}
				for _, insert := range []string{">", " ", "a", eol + "> ", "b"} {
					apply(0, insert)
				}
				for at > 0 {
					apply(1, "")
				}
			})
		}
	}
}
