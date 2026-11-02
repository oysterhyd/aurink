package document

import (
	"log/slog"
	"slices"
	"strings"
	"time"

	"aurink/internal/contract"
	"aurink/internal/core/markdown"
	"aurink/internal/core/text"
)

// Factory creates independent document models. Its zero value uses time.Now.
type Factory struct {
	// Clock supplies the time used to coalesce edits; nil means time.Now.
	Clock func() time.Time
}

var _ contract.DocumentFactory = Factory{}
var _ contract.DocumentModel = (*Model)(nil)

// New creates a document at version 1 with a caret at byte zero.
// An internal parser failure is logged and returns nil.
func (f Factory) New(id contract.DocID, source string, meta contract.DocMeta, exts contract.Extensions) contract.DocumentModel {
	p, err := markdown.New(source, 1, exts)
	if err != nil {
		slog.Error("document creation failed", "document", id, "error", err)
		return nil
	}
	clock := f.Clock
	if clock == nil {
		clock = time.Now
	}
	return &Model{id: id, text: text.New(source, 1), parser: p, meta: meta, selection: contract.Caret(0), clock: clock}
}

// Model owns transactions, history and synchronous events on the calling thread.
// Returned text and AST snapshots are immutable and support concurrent readers.
type Model struct {
	id          contract.DocID
	text        *text.Text
	parser      *markdown.Parser
	meta        contract.DocMeta
	selection   contract.Selection
	readOnly    bool
	clock       func() time.Time
	history     history
	subscribers []*subscriber
	pending     []contract.DocEvent
	emitting    bool
	lastUpdate  markdown.Stats
}

// ID returns the session identity.
func (m *Model) ID() contract.DocID { return m.id }

// Text returns the current immutable text snapshot.
func (m *Model) Text() contract.Text { return m.text }

// AST returns the current immutable syntax snapshot.
func (m *Model) AST() contract.AST { return m.parser.Snapshot() }

// Version returns the current transaction version.
func (m *Model) Version() contract.Version { return m.text.Version() }

// Meta returns the file metadata.
func (m *Model) Meta() contract.DocMeta { return m.meta }

// SetMeta updates metadata and emits an event if it changed.
func (m *Model) SetMeta(meta contract.DocMeta) {
	if m.meta != meta {
		m.meta = meta
		m.emit(contract.DocEvent{Kind: contract.DocMetaChanged, Version: m.Version()})
	}
}

// Selection returns an owned copy of the source selection.
func (m *Model) Selection() contract.Selection { return cloneSelection(m.selection) }

// SetSelection clamps endpoints to the document. A change ends coalescing and
// emits an event; setting the same selection does neither.
func (m *Model) SetSelection(sel contract.Selection) {
	sel = cloneSelection(sel)
	for i := range sel.Ranges {
		sel.Ranges[i].Anchor = min(max(sel.Ranges[i].Anchor, 0), m.text.Len())
		sel.Ranges[i].Head = min(max(sel.Ranges[i].Head, 0), m.text.Len())
	}
	if !sameSelection(m.selection, sel) {
		m.selection = sel
		m.BreakCoalescing()
		m.emit(contract.DocEvent{Kind: contract.DocSelectionChanged, Version: m.Version()})
	}
}

// Apply validates and commits a transaction, then delivers its events.
func (m *Model) Apply(tx contract.Transaction) (contract.Version, error) {
	return m.apply(tx, contract.DocTextChanged, 0)
}

// Replace replaces all bytes as one undo step, preserving the supplied origin.
// The selection keeps its offsets, clamped to the new text, so that a reload
// does not throw the caret to the end. A read-only document is unchanged.
func (m *Model) Replace(source string, origin contract.Origin) contract.Version {
	sel := keepSelection(m.selection, source)
	v, err := m.apply(contract.Transaction{Edits: []contract.TextEdit{{Range: contract.Range{End: m.text.Len()}, Text: source}}, Selection: &sel, Origin: origin}, contract.DocReplaced, 0)
	if err != nil && contract.CodeOf(err) != contract.CodeReadOnly {
		slog.Error("document replacement failed", "document", m.id, "error", err)
	}
	return v
}

// Undo reverses the latest step, including its selection.
func (m *Model) Undo() bool { return m.replay(false) }

// Redo reapplies the latest undone step, including its selection.
func (m *Model) Redo() bool { return m.replay(true) }

// CanUndo reports whether an undo step is retained.
func (m *Model) CanUndo() bool { return len(m.history.undo) != 0 }

// CanRedo reports whether a redo step is retained.
func (m *Model) CanRedo() bool { return len(m.history.redo) != 0 }

// UndoLabel returns the latest undo step's localization key.
func (m *Model) UndoLabel() string { return stepLabel(m.history.undo) }

// RedoLabel returns the latest redo step's localization key.
func (m *Model) RedoLabel() string { return stepLabel(m.history.redo) }

// BreakCoalescing prevents the next edit from joining the previous step.
func (m *Model) BreakCoalescing() { m.history.open = false }

// ReadOnly reports whether text mutations are disabled.
func (m *Model) ReadOnly() bool { return m.readOnly }

// SetReadOnly updates the edit permission and emits an event if it changed.
func (m *Model) SetReadOnly(ro bool) {
	if m.readOnly != ro {
		m.readOnly = ro
		m.BreakCoalescing()
		m.emit(contract.DocEvent{Kind: contract.DocReadOnlyChanged, Version: m.Version()})
	}
}

func (m *Model) apply(tx contract.Transaction, kind contract.DocEventKind, replay int) (contract.Version, error) {
	v := m.Version()
	if m.readOnly {
		return v, &contract.Error{Code: contract.CodeReadOnly, Op: "document.Apply"}
	}
	next, changes, err := m.text.Apply(tx.Edits)
	if err != nil {
		return v, err
	}
	var selection contract.Selection
	if tx.Selection != nil {
		selection = cloneSelection(*tx.Selection)
	} else {
		selection = mapSelection(m.selection, changes)
	}
	if !validSelection(selection, next.Len()) {
		return v, &contract.Error{Code: contract.CodeInvalid, Op: "document.Apply", Detail: "selection is out of bounds"}
	}
	selectionChanged := !sameSelection(m.selection, selection)
	if len(changes) == 0 {
		if !selectionChanged {
			return v, nil
		}
		if v == ^contract.Version(0) {
			return v, &contract.Error{Code: contract.CodeInvalid, Op: "document.Apply", Detail: "version is exhausted"}
		}
		next = text.New(m.text.String(), v+1)
		kind = contract.DocSelectionChanged
	}
	step := historyStep{edits: inverseEdits(m.text, changes), before: m.Selection(), after: selection, label: tx.Label, key: tx.Coalesce, origin: tx.Origin}
	update, err := m.parser.Update(next, changes)
	if err != nil {
		return v, err
	}
	if len(changes) > 0 {
		if replay == 0 {
			step.at = m.clock()
			m.history.record(step, tx, m.text, kind == contract.DocReplaced)
		} else {
			m.history.move(step, replay)
		}
	} else {
		m.BreakCoalescing()
	}
	m.text, m.selection, m.lastUpdate = next, selection, update.Stats
	v = next.Version()
	m.emit(contract.DocEvent{Kind: kind, Version: v, Changes: changes, Dirty: update.Dirty, Origin: tx.Origin})
	if replay != 0 && selectionChanged {
		m.emit(contract.DocEvent{Kind: contract.DocSelectionChanged, Version: v, Origin: tx.Origin})
	}
	return v, nil
}

func (m *Model) replay(redo bool) bool {
	steps, origin, direction := m.history.undo, contract.OriginUndo, -1
	if redo {
		steps, origin, direction = m.history.redo, contract.OriginRedo, 1
	}
	if len(steps) == 0 {
		return false
	}
	step := steps[len(steps)-1]
	sel := step.before
	if redo {
		sel = step.after
	}
	_, err := m.apply(contract.Transaction{Edits: step.edits, Selection: &sel, Origin: origin}, contract.DocTextChanged, direction)
	return err == nil
}

func inverseEdits(old *text.Text, changes []contract.Change) []contract.TextEdit {
	edits := make([]contract.TextEdit, len(changes))
	for i, c := range changes {
		edits[i] = contract.TextEdit{Range: contract.Range{Start: c.NewStart, End: c.NewEnd}, Text: strings.Clone(old.Slice(contract.Range{Start: c.OldStart, End: c.OldEnd}))}
	}
	return edits
}

type subscriber struct{ fn func(contract.DocEvent) }

// Subscribe registers a callback in delivery order. Cancellation is idempotent.
// An edit made from inside a callback returns at once; its events follow the
// current one, so every subscriber sees versions in order.
// New subscriptions start with the next event; cancellation takes effect at once.
func (m *Model) Subscribe(fn func(contract.DocEvent)) func() {
	s := &subscriber{fn: fn}
	m.subscribers = append(m.subscribers, s)
	return func() {
		if i := slices.Index(m.subscribers, s); i >= 0 {
			m.subscribers = slices.Delete(m.subscribers, i, i+1)
		}
		s.fn = nil
	}
}

func (m *Model) emit(event contract.DocEvent) {
	m.pending = append(m.pending, event)
	if m.emitting {
		return
	}
	m.emitting = true
	defer func() { m.emitting = false }()
	for len(m.pending) > 0 {
		next := m.pending[0]
		m.pending = m.pending[1:]
		m.deliver(next)
	}
	m.pending = nil
}

func (m *Model) deliver(event contract.DocEvent) {
	for _, s := range slices.Clone(m.subscribers) {
		if s.fn != nil {
			func() {
				defer func() {
					if p := recover(); p != nil {
						slog.Error("document subscriber panicked", "document", m.id, "panic", p)
					}
				}()
				s.fn(event)
			}()
		}
	}
}
