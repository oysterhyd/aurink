package astspike

import (
	"fmt"
	"strings"

	"aurink.local/spikes/contract-rc/contract"
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

func (s *snapshot) inlines(index int) ([]contract.Inline, error) {
	shift := 0
	if s.bodyAt > 0 {
		shift = 1
	}
	i := index - shift
	if i <= 0 {
		return nil, nil
	}
	in := s.doc.Inlines(i)
	var out []contract.Inline
	var walk func(int, int, bool) error
	walk = func(from, end int, literal bool) error {
		for k := from; k < end; k = in[k].Next {
			n := in[k]
			r := contract.Range{Start: n.Start + s.bodyAt, End: n.End + s.bodyAt}
			if n.Kind == parser.Text && !literal {
				parts := scanText(s.source, r)
				var text strings.Builder
				for _, p := range parts {
					text.WriteString(p.Text)
				}
				// Joining adjacent Text nodes can cover trailing spaces that a
				// hard break discarded. They are source, but not literal content.
				decoded := text.String()
				if decoded != n.Text {
					if strings.TrimRight(decoded, " ") != n.Text {
						return fmt.Errorf("text scan at %+v: %q != parser %q", r, decoded, n.Text)
					}
					// Trim only the final literal span, retaining exact source range.
					last := &parts[len(parts)-1]
					last.Text = strings.TrimRight(last.Text, " ")
				}
				for _, p := range parts {
					p.Next = len(out) + 1
					out = append(out, p)
				}
				continue
			}
			j := len(out)
			out = append(out, contract.Inline{Kind: contract.InlineKind(n.Kind), Range: r,
				Text: n.Text, Dest: n.Dest, Title: n.Title, Auto: n.Auto, Display: n.Display})
			if err := walk(k+1, n.Next, literal || n.Auto); err != nil {
				return err
			}
			out[j].Next = len(out)
		}
		return nil
	}
	if err := walk(0, len(in), false); err != nil {
		return nil, err
	}
	return out, nil
}
