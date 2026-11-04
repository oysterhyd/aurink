package projection

import (
	"testing"

	"aurink/internal/contract"
)

func TestTrailingQuoteMarkerProjection(t *testing.T) {
	for _, tc := range []struct {
		src        string
		containers []contract.BlockKind
	}{
		{"> ", []contract.BlockKind{contract.BlockQuote}},
		{"> a\n> ", []contract.BlockKind{contract.BlockQuote}},
		{"> a\n>", []contract.BlockKind{contract.BlockQuote}},
		{"> > a\r\n> > ", []contract.BlockKind{contract.BlockQuote, contract.BlockQuote}},
		{"- > a\n  > ", []contract.BlockKind{contract.BlockItem, contract.BlockQuote}},
	} {
		t.Run(tc.src, func(t *testing.T) {
			text, ast := parseExts(t, tc.src, contract.ExtDefault)
			pr := New()
			p := pr.Project(ast, text, ast.LeafAt(len(tc.src)), contract.Reveal{Mode: contract.RevealInline, Caret: len(tc.src)})
			if p.Kind != contract.BlockBlank || pr.DisplayText(&p) != "" || len(p.Containers) != len(tc.containers) {
				t.Fatalf("projection = %+v, display = %q", p, pr.DisplayText(&p))
			}
			for i, kind := range tc.containers {
				if p.Containers[i].Kind != kind || p.Containers[i].Depth != i {
					t.Fatalf("container %d = %+v", i, p.Containers[i])
				}
			}
			caret := ast.Block(ast.LeafAt(len(tc.src))).Range.Start
			for _, affinity := range []contract.Affinity{contract.Upstream, contract.Downstream} {
				if got := pr.ToSource(&p, 0, affinity); got != caret {
					t.Fatalf("ToSource(0, %v) = %d, want %d", affinity, got, caret)
				}
			}
		})
	}
}
