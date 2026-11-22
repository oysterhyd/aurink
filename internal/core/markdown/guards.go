package markdown

import (
	"aurink/internal/contract"
	"aurink/internal/core/text"
	"github.com/egoist/mygo/ui/markdown/parser"
)

func touchesAmbiguousCaret(t *text.Text, cs []contract.Change) bool {
	n := t.Len()
	for _, c := range cs {
		lo := max(0, c.NewStart-1005)
		hi := min(n, c.NewEnd+1005)
		if ambiguousCaretLabel(t.Slice(contract.Range{Start: lo, End: hi})) {
			return true
		}
	}
	return false
}

func ambiguousCaretLabel(src string) bool {
	for i := 0; i < len(src); i++ {
		if src[i] != '[' {
			continue
		}
		j := i + 1
		for j < len(src) && (src[j] == ' ' || src[j] == '\t' || src[j] == '\n' || src[j] == '\r') {
			j++
		}
		if j < len(src) && src[j] == '^' {
			space := j > i+1
			for k := j + 1; k < len(src) && k-i <= 1000 && src[k] != '['; k++ {
				if src[k] == ']' {
					if space {
						return true
					}
					break
				}
				space = space || src[k] == ' ' || src[k] == '\t' || src[k] == '\n' || src[k] == '\r'
			}
		}
	}
	return false
}

func adjacentIndentedCode(d *parser.Document) bool {
	previous := map[int]int{}
	for i := 1; i < d.Len(); i++ {
		b := d.Block(i)
		if prev, ok := previous[b.Parent]; ok && b.Kind == parser.CodeBlock && !b.Fenced {
			p := d.Block(prev)
			if p.Kind == parser.CodeBlock && !p.Fenced {
				return true
			}
		}
		previous[b.Parent] = i
	}
	return false
}
