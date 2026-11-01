package main

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestLayoutCost(t *testing.T) {
	para := strings.Repeat("The quick brown fox 敏捷的棕色狐狸 jumps over **the** lazy dog. ", 5) // ~330 bytes
	spans := []ui.Span{{Text: para[:100]}, {Text: para[100:200], Weight: 700}, {Text: para[200:], Font: "monospace", Size: 13}}
	// warm
	for i := 0; i < 50; i++ {
		ui.ShapeRichTextWith(spans, ui.Font{Size: 16, LineHeight: 1.6}, ui.TextShape{Width: 680})
	}
	const N = 2000
	t0 := time.Now()
	for i := 0; i < N; i++ {
		spans[0].Text = para[:100-(i%7)] // defeat any cache
		ui.ShapeRichTextWith(spans, ui.Font{Size: 16, LineHeight: 1.6}, ui.TextShape{Width: 680})
	}
	t.Logf("ShapeRichTextWith ~330B paragraph @680w: %v avg", time.Since(t0)/N)
}
