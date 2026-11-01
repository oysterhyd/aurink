package document

import (
	"slices"
	"testing"

	"aurink/internal/contract"
)

func TestReentrantEditKeepsEventOrder(t *testing.T) {
	m := newModel(t, "abc")
	var seen []contract.Version
	m.Subscribe(func(e contract.DocEvent) {
		if e.Kind == contract.DocTextChanged && e.Version == 2 {
			apply(t, m, edit(0, 0, "y"))
		}
	})
	m.Subscribe(func(e contract.DocEvent) {
		if e.Kind == contract.DocTextChanged {
			seen = append(seen, e.Version)
		}
	})
	apply(t, m, edit(0, 0, "x"))
	if !slices.Equal(seen, []contract.Version{2, 3}) {
		t.Fatalf("versions %v", seen)
	}
	if m.pending != nil || m.emitting {
		t.Fatalf("delivery did not finish: %v %v", m.pending, m.emitting)
	}
}

func TestSelectionInsideReplacedSpanCollapses(t *testing.T) {
	for _, r := range []contract.SelRange{{Anchor: 2, Head: 3}, {Anchor: 3, Head: 2}} {
		m := newModel(t, "abcdef")
		m.SetSelection(contract.Selection{Ranges: []contract.SelRange{r}})
		apply(t, m, edit(1, 4, "xy"))
		if got := m.Selection().Primary(); got.Anchor != 3 || got.Head != 3 {
			t.Errorf("%+v became %+v", r, got)
		}
	}
}

func TestReplaceKeepsSelectionOffsets(t *testing.T) {
	m := newModel(t, "line one\nline two\n")
	m.SetSelection(contract.Caret(5))
	m.Replace("line one\nline 2\nmore\n", contract.OriginReload)
	if got := m.Selection().Primary(); got.Head != 5 || got.Anchor != 5 {
		t.Errorf("selection %+v", got)
	}
	m.SetSelection(contract.Caret(20))
	m.Replace("short", contract.OriginReload)
	if got := m.Selection().Primary(); got.Head != 5 {
		t.Errorf("not clamped: %+v", got)
	}
	m.SetSelection(contract.Caret(1))
	m.Replace("中文", contract.OriginReload)
	if got := m.Selection().Primary(); got.Head != 0 {
		t.Errorf("inside a character: %+v", got)
	}
	m.Undo()
	if got := m.Selection().Primary(); got.Head != 1 || m.Text().String() != "short" {
		t.Errorf("undo: %q %+v", m.Text().String(), got)
	}
}

func TestSameSelectionSendsNoEvent(t *testing.T) {
	m := newModel(t, "abc")
	m.SetSelection(contract.Caret(1))
	n := 0
	m.Subscribe(func(contract.DocEvent) { n++ })
	m.SetSelection(contract.Caret(1))
	if n != 0 {
		t.Errorf("%d events", n)
	}
}

func TestMultiEditUndoRedo(t *testing.T) {
	m := newModel(t, "aaaa bbbb cccc")
	m.SetSelection(contract.Caret(7))
	_, err := m.Apply(contract.Transaction{Edits: []contract.TextEdit{
		{Range: contract.Range{Start: 0, End: 2}, Text: "X"},
		{Range: contract.Range{Start: 5, End: 5}, Text: "YYY"},
		{Range: contract.Range{Start: 10, End: 14}},
	}, Origin: contract.OriginCommand})
	if err != nil {
		t.Fatal(err)
	}
	after, sel := m.Text().String(), m.Selection()
	m.Undo()
	if m.Text().String() != "aaaa bbbb cccc" || m.Selection().Primary().Head != 7 {
		t.Errorf("undo: %q %+v", m.Text().String(), m.Selection())
	}
	m.Redo()
	if m.Text().String() != after || !sameSelection(m.Selection(), sel) {
		t.Errorf("redo: %q %+v", m.Text().String(), m.Selection())
	}
}
