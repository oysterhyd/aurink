package projection

import (
	"testing"

	"aurink/internal/contract"
)

func TestDoubleDollarInlineProjection(t *testing.T) {
	src := "$$abc$$\n"
	tx, ast := parseExts(t, src, contract.ExtDollarMath)
	leaf := ast.LeafAt(0)
	if ast.Block(leaf).Kind != contract.BlockParagraph {
		t.Fatalf("block = %+v", ast.Block(leaf))
	}
	pr := New()
	hidden := pr.Project(ast, tx, leaf, contract.Reveal{Mode: contract.RevealInline, Caret: -1})
	if got := pr.DisplayText(&hidden); got != "\ufffc" {
		t.Fatalf("hidden display = %q", got)
	}
	objects := 0
	for _, seg := range hidden.Segments {
		if seg.Object != nil {
			objects++
			if seg.Kind != contract.SegObject || seg.Object.Kind != contract.ObjMath || seg.Object.Display || seg.Object.Source != "abc" || seg.Style&contract.StyleMath == 0 {
				t.Fatalf("object = %+v", seg)
			}
		}
	}
	if objects != 1 {
		t.Fatalf("got %d objects", objects)
	}
	revealed := pr.Project(ast, tx, leaf, contract.Reveal{Mode: contract.RevealInline, Caret: 3})
	if got := pr.DisplayText(&revealed); got != "$$abc$$" {
		t.Fatalf("revealed display = %q", got)
	}
	var markers []string
	for _, seg := range revealed.Segments {
		if seg.Kind == contract.SegMarker {
			markers = append(markers, seg.Display)
		}
	}
	if len(markers) != 2 || markers[0] != "$$" || markers[1] != "$$" {
		t.Fatalf("markers = %q", markers)
	}
}
