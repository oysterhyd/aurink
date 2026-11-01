package editing

import (
	"testing"

	"aurink/internal/contract"
	"aurink/tests/corpus"
)

var benchmarkTransaction contract.Transaction

func benchmarkContexts(b *testing.B, list bool) []contract.EditContext {
	b.Helper()
	src := corpus.Generate(200<<10, 1331)
	ctx := contextFor(b, src, contract.Caret(0), "\n")
	var contexts []contract.EditContext
	for i := range ctx.AST.Leaves() {
		block := ctx.AST.Block(i)
		if block.Kind != contract.BlockParagraph || block.Range.Start < len(src)/4 || block.Range.End > 3*len(src)/4 {
			continue
		}
		isList := ancestor(ctx, block.Range.Start, contract.BlockItem) != nil
		if isList != list || len(block.Lines) == 0 {
			continue
		}
		at := block.Lines[0].Source.End
		copy := ctx
		copy.Selection = contract.Caret(at)
		contexts = append(contexts, copy)
		if len(contexts) == 64 {
			break
		}
	}
	if len(contexts) < 16 {
		b.Fatal("benchmark did not find enough ordinary paragraph/list carets")
	}
	return contexts
}

func BenchmarkHandleIntentTyping(b *testing.B) {
	contexts := benchmarkContexts(b, false)
	intent := contract.Intent{Kind: contract.IntentInsertText, Text: "中"}
	ed := New()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tx, ok := ed.HandleIntent(contexts[i%len(contexts)], intent)
		if !ok || tx.Coalesce != "typing" || len(tx.Edits) != 1 {
			b.Fatal("ordinary typing was not handled")
		}
		benchmarkTransaction = tx
	}
}

func BenchmarkHandleIntentListEnter(b *testing.B) {
	contexts := benchmarkContexts(b, true)
	intent := contract.Intent{Kind: contract.IntentNewline}
	ed := New()
	// Build the immutable text's line index outside the timed operation.
	contexts[0].Text.LineOf(contexts[0].Selection.Primary().Head)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tx, ok := ed.HandleIntent(contexts[i%len(contexts)], intent)
		if !ok || len(tx.Edits) != 1 || len(tx.Edits[0].Text) < 3 {
			b.Fatal("ordinary list continuation was not handled")
		}
		benchmarkTransaction = tx
	}
}
