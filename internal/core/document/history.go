package document

import (
	"slices"
	"strings"
	"time"

	"aurink/internal/contract"
	"aurink/internal/core/text"
)

const (
	maxHistorySteps = 1000
	maxHistoryBytes = 64 << 20
)

type historyStep struct {
	edits         []contract.TextEdit
	before, after contract.Selection
	label, key    string
	origin        contract.Origin
	at            time.Time
	last          contract.Range
}

func (s historyStep) bytes() int {
	n := 0
	for _, e := range s.edits {
		n += len(e.Text)
	}
	return n
}

type history struct {
	undo, redo []historyStep
	bytes      int
	open       bool
}

func stepLabel(steps []historyStep) string {
	if len(steps) == 0 {
		return ""
	}
	return steps[len(steps)-1].label
}

func (h *history) record(s historyStep, tx contract.Transaction, old *text.Text, replace bool) {
	for _, r := range h.redo {
		h.bytes -= r.bytes()
	}
	h.redo = nil
	eligible := !replace && s.key != "" && len(tx.Edits) == 1 && (s.origin == contract.OriginUser || s.origin == contract.OriginSource)
	if eligible {
		s.last = s.edits[0].Range
	}
	for _, e := range tx.Edits {
		if strings.ContainsAny(e.Text, "\r\n") || strings.ContainsAny(old.Slice(e.Range), "\r\n") {
			eligible = false
		}
	}
	merged := false
	if eligible && h.open && len(h.undo) != 0 {
		p := &h.undo[len(h.undo)-1]
		gap := s.at.Sub(p.at)
		a, b := p.edits[0], tx.Edits[0]
		if p.key == s.key && p.origin == s.origin && gap >= 0 && gap < time.Second && sameSelection(p.after, s.before) && b.Range.Start <= p.last.End && b.Range.End >= p.last.Start {
			h.bytes -= p.bytes()
			deleted := s.edits[0].Text
			prefix := max(0, a.Range.Start-b.Range.Start)
			suffix := max(0, b.Range.End-a.Range.End)
			// Own the composed inverse, including when only one fragment remains.
			var removed strings.Builder
			removed.Grow(prefix + len(a.Text) + suffix)
			removed.WriteString(deleted[:prefix])
			removed.WriteString(a.Text)
			removed.WriteString(deleted[len(deleted)-suffix:])
			combined := contract.TextEdit{
				Range: contract.Range{Start: min(a.Range.Start, b.Range.Start), End: max(a.Range.End, b.Range.End) + len(b.Text) - b.Range.Len()},
				Text:  removed.String(),
			}
			p.edits = []contract.TextEdit{combined}
			p.after, p.at, p.last = s.after, s.at, s.last
			h.bytes += p.bytes()
			merged = true
		}
	}
	if !merged {
		h.undo = append(h.undo, s)
		h.bytes += s.bytes()
	}
	h.open = eligible
	h.trim()
}

func (h *history) move(inverse historyStep, direction int) {
	from, to := &h.undo, &h.redo
	if direction > 0 {
		from, to = to, from
	}
	i := len(*from) - 1
	s := (*from)[i]
	h.bytes -= s.bytes()
	*from = slices.Delete(*from, i, i+1)
	s.edits = inverse.edits
	*to = append(*to, s)
	h.bytes += s.bytes()
	h.open = false
	h.trim()
}

func (h *history) trim() {
	for len(h.undo)+len(h.redo) > maxHistorySteps || h.bytes > maxHistoryBytes {
		steps := &h.undo
		if len(*steps) == 0 {
			steps = &h.redo
		}
		h.bytes -= (*steps)[0].bytes()
		*steps = slices.Delete(*steps, 0, 1)
	}
}
