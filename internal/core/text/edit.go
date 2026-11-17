package text

import (
	"sort"
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
	origLen := t.Len()
	size, end := origLen, 0
	for _, edit := range edits {
		r := edit.Range
		if r.Start < end || r.End < r.Start || r.End > origLen {
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

	if size <= maxChunk {
		var result strings.Builder
		result.Grow(size)
		changes := make([]contract.Change, 0, len(edits))
		end = 0
		for _, edit := range edits {
			result.WriteString(t.Slice(contract.Range{Start: end, End: edit.Range.Start}))
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
		result.WriteString(t.Slice(contract.Range{Start: end, End: origLen}))
		return New(result.String(), t.version+1), changes, nil
	}

	raw := make([]string, 0, len(t.chunks)+2*len(edits)+2)
	addPiece := func(s string) {
		for len(s) > maxChunk {
			raw = append(raw, s[:targetChunk])
			s = s[targetChunk:]
		}
		if len(s) > 0 {
			raw = append(raw, s)
		}
	}
	addUnchanged := func(from, to int) {
		if from >= to {
			return
		}
		i := sort.Search(len(t.chunks), func(i int) bool { return t.offs[i+1] > from })
		for ; i < len(t.chunks) && from < to; i++ {
			cStart, cEnd := t.offs[i], t.offs[i+1]
			if from == cStart && to >= cEnd {
				raw = append(raw, t.chunks[i])
				from = cEnd
				continue
			}
			subEnd := min(to, cEnd)
			addPiece(t.chunks[i][from-cStart : subEnd-cStart])
			from = subEnd
		}
	}

	changes := make([]contract.Change, 0, len(edits))
	end, newPos := 0, 0
	for _, edit := range edits {
		addUnchanged(end, edit.Range.Start)
		newPos += edit.Range.Start - end
		start := newPos
		addPiece(edit.Text)
		newPos += len(edit.Text)
		changes = append(changes, contract.Change{
			OldStart: edit.Range.Start,
			OldEnd:   edit.Range.End,
			NewStart: start,
			NewEnd:   newPos,
		})
		end = edit.Range.End
	}
	addUnchanged(end, origLen)

	// Coalesce adjacent small fragments produced around edit boundaries while
	// keeping untouched 16 KB chunks intact without copying their bytes.
	chunks := make([]string, 0, len(raw))
	for i := 0; i < len(raw); {
		if len(raw[i]) >= targetChunk/2 {
			chunks = append(chunks, raw[i])
			i++
			continue
		}
		j, sum := i+1, len(raw[i])
		for j < len(raw) && sum+len(raw[j]) <= maxChunk && (sum < targetChunk/2 || len(raw[j]) < targetChunk/2) {
			sum += len(raw[j])
			j++
		}
		if j == i+1 {
			chunks = append(chunks, raw[i])
		} else {
			var b strings.Builder
			b.Grow(sum)
			for k := i; k < j; k++ {
				b.WriteString(raw[k])
			}
			addMerged := b.String()
			for len(addMerged) > maxChunk {
				chunks = append(chunks, addMerged[:targetChunk])
				addMerged = addMerged[targetChunk:]
			}
			if len(addMerged) > 0 {
				chunks = append(chunks, addMerged)
			}
		}
		i = j
	}

	offs := make([]int, len(chunks)+1)
	for i, c := range chunks {
		offs[i+1] = offs[i] + len(c)
	}
	return &Text{chunks: chunks, offs: offs, version: t.version + 1}, changes, nil
}

func invalid(detail string) *contract.Error {
	return &contract.Error{Code: contract.CodeInvalid, Op: "text.Apply", Detail: detail}
}
