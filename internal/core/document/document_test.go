package document

import (
	"bytes"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"aurink/internal/contract"
	"aurink/internal/core/markdown"
	"aurink/internal/core/text"
	"aurink/tests/corpus"
)

func newModel(t testing.TB, source string) *Model {
	t.Helper()
	m, ok := (Factory{Clock: func() time.Time { return time.Unix(10, 0) }}).New("test", source, contract.DocMeta{}, contract.ExtFrontMatter).(*Model)
	if !ok {
		t.Fatal("factory returned no model")
	}
	return m
}

func TestDefaultFactory(t *testing.T) {
	m := (Factory{}).New("default", "text", contract.DocMeta{}, 0)
	if m == nil || m.ID() != "default" || m.Version() != 1 {
		t.Fatal("zero-value factory")
	}
	if _, err := m.Apply(edit(0, 0, "a")); err != nil {
		t.Fatal(err)
	}
}

func apply(t testing.TB, m *Model, tx contract.Transaction) contract.Version {
	t.Helper()
	v, err := m.Apply(tx)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func edit(at, end int, value string) contract.Transaction {
	return contract.Transaction{Edits: []contract.TextEdit{{Range: contract.Range{Start: at, End: end}, Text: value}}, Origin: contract.OriginUser, Coalesce: "typing", Label: "editing_undo_typing"}
}

func assertBudget(t testing.TB, h *history) {
	t.Helper()
	count, size := len(h.undo)+len(h.redo), 0
	for _, stack := range [][]historyStep{h.undo, h.redo} {
		for _, step := range stack {
			size += step.bytes()
		}
	}
	if count > maxHistorySteps || size > maxHistoryBytes || h.bytes != size {
		t.Fatalf("history: count=%d bytes=%d tracked=%d", count, size, h.bytes)
	}
}

func assertAST(t testing.TB, m *Model) {
	t.Helper()
	p, err := markdown.New(m.Text().String(), m.Version(), contract.ExtFrontMatter)
	if err != nil {
		t.Fatal(err)
	}
	a, b := m.AST(), p.Snapshot()
	if a.Version() != m.Version() || a.Len() != b.Len() || a.LeafCount() != b.LeafCount() {
		t.Fatalf("AST version/count: %d/%d blocks %d/%d leaves %d/%d", a.Version(), m.Version(), a.Len(), b.Len(), a.LeafCount(), b.LeafCount())
	}
	for i := 0; i < a.Len(); i++ {
		x, y := *a.Block(i), *b.Block(i)
		x.ID, y.ID = 0, 0
		if !reflect.DeepEqual(x, y) || !reflect.DeepEqual(a.Inlines(i), b.Inlines(i)) {
			t.Fatalf("AST block %d differs from fresh parse: %+v / %+v; source=%q", i, x, y, m.Text().String())
		}
	}
	x, y := slices.Clone(a.Outline()), slices.Clone(b.Outline())
	for i := range x {
		x[i].Block = 0
	}
	for i := range y {
		y[i].Block = 0
	}
	if !reflect.DeepEqual(x, y) || !reflect.DeepEqual(a.LinkDefs(), b.LinkDefs()) || !reflect.DeepEqual(a.Footnotes(), b.Footnotes()) || !slices.Equal(slices.Collect(a.Leaves()), slices.Collect(b.Leaves())) {
		t.Fatal("AST derived tables differ from fresh parse")
	}
}

func TestStateAndSelectionOwnership(t *testing.T) {
	m := newModel(t, "abc")
	if m.ID() != "test" || m.Meta() != (contract.DocMeta{}) || m.UndoLabel() != "" || m.RedoLabel() != "" || m.Undo() {
		t.Fatal("initial state")
	}
	var kinds []contract.DocEventKind
	m.Subscribe(func(e contract.DocEvent) { kinds = append(kinds, e.Kind) })
	meta := contract.DocMeta{EOL: contract.EOLCRLF, Dominant: "\r\n", Encoding: "utf-16le", BOM: true}
	m.SetMeta(meta)
	m.SetMeta(meta)
	m.SetReadOnly(true)
	m.SetReadOnly(true)
	if m.Meta() != meta || !slices.Equal(kinds, []contract.DocEventKind{contract.DocMetaChanged, contract.DocReadOnlyChanged}) {
		t.Fatal("state events", kinds)
	}
	m.Replace("new", contract.OriginReload)
	if m.Text().String() != "abc" || m.CanUndo() {
		t.Fatal("read-only Replace")
	}
	m.SetReadOnly(false)
	sel := contract.Selection{Ranges: []contract.SelRange{{Anchor: -10, Head: 99}, {Anchor: 3, Head: 1}}}
	m.SetSelection(sel)
	sel.Ranges[0] = contract.SelRange{}
	want := contract.Selection{Ranges: []contract.SelRange{{Anchor: 0, Head: 3}, {Anchor: 3, Head: 1}}}
	if !sameSelection(m.Selection(), want) {
		t.Fatal("selection clamp/copy", m.Selection())
	}
	returned := m.Selection()
	returned.Ranges[0] = contract.SelRange{}
	if !sameSelection(m.Selection(), want) {
		t.Fatal("selection getter aliases state")
	}
	oldVersion := m.Version()
	post := contract.Caret(2)
	apply(t, m, contract.Transaction{Selection: &post, Origin: contract.OriginAPI})
	post.Ranges[0].Head = 0
	if m.Version() != oldVersion+1 || m.AST().Version() != m.Version() || m.CanUndo() || !sameSelection(m.Selection(), contract.Caret(2)) {
		t.Fatal("selection-only transaction")
	}
	v := m.Version()
	apply(t, m, contract.Transaction{Selection: &m.selection})
	if m.Version() != v {
		t.Fatal("unchanged selection advanced the version")
	}
	bad := contract.Caret(9)
	_, err := m.Apply(contract.Transaction{Edits: []contract.TextEdit{{Range: contract.Range{Start: 1, End: 1}, Text: "z"}}, Selection: &bad})
	if contract.CodeOf(err) != contract.CodeInvalid || m.Version() != v || m.Text().String() != "abc" || m.CanUndo() {
		t.Fatal("invalid selection committed", err)
	}
	m.SetSelection(contract.Selection{})
	apply(t, m, edit(1, 1, "z"))
	if m.UndoLabel() != "editing_undo_typing" {
		t.Fatal("undo label")
	}
	m.SetReadOnly(true)
	if m.Undo() || !m.CanUndo() {
		t.Fatal("read-only Undo changed history")
	}
	m.SetReadOnly(false)
	if !m.Undo() || m.RedoLabel() != "editing_undo_typing" {
		t.Fatal("redo label")
	}
	m.SetReadOnly(true)
	if m.Redo() || !m.CanRedo() {
		t.Fatal("read-only Redo changed history")
	}
	m.SetReadOnly(false)
	apply(t, m, edit(0, 0, "x"))
	if m.CanRedo() || m.RedoLabel() != "" {
		t.Fatal("new edit preserved redo")
	}
	assertBudget(t, &m.history)
}

func TestVersionExhaustion(t *testing.T) {
	m := newModel(t, "abc")
	m.text = text.New("abc", ^contract.Version(0))
	for _, tx := range []contract.Transaction{edit(0, 0, "x"), {Selection: selectionPtr(contract.Caret(1))}} {
		_, err := m.Apply(tx)
		if contract.CodeOf(err) != contract.CodeInvalid || m.Text().String() != "abc" || m.CanUndo() {
			t.Fatal("exhausted version changed state", err)
		}
	}
}

func selectionPtr(s contract.Selection) *contract.Selection { return &s }

func TestReentrantSubscribers(t *testing.T) {
	m := newModel(t, "a")
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	var order []string
	var cancelSecond func()
	m.Subscribe(func(e contract.DocEvent) {
		if e.Version == 2 {
			order = append(order, "first")
			cancelSecond()
			m.Subscribe(func(e contract.DocEvent) { order = append(order, "new") })
			if !m.CanUndo() || m.Text().Version() != m.AST().Version() {
				t.Fatal("callback observed partial commit")
			}
			apply(t, m, edit(0, 0, "nested"))
		}
	})
	cancelSecond = m.Subscribe(func(e contract.DocEvent) { order = append(order, "cancelled") })
	m.Subscribe(nil)
	m.Subscribe(func(e contract.DocEvent) { panic("test panic") })
	m.Subscribe(func(e contract.DocEvent) { order = append(order, "last") })
	v := apply(t, m, edit(0, 0, "outer"))
	if v != 2 || m.Version() != 3 || m.Text().String() != "nestedoutera" || !slices.Equal(order, []string{"first", "last", "last", "new"}) {
		t.Fatal("reentrant delivery", v, m.Version(), order)
	}
	cancelSecond()
	if !strings.Contains(logs.String(), "document subscriber panicked") || !strings.Contains(logs.String(), "test panic") {
		t.Fatal("subscriber panic was not logged", logs.String())
	}
	// History is already on the opposite stack before replay events run.
	undoObserved := false
	m.Subscribe(func(e contract.DocEvent) {
		if e.Origin == contract.OriginUndo && e.Kind == contract.DocTextChanged && !undoObserved {
			undoObserved = true
			if !m.CanRedo() || !m.Redo() {
				t.Fatal("reentrant redo")
			}
		}
	})
	if !m.Undo() || !undoObserved || m.Text().String() != "nestedoutera" {
		t.Fatal("undo callback replay")
	}
}

func TestIncrementalEditsAndReplace(t *testing.T) {
	source := corpus.Generate(200<<10, 1331)
	for _, c := range ordinaryCases(t, source) {
		t.Run(c.name, func(t *testing.T) {
			m := newModel(t, source)
			m.SetSelection(contract.Caret(c.at))
			var events []contract.DocEvent
			m.Subscribe(func(e contract.DocEvent) { events = append(events, e) })
			oldText, oldAST := m.Text(), m.AST()
			beforeBlock := *oldAST.Block(oldAST.LeafAt(c.at))
			beforeInline := slices.Clone(oldAST.Inlines(oldAST.LeafAt(c.at)))
			apply(t, m, edit(c.at, c.at, c.value))
			if m.lastUpdate.FullRebuilds != 0 || m.lastUpdate.AdaptedBlocks >= m.AST().Len()/2 {
				t.Fatal("ordinary Apply rebuilt the document", m.lastUpdate)
			}
			if !m.Undo() || m.Text().String() != source || m.lastUpdate.FullRebuilds != 0 || m.lastUpdate.AdaptedBlocks >= m.AST().Len()/2 {
				t.Fatal("ordinary Undo rebuilt the document", m.lastUpdate)
			}
			if !m.Redo() || m.lastUpdate.FullRebuilds != 0 || m.lastUpdate.AdaptedBlocks >= m.AST().Len()/2 {
				t.Fatal("ordinary Redo rebuilt the document", m.lastUpdate)
			}
			if oldText.String() != source || !reflect.DeepEqual(beforeBlock, *oldAST.Block(oldAST.LeafAt(c.at))) || !reflect.DeepEqual(beforeInline, oldAST.Inlines(oldAST.LeafAt(c.at))) {
				t.Fatal("retained snapshots mutated")
			}
			for _, e := range events {
				if e.Kind == contract.DocTextChanged && e.Origin != contract.OriginUser && e.Origin != contract.OriginUndo && e.Origin != contract.OriginRedo {
					t.Fatal("replay event origin", e)
				}
			}
			assertAST(t, m)
		})
	}
	m := newModel(t, source)
	p, err := markdown.New(source, 1, contract.ExtFrontMatter)
	if err != nil {
		t.Fatal(err)
	}
	var events []contract.DocEvent
	m.Subscribe(func(e contract.DocEvent) { events = append(events, e) })
	replacement := "---\r\ntitle: New\r\n---\r\n\r\n# New\r\n\r\nbody\r\n"
	m.Replace(replacement, contract.OriginReload)
	changes := []contract.Change{{OldEnd: len(source), NewEnd: len(replacement)}}
	u, err := p.Update(m.Text(), changes)
	if err != nil {
		t.Fatal(err)
	}
	if m.lastUpdate.FullRebuilds > 1 || m.lastUpdate != u.Stats || len(m.history.undo) != 1 || len(events) != 1 || events[0].Kind != contract.DocReplaced || !slices.Equal(events[0].Changes, changes) || !slices.Equal(events[0].Dirty, u.Dirty) {
		t.Fatal("Replace work/events", m.lastUpdate, events)
	}
	assertAST(t, m)
	if !m.Undo() || m.Text().String() != source || !m.Redo() || m.Text().String() != replacement {
		t.Fatal("Replace history")
	}
	assertAST(t, m)
}
