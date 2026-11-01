package markdown

import (
	"aurink/internal/contract"
	"github.com/egoist/mygo/ui/markdown/parser"
)

func punctuation(c byte) bool {
	return c >= '!' && c <= '/' || c >= ':' && c <= '@' || c >= '[' && c <= '`' || c >= '{' && c <= '~'
}

// scanText runs only on parser.Text. Code, HTML, math and destinations are
// intentionally opaque. Entity uses the pinned parser's own CommonMark rules.
func scanText(src string, r contract.Range) []contract.Inline {
	var out []contract.Inline
	plain := r.Start
	add := func(kind contract.InlineKind, start, end int, text string) {
		if plain < start {
			out = append(out, contract.Inline{Kind: contract.InlineText, Range: contract.Range{Start: plain, End: start}, Text: src[plain:start]})
		}
		n := contract.Inline{Kind: kind, Range: contract.Range{Start: start, End: end}, Text: text}
		if kind == contract.InlineEscape {
			n.Open = contract.Range{Start: start, End: start + 1}
		}
		out = append(out, n)
		plain = end
	}
	for i := r.Start; i < r.End; {
		switch {
		case src[i] == '\\' && i+1 < r.End && punctuation(src[i+1]):
			add(contract.InlineEscape, i, i+2, src[i+1:i+2])
			i += 2
		case src[i] == '&':
			n, text := parser.Entity(src[i:r.End])
			if n > 0 {
				add(contract.InlineEntity, i, i+n, text)
				i += n
			} else {
				i++
			}
		default:
			i++
		}
	}
	if plain < r.End {
		out = append(out, contract.Inline{Kind: contract.InlineText, Range: contract.Range{Start: plain, End: r.End}, Text: src[plain:r.End]})
	}
	return out
}
