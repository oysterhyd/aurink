package contracttest

import (
	"math/rand"
	"testing"
	"time"

	"aurink/internal/contract"
)

// DocumentFixture creates documents for the suite.
type DocumentFixture struct {
	// New returns a document holding text.
	New func(text string) contract.DocumentModel
	// Advance moves the clock the document uses for coalescing.
	Advance func(d time.Duration)
}

func insertAt(off int, text string) contract.Transaction {
	return contract.Transaction{
		Edits:  []contract.TextEdit{{Range: contract.Range{Start: off, End: off}, Text: text}},
		Origin: contract.OriginUser,
	}
}

// TestDocument checks docs/04 §4.1 for a document model.
func TestDocument(t *testing.T, newFixture func(t *testing.T) DocumentFixture) {
	apply := func(t *testing.T, d contract.DocumentModel, tx contract.Transaction) contract.Version {
		t.Helper()
		v, err := d.Apply(tx)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	t.Run("versions", func(t *testing.T) {
		d := newFixture(t).New("ab")
		v0 := d.Version()
		events := 0
		defer d.Subscribe(func(contract.DocEvent) { events++ })()
		if v := apply(t, d, contract.Transaction{Origin: contract.OriginUser}); v != v0 || events != 0 {
			t.Errorf("an empty transaction: version %d, %d events", v, events)
		}
		if v := apply(t, d, insertAt(1, "x")); v != v0+1 || d.Version() != v || d.Text().Version() != v || d.AST().Version() != v {
			t.Errorf("version %d, model %d, text %d, AST %d", v, d.Version(), d.Text().Version(), d.AST().Version())
		}
	})
	t.Run("invalid transactions change nothing", func(t *testing.T) {
		d := newFixture(t).New("abcd")
		apply(t, d, insertAt(0, "x"))
		v := d.Version()
		sel := contract.Caret(2)
		d.SetSelection(sel)
		for _, edits := range [][]contract.TextEdit{
			{{Range: contract.Range{Start: 9, End: 9}}},
			{{Range: contract.Range{Start: 2, End: 1}}},
			{{Range: contract.Range{Start: 2, End: 3}}, {Range: contract.Range{Start: 1, End: 2}}},
			{{Range: contract.Range{Start: 1, End: 3}}, {Range: contract.Range{Start: 2, End: 4}}},
		} {
			_, err := d.Apply(contract.Transaction{Edits: edits, Origin: contract.OriginUser})
			wantCode(t, err, contract.CodeInvalid)
			if d.Version() != v || d.Text().String() != "xabcd" || d.Selection().Primary() != sel.Primary() {
				t.Fatalf("the document changed: %q, version %d", d.Text().String(), d.Version())
			}
		}
		if !d.CanUndo() {
			t.Error("the history was lost")
		}
		d.Undo()
		if d.CanUndo() || d.Text().String() != "abcd" {
			t.Errorf("after undo: %q, CanUndo %v", d.Text().String(), d.CanUndo())
		}
	})
	t.Run("read only", func(t *testing.T) {
		d := newFixture(t).New("a")
		d.SetReadOnly(true)
		_, err := d.Apply(insertAt(0, "x"))
		wantCode(t, err, contract.CodeReadOnly)
		if !d.ReadOnly() || d.Text().String() != "a" {
			t.Errorf("read only %v, text %q", d.ReadOnly(), d.Text().String())
		}
	})
	t.Run("selection mapping", func(t *testing.T) {
		for _, tt := range []struct {
			name                 string
			from                 contract.SelRange
			at                   int
			text                 string
			wantAnchor, wantHead int
		}{
			{"caret at the insertion moves after it", contract.SelRange{Anchor: 2, Head: 2}, 2, "xy", 4, 4},
			{"caret before stays", contract.SelRange{Anchor: 1, Head: 1}, 2, "xy", 1, 1},
			{"caret after shifts", contract.SelRange{Anchor: 3, Head: 3}, 2, "xy", 5, 5},
			{"a range start does not expand", contract.SelRange{Anchor: 2, Head: 4}, 2, "xy", 4, 6},
			{"a range end does not expand", contract.SelRange{Anchor: 1, Head: 2}, 2, "xy", 1, 2},
		} {
			t.Run(tt.name, func(t *testing.T) {
				d := newFixture(t).New("abcdef")
				d.SetSelection(contract.Selection{Ranges: []contract.SelRange{tt.from}})
				apply(t, d, insertAt(tt.at, tt.text))
				if got := d.Selection().Primary(); got.Anchor != tt.wantAnchor || got.Head != tt.wantHead {
					t.Errorf("selection %+v", got)
				}
			})
		}
	})
	t.Run("undo restores the selection", func(t *testing.T) {
		d := newFixture(t).New("abc")
		d.SetSelection(contract.Caret(1))
		after := contract.Caret(2)
		tx := insertAt(1, "x")
		tx.Selection = &after
		apply(t, d, tx)
		if !d.Undo() || d.Selection().Primary() != contract.Caret(1).Primary() || d.Text().String() != "abc" {
			t.Errorf("after undo: %q, %+v", d.Text().String(), d.Selection())
		}
		if !d.Redo() || d.Selection().Primary() != after.Primary() || d.Text().String() != "axbc" {
			t.Errorf("after redo: %q, %+v", d.Text().String(), d.Selection())
		}
		if d.Redo() {
			t.Error("a second redo")
		}
	})
	t.Run("coalescing", func(t *testing.T) {
		f := newFixture(t)
		typed := func(off int, s string) contract.Transaction {
			tx := insertAt(off, s)
			tx.Coalesce = "typing"
			return tx
		}
		d := f.New("")
		apply(t, d, typed(0, "a"))
		f.Advance(500 * time.Millisecond)
		apply(t, d, typed(1, "b"))
		d.Undo()
		if d.Text().String() != "" || d.CanUndo() {
			t.Errorf("adjacent typing is not one step: %q", d.Text().String())
		}

		d = f.New("")
		apply(t, d, typed(0, "a"))
		f.Advance(2 * time.Second)
		apply(t, d, typed(1, "b"))
		d.Undo()
		if d.Text().String() != "a" {
			t.Errorf("a pause did not end the step: %q", d.Text().String())
		}

		d = f.New("")
		apply(t, d, typed(0, "a"))
		d.BreakCoalescing()
		apply(t, d, typed(1, "b"))
		d.Undo()
		if d.Text().String() != "a" {
			t.Errorf("BreakCoalescing did not end the step: %q", d.Text().String())
		}

		d = f.New("xxxx")
		apply(t, d, typed(0, "a"))
		apply(t, d, typed(4, "b"))
		d.Undo()
		if d.Text().String() != "axxxx" {
			t.Errorf("non-adjacent typing merged: %q", d.Text().String())
		}

		// An input rule is its own step after the typing that triggered it.
		d = f.New("")
		apply(t, d, typed(0, "a"))
		rule := insertAt(1, "b")
		rule.Origin = contract.OriginInputRule
		apply(t, d, rule)
		d.Undo()
		if d.Text().String() != "a" {
			t.Errorf("an input rule merged with typing: %q", d.Text().String())
		}
	})
	t.Run("Replace is one step", func(t *testing.T) {
		d := newFixture(t).New("old")
		var got []contract.DocEvent
		defer d.Subscribe(func(e contract.DocEvent) { got = append(got, e) })()
		d.Replace("new\n", contract.OriginReload)
		if d.Text().String() != "new\n" || len(got) != 1 || got[0].Kind != contract.DocReplaced || got[0].Origin != contract.OriginReload {
			t.Fatalf("text %q, events %+v", d.Text().String(), got)
		}
		if c := got[0].Changes; len(c) != 1 || c[0].OldEnd != 3 || c[0].NewEnd != 4 {
			t.Errorf("changes %+v", c)
		}
		d.Undo()
		if d.Text().String() != "old" {
			t.Errorf("after undo: %q", d.Text().String())
		}
	})
	t.Run("events", func(t *testing.T) {
		d := newFixture(t).New("一\n\n二\n")
		var got []contract.DocEvent
		off := d.Subscribe(func(e contract.DocEvent) { got = append(got, e) })
		edited := d.AST().Block(d.AST().LeafAt(0)).ID
		other := d.AST().Block(d.AST().LeafAt(5)).ID
		apply(t, d, insertAt(0, "x"))
		if len(got) == 0 {
			t.Fatal("no event before Apply returned")
		}
		e := got[0]
		if e.Kind != contract.DocTextChanged || e.Version != d.Version() || len(e.Changes) != 1 || e.Origin != contract.OriginUser {
			t.Errorf("event %+v", e)
		}
		dirty := map[contract.BlockID]bool{}
		for _, id := range e.Dirty {
			dirty[id] = true
		}
		if !dirty[edited] || dirty[other] {
			t.Errorf("Dirty %v, edited %d, moved %d", e.Dirty, edited, other)
		}
		got = nil
		d.SetSelection(contract.Caret(0))
		if len(got) != 1 || got[0].Kind != contract.DocSelectionChanged {
			t.Errorf("selection events %+v", got)
		}
		got = nil
		d.SetSelection(contract.Caret(0))
		if len(got) != 0 {
			t.Errorf("events for the same selection: %+v", got)
		}
		off()
		got = nil
		apply(t, d, insertAt(0, "y"))
		if len(got) != 0 {
			t.Errorf("events after unsubscribing: %+v", got)
		}
	})
	t.Run("a changed definition dirties its users", func(t *testing.T) {
		src := "[a][r]\n\n[r]: /u\n"
		d := newFixture(t).New(src)
		user := d.AST().Block(d.AST().LeafAt(0)).ID
		var dirty []contract.BlockID
		defer d.Subscribe(func(e contract.DocEvent) { dirty = append(dirty, e.Dirty...) })()
		apply(t, d, contract.Transaction{Edits: []contract.TextEdit{{Range: contract.Range{Start: len(src) - 2, End: len(src) - 1}, Text: "v"}}, Origin: contract.OriginUser})
		found := false
		for _, id := range dirty {
			found = found || id == user
		}
		if !found {
			t.Errorf("the user %d is not in Dirty %v", user, dirty)
		}
	})
	t.Run("a panicking subscriber does not stop the others", func(t *testing.T) {
		d := newFixture(t).New("a")
		ran := false
		defer d.Subscribe(func(contract.DocEvent) { panic("subscriber") })()
		defer d.Subscribe(func(contract.DocEvent) { ran = true })()
		apply(t, d, insertAt(0, "x"))
		if !ran {
			t.Error("the second subscriber did not run")
		}
	})
	t.Run("random transactions match a string model", func(t *testing.T) {
		for _, seed := range []int64{1, 2, 3} {
			f := newFixture(t)
			rng := rand.New(rand.NewSource(seed))
			start := "# 标题\n\n段落 **粗**\n\n- 项\n"
			model := start
			d := f.New(start)
			history := []string{model}
			for step := 0; step < 500; step++ {
				edit := randomEdit(rng, model)
				if edit.Range.Empty() && edit.Text == "" {
					continue
				}
				if _, err := d.Apply(contract.Transaction{Edits: []contract.TextEdit{edit}, Origin: contract.OriginCommand}); err != nil {
					t.Fatal(err)
				}
				model = model[:edit.Range.Start] + edit.Text + model[edit.Range.End:]
				history = append(history, model)
				if d.Text().String() != model {
					t.Fatalf("seed %d step %d: %q, want %q", seed, step, d.Text().String(), model)
				}
			}
			n := len(history) - 1
			for i := n; i > 0; i-- {
				if !d.Undo() {
					t.Fatalf("seed %d: undo stopped at %d", seed, i)
				}
				if d.Text().String() != history[i-1] {
					t.Fatalf("seed %d undo to %d: %q", seed, i-1, d.Text().String())
				}
			}
			for i := 1; i <= n; i++ {
				if !d.Redo() || d.Text().String() != history[i] {
					t.Fatalf("seed %d redo to %d: %q", seed, i, d.Text().String())
				}
			}
		}
	})
}
