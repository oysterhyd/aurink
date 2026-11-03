package markdown

import (
	"strings"
	"testing"

	"aurink/internal/contract"
)

// This pairs the same edit and inline projection input with extensions on
// and off, to expose overhead on documents without extension syntax.
func BenchmarkExtensionFreeTyping(b *testing.B) {
	src := strings.Repeat("Ordinary paragraph: 中文 **bold** [link](file.md) and more words.\n\n", 150)
	at := strings.Index(src[len(src)/2:], "paragraph") + len(src)/2 + 5
	for _, tc := range []struct {
		name string
		exts contract.Extensions
	}{{"disabled", contract.ExtFrontMatter}, {"default", contract.ExtDefault}} {
		b.Run(tc.name, func(b *testing.B) {
			p := mustNew(b, src, 1, tc.exts)
			base := p.Snapshot()
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				p.current, p.next = base, base.highest
				u, err := p.Apply([]contract.TextEdit{{Range: contract.Range{Start: at, End: at}, Text: "x"}})
				if err != nil {
					b.Fatal(err)
				}
				_ = u.Snapshot.Inlines(u.Snapshot.LeafAt(at))
			}
		})
	}
}
