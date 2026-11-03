package markdown

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"aurink/internal/contract"
)

// extensionNode is used only for leaves containing candidate syntax. The
// parser's preorder representation is restored after matching sibling tokens.
type extensionNode struct {
	n        contract.Inline
	children []extensionNode
	marker   byte
	origin   int
}

func extendInlines(src string, in []contract.Inline, exts contract.Extensions) []contract.Inline {
	active := exts & (contract.ExtHighlight | contract.ExtSupSub | contract.ExtTildeSub | contract.ExtEmoji | contract.ExtWikiLink)
	if active == 0 || len(in) == 0 {
		return in
	}
	found := false
	for _, n := range in {
		if active&contract.ExtTildeSub != 0 && n.Kind == contract.InlineStrike && n.Open.Len() == 1 {
			found = true
			break
		}
		if n.Kind != contract.InlineText {
			continue
		}
		raw := src[n.Range.Start:n.Range.End]
		if active&contract.ExtHighlight != 0 && strings.Contains(raw, "==") ||
			active&contract.ExtSupSub != 0 && strings.Contains(raw, "^") ||
			active&contract.ExtTildeSub != 0 && strings.Contains(raw, "~") ||
			active&contract.ExtEmoji != 0 && strings.Count(raw, ":") >= 2 ||
			active&contract.ExtWikiLink != 0 && (strings.Contains(raw, "[[") || strings.HasSuffix(raw, "[")) {
			found = true
			break
		}
	}
	if !found {
		return in
	}
	var tree func(int, int) []extensionNode
	tree = func(start, end int) []extensionNode {
		var nodes []extensionNode
		for i := start; i < end; i = in[i].Next {
			n := in[i]
			x := extensionNode{n: n}
			if n.Next > i+1 {
				if n.Auto || n.Kind == contract.InlineCode || n.Kind == contract.InlineMath || n.Kind == contract.InlineHTML {
					// Preserve opaque subtrees exactly.
					var opaque func(int, int) []extensionNode
					opaque = func(a, b int) []extensionNode {
						var out []extensionNode
						for j := a; j < b; j = in[j].Next {
							out = append(out, extensionNode{n: in[j], children: opaque(j+1, in[j].Next)})
						}
						return out
					}
					x.children = opaque(i+1, n.Next)
				} else {
					x.children = tree(i+1, n.Next)
				}
			}
			if active&contract.ExtTildeSub != 0 && n.Kind == contract.InlineStrike && n.Open.Len() == 1 {
				nodes = append(nodes, extensionNode{n: contract.Inline{Kind: contract.InlineText, Range: n.Open, Text: src[n.Open.Start:n.Open.End]}, marker: '~'})
				nodes = append(nodes, x.children...)
				nodes = append(nodes, extensionNode{n: contract.Inline{Kind: contract.InlineText, Range: n.Close, Text: src[n.Close.Start:n.Close.End]}, marker: '~'})
			} else if n.Kind == contract.InlineText {
				nodes = append(nodes, extensionTokens(src, n, active, i+1)...)
			} else {
				nodes = append(nodes, x)
			}
		}
		if active&contract.ExtWikiLink != 0 {
			nodes = referenceWikiLinks(src, nodes)
			nodes = siblingWikiLinks(src, nodes)
		}
		return pairExtensions(src, nodes)
	}
	t := mergeExtensionText(tree(0, len(in)))
	out := make([]contract.Inline, 0, len(in))
	var flatten func([]extensionNode)
	flatten = func(nodes []extensionNode) {
		for _, x := range nodes {
			j := len(out)
			out = append(out, x.n)
			flatten(x.children)
			out[j].Next = len(out)
		}
	}
	flatten(t)
	return out
}

func mergeExtensionText(nodes []extensionNode) []extensionNode {
	out := nodes[:0]
	for _, x := range nodes {
		x.children = mergeExtensionText(x.children)
		if len(out) > 0 && x.origin != 0 && x.origin == out[len(out)-1].origin &&
			x.n.Kind == contract.InlineText && out[len(out)-1].n.Kind == contract.InlineText && out[len(out)-1].n.Range.End == x.n.Range.Start {
			prev := &out[len(out)-1]
			prev.n.Range.End = x.n.Range.End
			prev.n.Text += x.n.Text
		} else {
			out = append(out, x)
		}
	}
	return out
}

// Wiki delimiters can surround other inline nodes, such as an emphasized
// alias. Only literal text nodes supply delimiters; code and escapes cannot.
func siblingWikiLinks(src string, nodes []extensionNode) []extensionNode {
	var out []extensionNode
	for i := 0; i < len(nodes); i++ {
		first := nodes[i]
		if first.n.Kind != contract.InlineText || first.marker != 0 {
			out = append(out, first)
			continue
		}
		openOffset := strings.Index(src[first.n.Range.Start:first.n.Range.End], "[[")
		if openOffset < 0 {
			out = append(out, first)
			continue
		}
		open := first.n.Range.Start + openOffset
		matched := false
		for j := i + 1; j < len(nodes); j++ {
			last := nodes[j]
			if strings.ContainsAny(src[open:last.n.Range.Start], "\r\n") {
				break
			}
			if last.n.Kind != contract.InlineText || last.marker != 0 {
				continue
			}
			endOffset := strings.Index(src[last.n.Range.Start:last.n.Range.End], "]]")
			if endOffset < 0 {
				continue
			}
			close := last.n.Range.Start + endOffset
			inside := src[open+2 : close]
			if inside == "" || strings.ContainsAny(inside, "\r\n") {
				break
			}
			dest, labelStart := inside, open+2
			if k := strings.IndexByte(inside, '|'); k >= 0 {
				dest, labelStart = inside[:k], open+3+k
			}
			if dest == "" {
				break
			}
			children, ok := clipWikiNodes(src, nodes[i:j+1], labelStart, close)
			if !ok {
				break
			}
			// An alias target may contain only literal text, so Open never
			// cuts through a parser node or hides another inline's syntax.
			if labelStart != open+2 {
				for _, x := range nodes[i : j+1] {
					if x.n.Range.Start >= labelStart {
						break
					}
					if x.n.Range.End > open+2 && x.n.Kind != contract.InlineText {
						ok = false
						break
					}
				}
				if !ok {
					break
				}
			}
			if first.n.Range.Start < open {
				prefix := first.n
				prefix.Range.End, prefix.Text = open, src[prefix.Range.Start:open]
				out = append(out, extensionNode{n: prefix})
			}
			x := extensionNode{n: contract.Inline{Kind: contract.InlineWikiLink, Range: contract.Range{Start: open, End: close + 2}, Open: contract.Range{Start: open, End: labelStart}, Close: contract.Range{Start: close, End: close + 2}, Dest: dest}, children: pairExtensions(src, children)}
			out = append(out, x)
			if close+2 < last.n.Range.End {
				suffix := last.n
				suffix.Range.Start = close + 2
				suffix.Text = src[suffix.Range.Start:suffix.Range.End]
				out = append(out, extensionNode{n: suffix})
			}
			i, matched = j, true
			break
		}
		if !matched {
			out = append(out, first)
		}
	}
	return out
}

func clipWikiNodes(src string, nodes []extensionNode, start, end int) ([]extensionNode, bool) {
	var out []extensionNode
	for _, x := range nodes {
		if x.n.Range.End <= start || x.n.Range.Start >= end {
			continue
		}
		if x.n.Range.Start < start || x.n.Range.End > end {
			if x.n.Kind != contract.InlineText || x.marker != 0 {
				return nil, false
			}
			x.n.Range.Start = max(start, x.n.Range.Start)
			x.n.Range.End = min(end, x.n.Range.End)
			x.n.Text = src[x.n.Range.Start:x.n.Range.End]
		}
		out = append(out, x)
	}
	return out, true
}

// A reference definition can make the underlying parser consume the inner
// brackets of [[page]] as a shortcut link. Restore the wiki construct only
// when the neighboring literal brackets and shortcut link are contiguous.
func referenceWikiLinks(src string, nodes []extensionNode) []extensionNode {
	var out []extensionNode
	for i := 0; i < len(nodes); i++ {
		if i+2 < len(nodes) {
			left, link, right := nodes[i], nodes[i+1], nodes[i+2]
			l, m, r := left.n, link.n, right.n
			if left.marker == 0 && right.marker == 0 && l.Kind == contract.InlineText && r.Kind == contract.InlineText &&
				m.Kind == contract.InlineLink && !m.Auto && m.Open.Len() == 1 && m.Close.Len() == 1 &&
				l.Range.End == m.Range.Start && m.Range.End == r.Range.Start && l.Range.Len() > 0 && r.Range.Len() > 0 &&
				src[l.Range.End-1] == '[' && src[r.Range.Start] == ']' {
				inside := src[m.Open.End:m.Close.Start]
				dest := inside
				labelStart := m.Open.End
				if k := strings.IndexByte(inside, '|'); k >= 0 {
					dest, labelStart = inside[:k], m.Open.End+k+1
				}
				if dest != "" && !strings.ContainsAny(inside, "\r\n\\[]") {
					wikiStart := l.Range.End - 1
					if l.Range.Len() > 1 {
						l.Range.End--
						l.Text = src[l.Range.Start:l.Range.End]
						out = append(out, extensionNode{n: l})
					}
					close := contract.Range{Start: m.Close.Start, End: r.Range.Start + 1}
					x := extensionNode{n: contract.Inline{Kind: contract.InlineWikiLink, Range: contract.Range{Start: wikiStart, End: close.End}, Open: contract.Range{Start: wikiStart, End: labelStart}, Close: close, Dest: dest}}
					if labelStart < close.Start {
						x.children = []extensionNode{{n: contract.Inline{Kind: contract.InlineText, Range: contract.Range{Start: labelStart, End: close.Start}, Text: src[labelStart:close.Start]}}}
					}
					out = append(out, x)
					if r.Range.Len() > 1 {
						r.Range.Start++
						if len(r.Text) > 0 {
							r.Text = r.Text[1:]
						}
						out = append(out, extensionNode{n: r})
					}
					i += 2
					continue
				}
			}
		}
		out = append(out, nodes[i])
	}
	return out
}

func extensionTokens(src string, n contract.Inline, exts contract.Extensions, origin int) []extensionNode {
	var out []extensionNode
	start, end := n.Range.Start, n.Range.End
	plain := start
	noWikiClose := false
	addText := func(a, b int) {
		if a >= b {
			return
		}
		text := src[a:b]
		if b == end && len(n.Text) < end-start {
			text = strings.TrimRight(text, " ")
		}
		out = append(out, extensionNode{n: contract.Inline{Kind: contract.InlineText, Range: contract.Range{Start: a, End: b}, Text: text}, origin: origin})
	}
	for i := start; i < end; {
		length := 0
		var x extensionNode
		switch {
		case exts&contract.ExtWikiLink != 0 && !noWikiClose && strings.HasPrefix(src[i:end], "[["):
			if j := strings.Index(src[i+2:end], "]]"); j >= 0 {
				close := i + 2 + j
				inside := src[i+2 : close]
				dest, label := inside, inside
				labelStart := i + 2
				if k := strings.IndexByte(inside, '|'); k >= 0 {
					dest, label, labelStart = inside[:k], inside[k+1:], i+3+k
				}
				if dest != "" && !strings.ContainsAny(inside, "\r\n") {
					length = close + 2 - i
					x.n = contract.Inline{Kind: contract.InlineWikiLink, Range: contract.Range{Start: i, End: i + length}, Open: contract.Range{Start: i, End: labelStart}, Close: contract.Range{Start: close, End: close + 2}, Dest: dest}
					if label != "" {
						x.children = []extensionNode{{n: contract.Inline{Kind: contract.InlineText, Range: contract.Range{Start: labelStart, End: close}, Text: label}}}
					}
				}
			} else {
				noWikiClose = true
			}
		case exts&contract.ExtEmoji != 0 && src[i] == ':':
			if j := strings.IndexByte(src[i+1:end], ':'); j >= 0 {
				if emoji, ok := emojiShortcodes[src[i+1:i+1+j]]; ok {
					length = j + 2
					x.n = contract.Inline{Kind: contract.InlineEmoji, Range: contract.Range{Start: i, End: i + length}, Text: emoji}
				}
			}
		case exts&contract.ExtHighlight != 0 && strings.HasPrefix(src[i:end], "=="):
			length, x.marker = 2, '='
		case exts&contract.ExtSupSub != 0 && src[i] == '^':
			length, x.marker = 1, '^'
		case exts&contract.ExtTildeSub != 0 && src[i] == '~' && (i == start || src[i-1] != '~') && (i+1 == end || src[i+1] != '~'):
			length, x.marker = 1, '~'
		}
		if length == 0 {
			i++
			continue
		}
		addText(plain, i)
		if x.marker != 0 {
			x.n = contract.Inline{Kind: contract.InlineText, Range: contract.Range{Start: i, End: i + length}, Text: src[i : i+length]}
			x.origin = origin
		}
		out = append(out, x)
		i += length
		plain = i
	}
	addText(plain, end)
	return out
}

func pairExtensions(src string, nodes []extensionNode) []extensionNode {
	pairs := make([]int, len(nodes))
	for i := range pairs {
		pairs[i] = -1
	}
	var stack []int
	for i, x := range nodes {
		if x.marker == 0 {
			continue
		}
		closed := false
		for k := len(stack) - 1; k >= 0; k-- {
			j := stack[k]
			if nodes[j].marker == x.marker {
				a, b := nodes[j].n.Range.End, x.n.Range.Start
				if a < b && !spaceAt(src, a) && !spaceBefore(src, b) && (x.marker == '=' || strings.IndexFunc(src[a:b], unicode.IsSpace) < 0) {
					pairs[j] = i
					stack = stack[:k]
					closed = true
					break
				}
				// An earlier opener contains the same intervening whitespace.
				if a < b && (spaceBefore(src, b) || x.marker != '=' && strings.IndexFunc(src[a:b], unicode.IsSpace) >= 0) {
					break
				}
			}
		}
		if closed {
			continue
		}
		if x.n.Range.End < len(src) && !spaceAt(src, x.n.Range.End) {
			stack = append(stack, i)
		}
	}
	var group func(int, int) []extensionNode
	group = func(a, b int) []extensionNode {
		var out []extensionNode
		for i := a; i < b; i++ {
			j := pairs[i]
			if j < 0 || j >= b {
				out = append(out, nodes[i])
				continue
			}
			kind := contract.InlineHighlight
			if nodes[i].marker == '^' {
				kind = contract.InlineSup
			}
			if nodes[i].marker == '~' {
				kind = contract.InlineSub
			}
			open, close := nodes[i].n.Range, nodes[j].n.Range
			out = append(out, extensionNode{n: contract.Inline{Kind: kind, Range: contract.Range{Start: open.Start, End: close.End}, Open: open, Close: close}, children: group(i+1, j)})
			i = j
		}
		return out
	}
	return group(0, len(nodes))
}

func spaceAt(s string, at int) bool {
	r, _ := utf8.DecodeRuneInString(s[at:])
	return unicode.IsSpace(r)
}
func spaceBefore(s string, at int) bool {
	r, _ := utf8.DecodeLastRuneInString(s[:at])
	return unicode.IsSpace(r)
}
