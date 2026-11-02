package export

import (
	"bufio"
	"bytes"
	"strconv"

	"aurink/internal/contract"
)

func (r *renderer) inlines(i int) {
	in := r.ast.Inlines(i)
	r.raw.inline = true
	r.inline(in, 0, len(in), &r.raw)
	r.raw.inline = false
}

func (r *renderer) inline(in []contract.Inline, from, end int, raw *rawHTML) {
	for i := from; i < end && r.err == nil; {
		n := in[i]
		next := n.Next
		if next <= i || next > end {
			next = i + 1
		}
		if n.Kind == contract.InlineHTML {
			raw.scan(n.Text)
			i = next
			continue
		}
		visible := !raw.dropped()
		markup := visible && !raw.literal()
		switch n.Kind {
		case contract.InlineText, contract.InlineEscape, contract.InlineEntity, contract.InlineEmoji:
			if visible {
				r.escaped(n.Text)
			}
		case contract.InlineSoftBreak:
			if visible {
				r.write("\n")
			}
		case contract.InlineLineBreak:
			if markup {
				r.write("<br />\n")
			} else if visible {
				r.write("\n")
			}
		case contract.InlineCode, contract.InlineMath:
			if markup {
				if n.Kind == contract.InlineMath {
					r.write("<code class=\"math\">")
				} else {
					r.write("<code>")
				}
			}
			if visible {
				r.escaped(n.Text)
			}
			if markup {
				r.write("</code>")
			}
		case contract.InlineEmphasis, contract.InlineStrong, contract.InlineStrike,
			contract.InlineHighlight, contract.InlineSup, contract.InlineSub:
			tag := map[contract.InlineKind]string{
				contract.InlineEmphasis: "em", contract.InlineStrong: "strong", contract.InlineStrike: "del",
				contract.InlineHighlight: "mark", contract.InlineSup: "sup", contract.InlineSub: "sub",
			}[n.Kind]
			if markup {
				r.write("<" + tag + ">")
				raw.beginContainer()
			}
			r.inline(in, i+1, next, raw)
			if markup {
				raw.endContainer()
				r.write("</" + tag + ">")
			}
		case contract.InlineLink, contract.InlineWikiLink:
			if markup {
				r.write("<a href=\"")
				r.escaped(safeURL(n.Dest, false))
				r.write("\"")
				if n.Title != "" {
					r.write(" title=\"")
					r.escaped(n.Title)
					r.write("\"")
				}
				r.write(">")
				raw.beginContainer()
			}
			r.inline(in, i+1, next, raw)
			if markup {
				raw.endContainer()
				r.write("</a>")
			}
		case contract.InlineImage:
			if markup {
				r.write("<img src=\"")
				r.escaped(r.imageURL(n.Dest))
				r.write("\" alt=\"")
				r.escaped(plain(in[i+1 : next]))
				r.write("\"")
				if n.Title != "" {
					r.write(" title=\"")
					r.escaped(n.Title)
					r.write("\"")
				}
				r.write(" />")
			} else if visible {
				r.escaped(plain(in[i+1 : next]))
			}
		case contract.InlineFootnoteRef:
			if markup {
				r.noteRef(n.Text)
			} else if visible {
				r.escaped("[^" + n.Text + "]")
			}
		}
		i = next
	}
}

func (r *renderer) noteRef(label string) {
	key := normalizeLabel(label)
	n := r.noteIndex[key]
	if n == nil {
		index, ok := r.definitions[key]
		if !ok {
			r.escaped("[^" + label + "]")
			return
		}
		base := slug(key)
		id := base
		for suffix := 1; r.usedNoteIDs[id]; suffix++ {
			id = base + "-" + strconv.Itoa(suffix)
		}
		r.usedNoteIDs[id] = true
		n = &note{id: id, block: index, number: len(r.notes) + 1}
		r.notes = append(r.notes, n)
		r.noteIndex[key] = n
	}
	n.refs++
	refID := "fnref-" + n.id
	if n.refs > 1 {
		refID += "-" + strconv.Itoa(n.refs)
	}
	r.write("<sup class=\"footnote-ref\"><a href=\"#fn-")
	r.escaped(n.id)
	r.write("\" id=\"")
	r.escaped(refID)
	r.write("\">")
	r.write(strconv.Itoa(n.number))
	r.write("</a></sup>")
}

func (r *renderer) footnotes() {
	if len(r.notes) == 0 {
		return
	}
	// Collect all note bodies before emitting backlinks: a later note can
	// refer back to an earlier one, and can introduce further definitions.
	out := r.out
	for i := 0; i < len(r.notes) && r.err == nil; i++ {
		var body bytes.Buffer
		r.out = bufio.NewWriter(&body)
		r.blocks(r.notes[i].block)
		r.raw.close()
		if r.err == nil {
			r.err = r.out.Flush()
		}
		r.notes[i].content = body.String()
	}
	r.out = out
	r.write("<section class=\"footnotes\"><ol>\n")
	for i := 0; i < len(r.notes) && r.err == nil; i++ {
		n := r.notes[i]
		r.write("<li id=\"fn-")
		r.escaped(n.id)
		r.write("\">\n")
		r.write(n.content)
		for ref := 1; ref <= n.refs; ref++ {
			r.write("<a class=\"footnote-backref\" href=\"#fnref-")
			r.escaped(n.id)
			if ref > 1 {
				r.write("-" + strconv.Itoa(ref))
			}
			r.write("\">↩</a>")
		}
		r.write("</li>\n")
	}
	r.write("</ol></section>\n")
}
