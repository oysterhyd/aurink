package document

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

	"aurink/internal/contract"
	"aurink/internal/core/text"
)

func TestCoalescingBoundaries(t *testing.T) {
	for _, c := range []struct {
		name   string
		change func(*Model, *time.Time, *contract.Transaction)
	}{
		{"one second", func(_ *Model, now *time.Time, _ *contract.Transaction) { *now = now.Add(time.Second) }},
		{"clock backwards", func(_ *Model, now *time.Time, _ *contract.Transaction) { *now = now.Add(-time.Millisecond) }},
		{"different key", func(_ *Model, _ *time.Time, tx *contract.Transaction) { tx.Coalesce = "other" }},
		{"caret jump and return", func(m *Model, _ *time.Time, _ *contract.Transaction) {
			m.SetSelection(contract.Caret(0))
			m.SetSelection(contract.Caret(1))
		}},
		{"selection-only transaction", func(m *Model, _ *time.Time, _ *contract.Transaction) {
			apply(t, m, contract.Transaction{Selection: selectionPtr(contract.Caret(0))})
		}},
		{"newline insertion", func(_ *Model, _ *time.Time, tx *contract.Transaction) { tx.Edits[0].Text = "\n" }},
		{"CR insertion", func(_ *Model, _ *time.Time, tx *contract.Transaction) { tx.Edits[0].Text = "\r" }},
		{"newline deletion", func(_ *Model, _ *time.Time, tx *contract.Transaction) { tx.Edits[0].Range.End++; tx.Edits[0].Text = "" }},
		{"multiple edits", func(_ *Model, _ *time.Time, tx *contract.Transaction) {
			tx.Edits = append(tx.Edits, contract.TextEdit{Range: contract.Range{Start: 3, End: 3}, Text: "z"})
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			now := time.Unix(1, 0)
			m := (Factory{Clock: func() time.Time { return now }}).New("clock", "\nxx", contract.DocMeta{}, 0).(*Model)
			apply(t, m, edit(0, 0, "a"))
			tx := edit(1, 1, "b")
			c.change(m, &now, &tx)
			apply(t, m, tx)
			if len(m.history.undo) != 2 || !m.Undo() || m.Text().String() != "a\nxx" {
				t.Fatal("separate steps merged", m.Text().String(), len(m.history.undo))
			}
		})
	}
	for _, origin := range []contract.Origin{contract.OriginIME, contract.OriginCommand, contract.OriginInputRule, contract.OriginPaste, contract.OriginDrop, contract.OriginReload, contract.OriginAPI, contract.OriginUndo, contract.OriginRedo} {
		t.Run(fmt.Sprint(origin), func(t *testing.T) {
			m := newModel(t, "")
			for i := 0; i < 2; i++ {
				tx := edit(i, i, "x")
				tx.Origin = origin
				apply(t, m, tx)
			}
			if len(m.history.undo) != 2 || !m.Undo() || m.Text().String() != "x" {
				t.Fatal("origin coalesced", origin)
			}
			apply(t, m, edit(1, 1, "y"))
			if len(m.history.undo) != 2 {
				t.Fatal("typing merged into a separate origin")
			}
		})
	}
	// A separator also prevents the following typing from merging into it.
	m := newModel(t, "")
	apply(t, m, edit(0, 0, "a\n"))
	apply(t, m, edit(2, 2, "b"))
	if len(m.history.undo) != 2 {
		t.Fatal("typing merged across a newline")
	}
}

func TestCoalescedInverseComposition(t *testing.T) {
	for _, c := range []struct {
		name, source string
		edits        []contract.Transaction
	}{
		{"typing", "abc", []contract.Transaction{edit(1, 1, "X"), edit(2, 2, "Y"), edit(3, 3, "Z")}},
		{"backspace", "abcdef", []contract.Transaction{edit(3, 4, ""), edit(2, 3, ""), edit(1, 2, "")}},
		{"delete", "abcdef", []contract.Transaction{edit(1, 2, ""), edit(1, 2, ""), edit(1, 2, "")}},
		{"replacements", "abcdef", []contract.Transaction{edit(2, 4, "XY"), edit(1, 3, "Z"), edit(2, 4, "Q")}},
		{"both sides", "abcdef", []contract.Transaction{edit(2, 4, "XY"), edit(1, 5, "Z")}},
		{"insert at left", "abcdef", []contract.Transaction{edit(2, 4, "XY"), edit(2, 2, "Z")}},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := newModel(t, c.source)
			m.SetSelection(contract.Caret(2))
			for _, tx := range c.edits {
				apply(t, m, tx)
			}
			finalText, finalSel := m.Text().String(), m.Selection()
			if len(m.history.undo) != 1 || !m.Undo() || m.CanUndo() || m.Text().String() != c.source || !sameSelection(m.Selection(), contract.Caret(2)) {
				t.Fatal("coalesced undo", m.Text().String(), len(m.history.undo))
			}
			if !m.Redo() || m.Text().String() != finalText || !sameSelection(m.Selection(), finalSel) {
				t.Fatal("coalesced redo")
			}
			assertBudget(t, &m.history)
		})
	}
	// Random touching replacements stress composition beyond simple typing.
	rng := rand.New(rand.NewSource(8123))
	for run := 0; run < 50; run++ {
		m := newModel(t, "abcdefghijklmnopqrstuvwxyz")
		for step := 0; step < 40; step++ {
			start, end := 10, 12
			if step > 0 {
				r := m.history.undo[0].last
				start = max(0, r.Start-rng.Intn(3))
				end = min(m.text.Len(), max(r.Start, r.End+rng.Intn(3)-1))
			}
			apply(t, m, edit(start, end, strings.Repeat("x", rng.Intn(4))))
		}
		final := m.Text().String()
		if len(m.history.undo) != 1 || !m.Undo() || m.Text().String() != "abcdefghijklmnopqrstuvwxyz" || !m.Redo() || m.Text().String() != final {
			t.Fatalf("composition run %d: %q", run, m.Text().String())
		}
	}
}

func TestCoalescingAdjacencyAndFailedEdits(t *testing.T) {
	m := newModel(t, "xxxx")
	apply(t, m, edit(1, 1, "a"))
	for _, tx := range []contract.Transaction{edit(-1, 0, "x"), {Selection: selectionPtr(contract.Caret(99))}} {
		if _, err := m.Apply(tx); contract.CodeOf(err) != contract.CodeInvalid {
			t.Fatal("invalid transaction", err)
		}
	}
	apply(t, m, edit(2, 2, "b"))
	if len(m.history.undo) != 1 {
		t.Fatal("failed edit interrupted coalescing")
	}
	apply(t, m, edit(1, 1, "c"))
	if len(m.history.undo) != 2 || !m.Undo() || m.Text().String() != "xabxxx" {
		t.Fatal("edit away from the last insertion coalesced")
	}
	// Source typing can coalesce, but crossing origins still ends a step.
	m = newModel(t, "")
	for i := 0; i < 2; i++ {
		tx := edit(i, i, "a")
		tx.Origin = contract.OriginSource
		apply(t, m, tx)
	}
	apply(t, m, edit(2, 2, "b"))
	if len(m.history.undo) != 2 {
		t.Fatal("coalescing across origins")
	}
	if !m.Undo() || m.Text().String() != "aa" || !m.Undo() || m.Text().String() != "" {
		t.Fatal("source typing was not one step")
	}
}

func TestHistoryLimits(t *testing.T) {
	m := newModel(t, "z")
	const count = 1037
	for i := 0; i < count; i++ {
		tx := edit(0, 1, string(byte('a'+i%26)))
		tx.Origin = contract.OriginCommand
		apply(t, m, tx)
		assertBudget(t, &m.history)
	}
	if len(m.history.undo) != 1000 || m.history.bytes != 1000 {
		t.Fatal("step cap", len(m.history.undo), m.history.bytes)
	}
	for i := 0; i < 1000; i++ {
		if !m.Undo() {
			t.Fatal("early undo limit", i)
		}
		assertBudget(t, &m.history)
	}
	want := string(byte('a' + (count-1000-1)%26))
	if m.Undo() || m.Text().String() != want || len(m.history.redo) != 1000 || m.history.bytes != 1000 {
		t.Fatal("oldest undo steps were not dropped", m.Text().String(), want)
	}
	for i := 0; i < 1000; i++ {
		if !m.Redo() {
			t.Fatal("early redo limit", i)
		}
		assertBudget(t, &m.history)
	}
	if m.Redo() || m.Text().String() != string(byte('a'+(count-1)%26)) {
		t.Fatal("redo cap")
	}
}

func TestInverseByteLimit(t *testing.T) {
	// Exercise the real history policy without reparsing tens of MB per step.
	var h history
	chunk := strings.Repeat("x", 2<<20)
	old := text.New(chunk, 1)
	for i := 0; i < 34; i++ {
		s := historyStep{edits: inverseEdits(old, []contract.Change{{OldEnd: len(chunk)}}), label: fmt.Sprint(i)}
		h.record(s, contract.Transaction{}, old, false)
		assertBudget(t, &h)
	}
	if len(h.undo) != 32 || h.bytes != maxHistoryBytes || h.undo[0].label != "2" {
		t.Fatal("inverse byte cap", len(h.undo), h.bytes)
	}
	// Undo replaces each stored inverse with the inverse of that replay.
	h.move(historyStep{edits: []contract.TextEdit{{Text: "redo"}}}, -1)
	assertBudget(t, &h)
	h.move(historyStep{edits: []contract.TextEdit{{Text: chunk}}}, 1)
	assertBudget(t, &h)
	// Redo entries retain no source snapshots and are also bounded. Removing
	// the farthest redo preserves the executable chain near the current state.
	h = history{redo: []historyStep{{edits: []contract.TextEdit{{Text: chunk}}, label: "old"}, {edits: []contract.TextEdit{{Text: chunk}}, label: "recent"}}, bytes: 2 * len(chunk)}
	oversize := strings.Repeat("y", maxHistoryBytes-1)
	h.redo = append(h.redo, historyStep{edits: []contract.TextEdit{{Text: oversize}}})
	h.bytes += len(oversize)
	h.trim()
	assertBudget(t, &h)
	if len(h.redo) != 1 || h.bytes != len(oversize) {
		t.Fatal("redo memory cap")
	}
	h.record(historyStep{edits: []contract.TextEdit{{Text: "a"}}}, contract.Transaction{}, old, false)
	if len(h.redo) != 0 || h.bytes != 1 {
		t.Fatal("discarding redo leaked inverse bytes", h.bytes)
	}
	// A single step exceeding the budget cannot be retained.
	h.record(historyStep{edits: []contract.TextEdit{{Text: oversize}, {Text: "xx"}}}, contract.Transaction{}, old, false)
	assertBudget(t, &h)
	if len(h.undo) != 0 || h.bytes != 0 {
		t.Fatal("oversized step retained")
	}
}
