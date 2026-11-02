package document

import (
	"slices"
	"strings"
	"testing"

	"aurink/internal/contract"
)

func TestManyEditsUndoAndDirtyBlocks(t *testing.T) {
	const paragraphs = 256
	const line = "a cat and a dog\n\n"
	source := strings.Repeat(line, paragraphs)
	var replaced strings.Builder
	var edits []contract.TextEdit
	for i := 0; i < paragraphs; i++ {
		if i%2 == 0 {
			start := i*len(line) + 2
			edits = append(edits, contract.TextEdit{Range: contract.Range{Start: start, End: start + 3}, Text: "puma"})
			replaced.WriteString("a puma and a dog\n\n")
		} else {
			replaced.WriteString(line)
		}
	}
	m := newModel(t, source)
	beforeSelection := contract.Selection{Ranges: []contract.SelRange{{Anchor: len(source), Head: len(source)}, {Anchor: len(source) - 1, Head: 2}}}
	m.SetSelection(beforeSelection)
	old := m.AST()
	var events []contract.DocEvent
	m.Subscribe(func(e contract.DocEvent) {
		if e.Kind == contract.DocTextChanged {
			events = append(events, e)
		}
	})
	apply(t, m, contract.Transaction{Edits: edits, Origin: contract.OriginCommand, Label: "replace_all"})
	wantSelection := contract.Selection{Ranges: []contract.SelRange{{Anchor: len(replaced.String()), Head: len(replaced.String())}, {Anchor: len(source) - 1 + len(edits), Head: 6}}}
	if m.Text().String() != replaced.String() || !slices.Equal(m.Selection().Ranges, wantSelection.Ranges) || !m.CanUndo() {
		t.Fatalf("replace-all text, selection or undo state is wrong: selection=%v", m.Selection())
	}
	checkManyEditEvent(t, old, m.AST(), events, contract.OriginCommand, len(edits), len(line))
	assertAST(t, m)

	updated := m.AST()
	if !m.Undo() || m.Text().String() != source || !slices.Equal(m.Selection().Ranges, beforeSelection.Ranges) || m.CanUndo() || !m.CanRedo() {
		t.Fatal("one undo did not restore the document and selection")
	}
	checkManyEditEvent(t, updated, m.AST(), events[1:], contract.OriginUndo, len(edits), len("a puma and a dog\n\n"))
	assertAST(t, m)
	if !m.Redo() || m.Text().String() != replaced.String() || !slices.Equal(m.Selection().Ranges, wantSelection.Ranges) {
		t.Fatal("one redo did not restore the transaction")
	}
	if len(events) != 3 || events[2].Origin != contract.OriginRedo || len(events[2].Changes) != len(edits) {
		t.Fatalf("redo event = %+v", events)
	}
}

func checkManyEditEvent(t *testing.T, before, after contract.AST, events []contract.DocEvent, origin contract.Origin, editCount, changedLineLen int) {
	t.Helper()
	if len(events) != 1 || events[0].Origin != origin || len(events[0].Changes) != editCount || before.Len() != after.Len() {
		t.Fatalf("wrong event or block count: events=%d blocks=%d/%d", len(events), before.Len(), after.Len())
	}
	dirty := make(map[contract.BlockID]bool, len(events[0].Dirty))
	for _, id := range events[0].Dirty {
		dirty[id] = true
	}
	for i := range before.Leaves() {
		old, next := before.Block(i), after.Block(i)
		if old.ID != next.ID {
			t.Fatalf("leaf %d changed identity: %d -> %d", i, old.ID, next.ID)
		}
		// Each pair contains a changed paragraph followed by an unchanged one.
		// Their combined length is the same in both coordinate spaces.
		pairLen := changedLineLen + len("a cat and a dog\n\n")
		shouldBeDirty := old.Kind == contract.BlockParagraph && old.Range.Start%pairLen == 0
		if dirty[next.ID] != shouldBeDirty {
			t.Fatalf("leaf %d at %d: dirty=%v, want %v", i, old.Range.Start, dirty[next.ID], shouldBeDirty)
		}
	}
}
