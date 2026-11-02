package richpaste

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/html"

	"aurink/internal/contract"
)

// Converter converts clipboard HTML and RTF into safe Markdown.
type Converter struct{}

var _ contract.PasteConverter = Converter{}

// New returns a stateless clipboard converter.
func New() *Converter { return &Converter{} }

type marks struct{ bold, italic, strike bool }

type run struct {
	text  string
	marks marks
	raw   bool
}

type inlineWriter struct {
	runs        []run
	buf         strings.Builder
	current     marks
	raw, active bool
}

func (w *inlineWriter) flush() {
	if w.active {
		w.runs = append(w.runs, run{text: w.buf.String(), marks: w.current, raw: w.raw})
		w.buf = strings.Builder{}
		w.active = false
	}
}

func (w *inlineWriter) add(s string, m marks, raw bool) {
	if s == "" {
		return
	}
	if w.active && (w.current != m || w.raw != raw) {
		w.flush()
	}
	if !w.active {
		w.current, w.raw, w.active = m, raw, true
	}
	w.buf.WriteString(s)
}

func (w *inlineWriter) markdown() string {
	w.flush()
	var b strings.Builder
	for _, r := range w.runs {
		s := r.text
		if !r.raw {
			s = escapeText(s)
		}
		lead := len(s) - len(strings.TrimLeft(s, " \t\n"))
		tail := len(s) - len(strings.TrimRight(s, " \t\n"))
		if lead+tail > len(s) {
			tail = len(s) - lead
		}
		b.WriteString(s[:lead])
		body := s[lead : len(s)-tail]
		if body != "" {
			if r.marks.strike {
				body = "~~" + body + "~~"
			}
			if r.marks.italic {
				body = "*" + body + "*"
			}
			if r.marks.bold {
				body = "**" + body + "**"
			}
			b.WriteString(body)
		}
		b.WriteString(s[len(s)-tail:])
	}
	return b.String()
}

func escapeText(s string) string {
	var b strings.Builder
	lineStart := true
	for i, r := range s {
		if strings.ContainsRune(`\*_[]~`+"`<>!", r) || lineStart && strings.ContainsRune("#>+-", r) || r == '.' && numericLinePrefix(s, i) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
		if r == '\n' {
			lineStart = true
		} else if !unicode.IsSpace(r) {
			lineStart = false
		}
	}
	return b.String()
}

func numericLinePrefix(s string, at int) bool {
	start := strings.LastIndexByte(s[:at], '\n') + 1
	p := strings.TrimSpace(s[start:at])
	if p == "" {
		return false
	}
	for _, r := range p {
		if r < '0' || r > '9' {
			return false
		}
	}
	return at+1 < len(s) && (s[at+1] == ' ' || s[at+1] == '\t')
}

func attr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, name) {
			return a.Val
		}
	}
	return ""
}

func css(n *html.Node) map[string]string {
	style := attr(n, "style")
	if style == "" {
		return nil
	}
	m := map[string]string{}
	for _, item := range strings.Split(style, ";") {
		if k, v, ok := strings.Cut(item, ":"); ok {
			m[strings.ToLower(strings.TrimSpace(k))] = strings.ToLower(strings.TrimSpace(v))
		}
	}
	return m
}

func styled(n *html.Node, m marks) marks {
	switch n.Data {
	case "b", "strong":
		m.bold = true
	case "i", "em":
		m.italic = true
	case "s", "strike", "del":
		m.strike = true
	}
	c := css(n)
	if v, ok := c["font-weight"]; ok {
		if v == "normal" || v == "400" || v == "lighter" {
			m.bold = false
		} else if v == "bold" || v == "bolder" {
			m.bold = true
		} else if n, err := strconv.Atoi(v); err == nil {
			m.bold = n >= 600
		}
	}
	if v, ok := c["font-style"]; ok {
		m.italic = v == "italic" || v == "oblique"
	}
	if v, ok := c["text-decoration"]; ok {
		m.strike = strings.Contains(v, "line-through")
	}
	return m
}

func hidden(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	switch n.Data {
	case "script", "style", "head", "iframe", "object", "embed", "svg", "math", "noscript", "template", "o:p":
		return true
	}
	c := css(n)
	return c["display"] == "none" || c["visibility"] == "hidden" || c["mso-hide"] == "all" || c["mso-list"] == "ignore"
}

func collapse(s string) string {
	var b strings.Builder
	spacePending := false
	for _, r := range s {
		if unicode.IsSpace(r) || r == '\u00a0' {
			spacePending = true
			continue
		}
		if spacePending {
			b.WriteByte(' ')
			spacePending = false
		}
		b.WriteRune(r)
	}
	if spacePending {
		b.WriteByte(' ')
	}
	return b.String()
}

func safeURL(s string, image bool) string {
	s = strings.TrimSpace(s)
	for _, r := range s {
		if unicode.IsControl(r) {
			return ""
		}
	}
	u, err := url.Parse(s)
	if err != nil {
		return ""
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "" && scheme != "http" && scheme != "https" && (!(!image && scheme == "mailto")) {
		return ""
	}
	if strings.HasPrefix(s, "//") {
		return ""
	}
	return strings.NewReplacer(" ", "%20", "(", "%28", ")", "%29", "<", "%3C", ">", "%3E", "\\", "%5C").Replace(s)
}

func textOf(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(textOf(c))
	}
	return b.String()
}

func inline(n *html.Node, w *inlineWriter, m marks) {
	if hidden(n) {
		return
	}
	if n.Type == html.TextNode {
		w.add(collapse(n.Data), m, false)
		return
	}
	if n.Type != html.ElementNode && n.Type != html.DocumentNode {
		return
	}
	if n.Type == html.ElementNode {
		m = styled(n, m)
		switch n.Data {
		case "br":
			if !strings.Contains(attr(n, "class"), "Apple-interchange-newline") {
				w.add("\\\n", m, true)
			}
			return
		case "img":
			dest := safeURL(attr(n, "src"), true)
			if dest != "" {
				w.add("!["+escapeText(attr(n, "alt"))+"]("+dest+")", m, true)
			}
			return
		case "a":
			var label inlineWriter
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				inline(c, &label, marks{})
			}
			text, dest := strings.TrimSpace(label.markdown()), safeURL(attr(n, "href"), false)
			if dest != "" && text != "" {
				w.add("["+text+"]("+dest+")", m, true)
			} else {
				w.add(text, m, true)
			}
			return
		case "code":
			s := strings.TrimSpace(textOf(n))
			fence := "`"
			for strings.Contains(s, fence) {
				fence += "`"
			}
			w.add(fence+s+fence, m, true)
			return
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		inline(c, w, m)
	}
}

func inlineContent(n *html.Node) string {
	var w inlineWriter
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		inline(c, &w, marks{})
	}
	return strings.TrimSpace(w.markdown())
}

type block struct {
	text string
	list bool
}

func isBlock(tag string) bool {
	switch tag {
	case "html", "body", "div", "section", "article", "main", "header", "footer", "p", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote", "pre", "ul", "ol", "table", "hr":
		return true
	}
	return false
}

var wordLevel = regexp.MustCompile(`(?i)\blevel(\d+)`)
var wordNumber = regexp.MustCompile(`^\s*\d+[.)]`)

func blocks(n *html.Node) []block {
	if hidden(n) {
		return nil
	}
	if n.Type == html.ElementNode {
		switch n.Data {
		case "p", "h1", "h2", "h3", "h4", "h5", "h6":
			s := inlineContent(n)
			if s == "" {
				return nil
			}
			if n.Data[0] == 'h' {
				return []block{{text: strings.Repeat("#", int(n.Data[1]-'0')) + " " + s}}
			}
			style := css(n)["mso-list"]
			if style != "" && style != "none" {
				level := 1
				if m := wordLevel.FindStringSubmatch(style); len(m) > 0 {
					if v, err := strconv.Atoi(m[1]); err == nil {
						level = min(max(v, 1), 9)
					}
				}
				marker := "- "
				if m := wordNumber.FindString(strings.TrimSpace(textOf(n))); m != "" {
					marker = strings.TrimSpace(m) + " "
				}
				return []block{{text: strings.Repeat("  ", level-1) + marker + s, list: true}}
			}
			return []block{{text: s}}
		case "blockquote":
			inner := joinBlocks(containerBlocks(n))
			if inner == "" {
				return nil
			}
			return []block{{text: "> " + strings.ReplaceAll(inner, "\n", "\n> ")}}
		case "pre":
			text := strings.Trim(textOf(n), "\n\r")
			lang := ""
			var findCode func(*html.Node)
			findCode = func(x *html.Node) {
				if x.Type == html.ElementNode && x.Data == "code" {
					for _, c := range strings.Fields(attr(x, "class")) {
						if strings.HasPrefix(c, "language-") {
							lang = strings.TrimPrefix(c, "language-")
						}
					}
				}
				for c := x.FirstChild; c != nil; c = c.NextSibling {
					findCode(c)
				}
			}
			findCode(n)
			fence := "```"
			for strings.Contains(text, fence) {
				fence += "`"
			}
			return []block{{text: fence + lang + "\n" + text + "\n" + fence}}
		case "ul", "ol":
			return []block{{text: renderList(n, 0)}}
		case "table":
			return []block{{text: renderTable(n)}}
		case "hr":
			return []block{{text: "---"}}
		}
	}
	return containerBlocks(n)
}

func containerBlocks(n *html.Node) []block {
	var out []block
	var pending inlineWriter
	flush := func() {
		if s := strings.TrimSpace(pending.markdown()); s != "" && s != `\` {
			out = append(out, block{text: s})
		}
		pending = inlineWriter{}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && (isBlock(c.Data) || containsBlock(c)) {
			flush()
			out = append(out, blocks(c)...)
			continue
		}
		inline(c, &pending, marks{})
	}
	flush()
	return out
}

func containsBlock(n *html.Node) bool {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && (isBlock(c.Data) || containsBlock(c)) {
			return true
		}
	}
	return false
}

func joinBlocks(bs []block) string {
	var b strings.Builder
	previousList := false
	for _, item := range bs {
		if item.text == "" {
			continue
		}
		if b.Len() > 0 {
			if item.list && previousList {
				b.WriteByte('\n')
			} else {
				b.WriteString("\n\n")
			}
		}
		b.WriteString(item.text)
		previousList = item.list
	}
	return b.String()
}

func renderList(n *html.Node, depth int) string {
	var lines []string
	number := 0
	for li := n.FirstChild; li != nil; li = li.NextSibling {
		// Google Docs emits nested lists as siblings of their parent <li>.
		if li.Type == html.ElementNode && (li.Data == "ul" || li.Data == "ol") {
			lines = append(lines, renderList(li, depth+1))
			continue
		}
		if li.Type != html.ElementNode || li.Data != "li" {
			continue
		}
		number++
		var w inlineWriter
		var children []string
		for c := li.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && (c.Data == "ul" || c.Data == "ol") {
				children = append(children, renderList(c, depth+1))
			} else {
				inline(c, &w, marks{})
			}
		}
		marker := "- "
		if n.Data == "ol" {
			marker = strconv.Itoa(number) + ". "
		}
		lines = append(lines, strings.Repeat("  ", depth)+marker+strings.TrimSpace(w.markdown()))
		lines = append(lines, children...)
	}
	return strings.Join(lines, "\n")
}

func renderTable(n *html.Node) string {
	var rows [][]string
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		if x != n && x.Type == html.ElementNode && x.Data == "table" {
			return
		}
		if x.Type == html.ElementNode && x.Data == "tr" {
			var cells []string
			for c := x.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && (c.Data == "td" || c.Data == "th") {
					s := inlineContent(c)
					s = strings.ReplaceAll(s, "|", `\|`)
					s = strings.ReplaceAll(s, "\\\n", "<br>")
					cells = append(cells, s)
				}
			}
			if len(cells) > 0 {
				rows = append(rows, cells)
			}
			return
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	if len(rows) == 0 {
		return ""
	}
	cols := len(rows[0])
	for _, row := range rows {
		cols = max(cols, len(row))
	}
	line := func(cells []string) string {
		v := make([]string, cols)
		copy(v, cells)
		return "| " + strings.Join(v, " | ") + " |"
	}
	sep := make([]string, cols)
	for i := range sep {
		sep[i] = "---"
	}
	out := []string{line(rows[0]), line(sep)}
	for _, row := range rows[1:] {
		out = append(out, line(row))
	}
	return strings.Join(out, "\n")
}

// HTMLToMarkdown removes executable markup and converts supported elements.
func (Converter) HTMLToMarkdown(input string) (string, error) {
	doc, err := html.Parse(strings.NewReader(input))
	if err != nil {
		return escapeText(input), nil
	}
	return joinBlocks(blocks(doc)), nil
}
