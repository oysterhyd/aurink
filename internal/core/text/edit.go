package text

import (
	"strings"

	"aurink/internal/contract"
)

// Apply returns a new snapshot at Version()+1 and one Change per edit. Ranges
// refer to this snapshot and must be ordered and non-overlapping. Adjacent
// ranges and repeated insertions at the same offset are applied in slice order.
// Empty edits return this snapshot and no changes. Invalid edits or an exhausted
// version return a *contract.Error with CodeInvalid and no snapshot or changes.
// Apply leaves this snapshot and the edits unchanged and is safe for concurrent use.
func (t *Text) Apply(edits []contract.TextEdit) (*Text, []contract.Change, error) {
	if len(edits) == 0 {
		return t, nil, nil
	}
	size, end := len(t.s), 0
	for _, edit := range edits {
		r := edit.Range
		if r.Start < end || r.End < r.Start || r.End > len(t.s) {
			return nil, nil, invalid("edit ranges must be in bounds, ordered, and non-overlapping")
		}
		size -= r.Len()
		if len(edit.Text) > int(^uint(0)>>1)-size {
			return nil, nil, invalid("resulting text is too large")
		}
		size += len(edit.Text)
		end = r.End
	}
	if t.version == ^contract.Version(0) {
		return nil, nil, invalid("version is exhausted")
	}

	var result strings.Builder
	result.Grow(size)
	changes := make([]contract.Change, 0, len(edits))
	end = 0
	for _, edit := range edits {
		result.WriteString(t.s[end:edit.Range.Start])
		start := result.Len()
		result.WriteString(edit.Text)
		changes = append(changes, contract.Change{
			OldStart: edit.Range.Start,
			OldEnd:   edit.Range.End,
			NewStart: start,
			NewEnd:   result.Len(),
		})
		end = edit.Range.End
	}
	result.WriteString(t.s[end:])
	return New(result.String(), t.version+1), changes, nil
}

func invalid(detail string) *contract.Error {
	return &contract.Error{Code: contract.CodeInvalid, Op: "text.Apply", Detail: detail}
}
