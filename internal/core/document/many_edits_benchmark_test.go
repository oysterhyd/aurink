package document

import (
	"strings"
	"testing"

	"aurink/internal/contract"
)

func BenchmarkApplyManyEdits(b *testing.B) {
	const paragraphs = 30000
	const line = "a cat and a dog\n\n"
	source := strings.Repeat(line, paragraphs)
	replaced := strings.Repeat("a cow and a dog\n\n", paragraphs)
	edits := make([]contract.TextEdit, paragraphs)
	for i := range edits {
		start := i*len(line) + len("a ")
		edits[i] = contract.TextEdit{Range: contract.Range{Start: start, End: start + len("cat")}, Text: "cow"}
	}
	tx := contract.Transaction{Edits: edits, Origin: contract.OriginCommand, Label: "replace_all"}
	for _, undo := range []bool{false, true} {
		name := "Apply"
		if undo {
			name = "Undo"
		}
		b.Run(name, func(b *testing.B) {
			m := newModel(b, source)
			blocks := m.AST().Len()
			events, changes := 0, 0
			m.Subscribe(func(e contract.DocEvent) {
				if e.Kind == contract.DocTextChanged {
					events++
					changes += len(e.Changes)
				}
			})
			b.ReportAllocs()
			b.ResetTimer()
			b.StopTimer()
			for i := 0; i < b.N; i++ {
				if !undo {
					b.StartTimer()
				}
				apply(b, m, tx)
				if !undo {
					b.StopTimer()
				}
				if m.Text().String() != replaced || !m.CanUndo() {
					b.Fatal("replace-all did not apply as one undoable transaction")
				}
				if undo {
					b.StartTimer()
				}
				if !m.Undo() {
					b.Fatal("undo failed")
				}
				if undo {
					b.StopTimer()
				}
				if m.Text().String() != source || m.CanUndo() || !m.CanRedo() {
					b.Fatal("one undo did not restore the original document")
				}
			}
			b.ReportMetric(float64(blocks), "blocks/op")
			if events != 2*b.N || changes != 2*paragraphs*b.N {
				b.Fatalf("events=%d changes=%d", events, changes)
			}
		})
	}
}
