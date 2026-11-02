package projection

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"aurink/internal/contract"
)

func TestPendingMarkers(t *testing.T) {
	var markers []string
	for level := 1; level <= 6; level++ {
		markers = append(markers, strings.Repeat("#", level))
	}
	markers = append(markers, "+", "-", "*", "1.", "1)", "42.", "987654321)", ">")
	for _, marker := range markers {
		for _, ending := range []string{"", "\n", "\r\n", "\r"} {
			t.Run(fmt.Sprintf("%q/%q", marker, ending), func(t *testing.T) {
				src := marker + ending
				txt, ast := parse(t, src)
				leaf := ast.LeafAt(0)
				pr := New()
				var first contract.BlockProjection
				for i, r := range []contract.Reveal{
					{Mode: contract.RevealInline, Caret: len(marker)},
					{Mode: contract.RevealInline, Caret: 0},
					{Mode: contract.RevealInline, Caret: -1},
					{Mode: contract.RevealNone, Caret: -1},
					{Mode: contract.RevealAlways, Caret: 0, Selection: contract.Range{Start: 0, End: len(marker)}},
				} {
					p := pr.Project(ast, txt, leaf, r)
					if p.Kind != contract.BlockParagraph || p.Level != 0 || len(p.Containers) != 0 || pr.DisplayText(&p) != marker {
						t.Fatalf("reveal %+v: %+v, display %q", r, p, pr.DisplayText(&p))
					}
					if len(p.Segments) != 3 || p.Segments[1].Kind != contract.SegText || p.Segments[1].Style != 0 || p.Segments[1].Src != (contract.Range{End: len(marker)}) {
						t.Fatalf("marker is not plain text: %+v", p.Segments)
					}
					if i > 0 && !reflect.DeepEqual(p, first) {
						t.Fatalf("pending projection depends on reveal: %+v vs %+v", p, first)
					}
					first = p
					assertPendingMapping(t, pr, &p, 0, marker)
				}
			})
		}
		for _, whitespace := range []string{" ", "\t"} {
			t.Run(fmt.Sprintf("%q/with %q", marker, whitespace), func(t *testing.T) {
				src := marker + whitespace
				txt, ast := parse(t, src)
				leaf := ast.LeafAt(0)
				for _, r := range []contract.Reveal{{Caret: len(src)}, {Caret: -1}, {Mode: contract.RevealNone, Caret: -1}} {
					p := New().Project(ast, txt, leaf, r)
					if got := New().DisplayText(&p); got != "" {
						t.Fatalf("reveal %+v: completed empty block displays %q", r, got)
					}
					if marker[0] == '#' {
						if p.Kind != contract.BlockHeading || p.Level != len(marker) {
							t.Fatalf("reveal %+v: expected heading, got %+v", r, p)
						}
					} else if p.Kind != contract.BlockBlank || len(p.Containers) != 1 {
						t.Fatalf("reveal %+v: expected decorated empty container, got %+v", r, p)
					}
				}
			})
		}
	}
}

func assertPendingMapping(t *testing.T, pr *Projector, p *contract.BlockProjection, start int, marker string) {
	t.Helper()
	for i := 0; i <= len(marker); i++ {
		for _, aff := range []contract.Affinity{contract.Upstream, contract.Downstream} {
			if got := pr.ToDisplay(p, start+i, aff); got != i {
				t.Fatalf("ToDisplay(%d, %v) = %d, want %d", start+i, aff, got, i)
			}
			if got := pr.ToSource(p, i, aff); got != start+i {
				t.Fatalf("ToSource(%d, %v) = %d, want %d", i, aff, got, start+i)
			}
			if got := pr.Snap(p, start+i, aff); got != start+i {
				t.Fatalf("Snap(%d, %v) = %d", start+i, aff, got)
			}
		}
	}
}

func TestPendingNestedContainers(t *testing.T) {
	for _, tc := range []struct {
		src, marker string
		containers  []contract.BlockKind
	}{
		{"> +", "+", []contract.BlockKind{contract.BlockQuote}},
		{"> >", ">", []contract.BlockKind{contract.BlockQuote}},
		{"> 123)", "123)", []contract.BlockKind{contract.BlockQuote}},
		{"> #", "#", []contract.BlockKind{contract.BlockQuote}},
		{"- a\n+", "+", nil},
		{"- a\n\n  +", "+", []contract.BlockKind{contract.BlockItem}},
		{"> - +", "+", []contract.BlockKind{contract.BlockQuote, contract.BlockItem}},
		{" >\t>", ">", []contract.BlockKind{contract.BlockQuote}},
	} {
		t.Run(tc.src, func(t *testing.T) {
			start := strings.Index(tc.src, tc.marker)
			if strings.HasPrefix(tc.src, "> >") || tc.src == " >\t>" {
				start = strings.Index(tc.src, ">") + 2
			}
			txt, ast := parse(t, tc.src)
			pr := New()
			p := pr.Project(ast, txt, ast.LeafAt(start), contract.Reveal{Mode: contract.RevealNone, Caret: -1})
			if p.Kind != contract.BlockParagraph || p.Level != 0 || pr.DisplayText(&p) != tc.marker || len(p.Containers) != len(tc.containers) {
				t.Fatalf("got %+v display %q", p, pr.DisplayText(&p))
			}
			for i, kind := range tc.containers {
				if p.Containers[i].Kind != kind || p.Containers[i].Depth != i {
					t.Fatalf("container %d: %+v", i, p.Containers[i])
				}
			}
			assertPendingMapping(t, pr, &p, start, tc.marker)
		})
	}
}

func TestPendingDoesNotChangeOtherBlocks(t *testing.T) {
	for _, tc := range []struct {
		src  string
		kind contract.BlockKind
	}{
		{"title\n-", contract.BlockHeading},
		{"title\n===", contract.BlockHeading},
		{"---", contract.BlockThematicBreak},
		{"***", contract.BlockThematicBreak},
		{"```\n+\n```", contract.BlockCode},
		{"    +", contract.BlockCode},
		{"#a", contract.BlockParagraph},
		{"#######", contract.BlockParagraph},
		{"-\n  a", contract.BlockParagraph},
	} {
		t.Run(tc.src, func(t *testing.T) {
			txt, ast := parse(t, tc.src)
			leaf := ast.LeafAt(len(tc.src) / 2)
			p := New().Project(ast, txt, leaf, contract.Reveal{Mode: contract.RevealNone, Caret: -1})
			if p.Kind != tc.kind {
				t.Fatalf("kind %v, want %v", p.Kind, tc.kind)
			}
		})
	}
	pr := New()
	var sigs []uint64
	for _, src := range []string{"#", "# "} {
		txt, ast := parse(t, src)
		p := pr.Project(ast, txt, ast.LeafAt(0), contract.Reveal{Caret: len(src)})
		sigs = append(sigs, p.Sig)
	}
	if sigs[0] == sigs[1] {
		t.Fatal("pending and completed headings have the same layout signature")
	}
	for _, src := range []string{">\n> a", "> >\n> > a", "> a\n>"} {
		txt, ast := parse(t, src)
		for leaf := range ast.Leaves() {
			p := pr.Project(ast, txt, leaf, contract.Reveal{Mode: contract.RevealNone, Caret: -1})
			if strings.Contains(pr.DisplayText(&p), ">") {
				t.Fatalf("quote with content exposed a marker in %q: %+v", src, p)
			}
		}
	}
}
