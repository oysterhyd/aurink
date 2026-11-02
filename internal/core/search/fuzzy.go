package search

import (
	"sort"
	"unicode"
	"unicode/utf8"

	"aurink/internal/contract"
)

// Fuzzy ranks subsequence matches for paths and command names.
type Fuzzy struct{}

var _ contract.Fuzzy = (*Fuzzy)(nil)

// NewFuzzy returns a matcher safe for concurrent use.
func NewFuzzy() *Fuzzy { return &Fuzzy{} }

// Match returns the highest-scoring positions in each item's Text.
func (f *Fuzzy) Match(query string, items []contract.FuzzyItem, limit int) []contract.FuzzyHit {
	var wanted []rune
	for _, r := range query {
		if !unicode.IsSpace(r) {
			wanted = append(wanted, foldRune(r))
		}
	}
	if len(wanted) == 0 {
		n := len(items)
		if limit > 0 && limit < n {
			n = limit
		}
		hits := make([]contract.FuzzyHit, n)
		for i := range hits {
			hits[i].Index = i
		}
		return hits
	}

	var hits []contract.FuzzyHit
	var chars, keys []rune
	var previous, current, parents []int
	for index, item := range items {
		chars, keys = chars[:0], keys[:0]
		filename := 0
		matched := 0
		for _, r := range item.Text {
			if r == '/' || r == '\\' {
				filename = len(chars) + 1
			}
			chars = append(chars, r)
			key := foldRune(r)
			keys = append(keys, key)
			if matched < len(wanted) && key == wanted[matched] {
				matched++
			}
		}
		if matched != len(wanted) {
			continue
		}
		n := len(chars)
		if cap(previous) < n {
			previous, current = make([]int, n), make([]int, n)
		} else {
			previous, current = previous[:n], current[:n]
		}
		if cap(parents) < len(wanted)*n {
			parents = make([]int, len(wanted)*n)
		} else {
			parents = parents[:len(wanted)*n]
		}
		const unreachable = -1 << 30
		for k, want := range wanted {
			best, bestPos := unreachable, -1
			for j := range chars {
				current[j] = unreachable
				if k > 0 && j > 0 && previous[j-1] != unreachable {
					if score := previous[j-1] + j - 1; score > best {
						best, bestPos = score, j-1
					}
				}
				if keys[j] != want {
					continue
				}
				bonus := 16
				if j >= filename {
					bonus += 4
				}
				if j == 0 || (!unicode.IsLetter(chars[j-1]) && !unicode.IsDigit(chars[j-1])) || (unicode.IsLower(chars[j-1]) && unicode.IsUpper(chars[j])) {
					bonus += 10
				}
				if k == 0 {
					current[j] = bonus
					parents[k*n+j] = -1
				} else if bestPos >= 0 {
					current[j] = best - (j - 1) + bonus
					parents[k*n+j] = bestPos
					if j > 0 && previous[j-1] != unreachable {
						if contiguous := previous[j-1] + bonus + 16; contiguous > current[j] {
							current[j] = contiguous
							parents[k*n+j] = j - 1
						}
					}
				}
			}
			previous, current = current, previous
		}
		best, end := unreachable, -1
		for j, score := range previous {
			if score > best {
				best, end = score, j
			}
		}
		positions := make([]int, len(wanted))
		for k := len(wanted) - 1; k >= 0; k-- {
			positions[k] = end
			end = parents[k*n+end]
		}
		hits = append(hits, contract.FuzzyHit{Index: index, Score: best, Positions: positions})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].Index < hits[j].Index
	})
	if limit > 0 && limit < len(hits) {
		hits = hits[:limit]
	}
	return hits
}

func foldRune(r rune) rune {
	if r < utf8.RuneSelf {
		if 'a' <= r && r <= 'z' {
			return r - ('a' - 'A')
		}
		return r
	}
	first := r
	for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
		if next < first {
			first = next
		}
	}
	return first
}
