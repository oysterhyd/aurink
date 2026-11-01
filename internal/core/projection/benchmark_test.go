package projection

import (
	"strings"
	"testing"

	"aurink/internal/contract"
)

var benchmarkProjection contract.BlockProjection

func BenchmarkProject(b *testing.B) {
	for _, tc := range []struct {
		name, src, find string
		kind            contract.BlockKind
	}{
		{"Paragraph", "A normal 段落 with **strong**, *emphasis*, `code`, [a link](a.md), &amp; an emoji 👩‍💻.\n", "", contract.BlockParagraph},
		{"LongParagraph", strings.Repeat("Mixed 中文 😀 **strong** [link](a.md) &amp; `code` words. ", 120) + "\n", "", contract.BlockParagraph},
		{"Code200Lines", "```go\n" + strings.Repeat("func hello() { println(\"世界 😀\") }\n", 200) + "```\n", "", contract.BlockCode},
		{"TableCell", "| Heading | Other |\n|---|---|\n| 中文 **strong** and [link](a.md) &amp; `code` 😀 | value |\n", "中文", contract.BlockTableCell},
	} {
		b.Run(tc.name, func(b *testing.B) {
			tx, a := parse(b, tc.src)
			at := 0
			if tc.find != "" {
				at = strings.Index(tc.src, tc.find)
			}
			leaf := a.LeafAt(at)
			n := a.Block(leaf)
			if n.Kind != tc.kind || tc.kind == contract.BlockCode && len(n.Lines) != 200 {
				b.Fatal("benchmark did not find its ordinary leaf")
			}
			// Measure projection after the AST's lazy inline parse, as on caret
			// movement. All ordinary syntax still goes through Project each time.
			caret := n.Content.Start
			for _, inline := range a.Inlines(leaf) {
				if !inline.Open.Empty() {
					caret = inline.Open.End
					break
				}
			}
			if p := New().Project(a, tx, leaf, contract.Reveal{Caret: caret}); !p.Revealed {
				b.Fatal("editing benchmark did not reveal syntax")
			}
			for _, ctx := range []struct {
				name string
				r    contract.Reveal
			}{
				{"Resting", contract.Reveal{Caret: -1}},
				{"Editing", contract.Reveal{Caret: caret}},
			} {
				b.Run(ctx.name, func(b *testing.B) {
					pr := New()
					b.ReportAllocs()
					b.SetBytes(int64(n.Range.Len()))
					b.ResetTimer()
					for b.Loop() {
						benchmarkProjection = pr.Project(a, tx, leaf, ctx.r)
					}
				})
			}
		})
	}
}
