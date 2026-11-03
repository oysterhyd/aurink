package markdown

import (
	"strings"
	"unicode"

	"aurink/internal/contract"
	"github.com/egoist/mygo/ui/markdown/parser"
)

func (s *Snapshot) derive() {
	s.derivedOnce.Do(func() {
		s.notes = map[string]int{}
		s.defs = map[string]contract.LinkDef{}
		slugs := map[string]bool{}
		for i := range s.entries {
			b := s.Block(i)
			switch b.Kind {
			case contract.BlockHeading:
				var title strings.Builder
				for _, n := range s.Inlines(i) {
					switch n.Kind {
					case contract.InlineText, contract.InlineCode, contract.InlineEscape, contract.InlineEntity, contract.InlineMath, contract.InlineEmoji:
						title.WriteString(n.Text)
					case contract.InlineSoftBreak, contract.InlineLineBreak:
						title.WriteByte(' ')
					}
				}
				t := title.String()
				base := slug(t)
				key := base
				for n := 1; slugs[key]; n++ {
					key = base + "-" + decimal(n)
				}
				slugs[key] = true
				s.outline = append(s.outline, contract.Heading{Block: b.ID, Index: i, Level: b.Level, Text: t, Slug: key, Range: b.Range})
			case contract.BlockFootnoteDef:
				key := normalizeLabel(b.Label)
				if _, ok := s.notes[key]; !ok {
					s.notes[key] = i
				}
			case contract.BlockLinkDef:
				s.readDefinitions(b.Range)
			}
		}
	})
}

func decimal(n int) string {
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte(n%10) + '0'
		n /= 10
	}
	return string(b[i:])
}
func slug(s string) string {
	var out strings.Builder
	// GitHub keeps letters, marks, decimal/letter numbers and connector
	// punctuation. Symbols (including emoji) disappear; ASCII spaces become '-'.
	s = strings.ReplaceAll(s, "İ", "i\u0307")
	for _, r := range strings.ToLower(s) {
		if r == ' ' {
			out.WriteByte('-')
		} else if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) || unicode.In(r, unicode.Nl, unicode.Pc) || r == '-' {
			out.WriteRune(r)
		}
	}
	return out.String()
}

// Resolve candidate labels through the parser itself. Prefixing a probe with a
// blank separator preserves the document's actual definition context, including
// definitions in containers, multiline titles, and first-definition precedence.
func (s *Snapshot) readDefinitions(r contract.Range) {
	src := s.text.String()
	for at := r.Start; at < r.End; at++ {
		if src[at] != '[' {
			continue
		}
		end := at + 1
		for end < len(src) && end-at <= 1000 {
			if src[end] == '\\' && end+1 < len(src) {
				end += 2
				continue
			}
			if src[end] == ']' {
				break
			}
			end++
		}
		if end+1 >= len(src) || src[end] != ']' || src[end+1] != ':' {
			continue
		}
		label := cleanDefinitionLabel(src[at+1 : end])
		key := normalizeLabel(label)
		if _, ok := s.defs[key]; ok {
			continue
		}
		d := parser.New(parser.Default)
		d.Set("[aurink][" + label + "]\n\n" + src[s.bodyAt:])
		if d.Len() < 2 {
			continue
		}
		for _, n := range d.Inlines(1) {
			if n.Kind == parser.Link {
				finish := r.End
				for i := range s.entries {
					b := s.Block(i)
					if b.Kind == contract.BlockLinkDef && b.Range.Contains(end) {
						finish = max(finish, b.Range.End)
					}
				}
				finish = s.definitionEnd(label, n.Dest, n.Title, at, finish)
				s.defs[key] = contract.LinkDef{Label: label, Dest: n.Dest, Title: n.Title, Range: contract.Range{Start: at, End: finish}}
				break
			}
		}
	}
}

func (s *Snapshot) definitionEnd(label, dest, title string, at, limit int) int {
	src := s.text.String()
	for start := at; start <= limit; {
		end, next := lineEnd(src, start)
		d := parser.New(parser.Default)
		d.Set("[aurink][" + label + "]\n\n" + src[s.bodyAt:end])
		if d.Len() > 1 {
			for _, n := range d.Inlines(1) {
				if n.Kind == parser.Link && n.Dest == dest && n.Title == title {
					return end
				}
			}
		}
		if next <= start || next > limit {
			break
		}
		start = next
	}
	return limit
}

func cleanDefinitionLabel(label string) string {
	var out strings.Builder
	for start := 0; start < len(label); {
		end, next := lineEnd(label, start)
		at := start
		if start > 0 {
			for at < end && (label[at] == ' ' || label[at] == '\t' || label[at] == '>') {
				at++
			}
		}
		out.WriteString(label[at:end])
		if next > end {
			out.WriteByte('\n')
		}
		start = next
	}
	return out.String()
}

// Outline returns headings with GitHub-style unique slugs, in source order.
func (s *Snapshot) Outline() []contract.Heading { s.derive(); return s.outline }

// Footnotes returns normalized labels and their first definition's block index.
func (s *Snapshot) Footnotes() map[string]int {
	s.derive()
	out := make(map[string]int, len(s.notes))
	for k, v := range s.notes {
		out[k] = v
	}
	return out
}

// LinkDefs returns normalized reference labels and their first definitions.
func (s *Snapshot) LinkDefs() map[string]contract.LinkDef {
	s.derive()
	out := make(map[string]contract.LinkDef, len(s.defs))
	for k, v := range s.defs {
		out[k] = v
	}
	return out
}
