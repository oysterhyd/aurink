package export

import (
	"bufio"
	"context"
	"html"
	"io"
	"strconv"
	"strings"
	"unicode"

	"aurink/internal/contract"
)

// Exporter writes HTML from immutable text and AST snapshots. Its zero value is
// ready to use; Export is blocking and must run off the UI thread.
type Exporter struct{}

var _ contract.Exporter = (*Exporter)(nil)

// New returns an exporter.
func New() *Exporter { return &Exporter{} }

// Export writes a complete HTML document or a body fragment.
func (e *Exporter) Export(ctx context.Context, t contract.Text, ast contract.AST, opts contract.ExportOptions, w io.Writer) error {
	const op = "export.Export"
	if ctx == nil || t == nil || ast == nil || w == nil {
		return contract.E(contract.CodeInvalid, op, nil)
	}
	if err := ctx.Err(); err != nil {
		return contract.E(contract.CodeCanceled, op, err)
	}
	if opts.Format == contract.ExportPDF {
		return contract.E(contract.CodeUnavailable, op, nil)
	}
	if opts.Format != contract.ExportHTML || t.Version() != ast.Version() || ast.Len() == 0 {
		return contract.E(contract.CodeInvalid, op, nil)
	}

	r := renderer{
		ctx: ctx, text: t, ast: ast, opts: opts, out: bufio.NewWriter(w),
		headings: make(map[int]heading), definitions: make(map[string]int),
		noteIndex: make(map[string]*note), usedNoteIDs: make(map[string]bool),
	}
	r.raw.r = &r
	r.progress(0)
	r.prepare()
	if !opts.Fragment {
		r.write("<!DOCTYPE html>\n<html><head><meta charset=\"utf-8\"><title>")
		r.escaped(opts.Title)
		r.write("</title><style>\n")
		if opts.Dark {
			r.write(darkCSS)
		} else {
			r.write(lightCSS)
		}
		r.write(commonCSS)
		r.write("</style></head><body>\n")
	}
	if opts.TOC {
		r.toc()
	}
	r.blocks(0)
	r.raw.close()
	r.footnotes()
	if !opts.Fragment {
		r.write("</body></html>\n")
	}
	if r.err == nil {
		r.err = ctx.Err()
	}
	if r.err == nil {
		r.err = r.out.Flush()
	}
	if r.err == nil {
		r.err = ctx.Err()
	}
	if r.err != nil {
		if ctx.Err() != nil {
			return contract.E(contract.CodeCanceled, op, ctx.Err())
		}
		return contract.E(contract.CodeIO, op, r.err)
	}
	r.progress(1)
	return nil
}

type heading struct {
	id, text string
	level    int
}

type note struct {
	id, content string
	block       int
	number      int
	refs        int
}

type renderer struct {
	ctx         context.Context
	text        contract.Text
	ast         contract.AST
	opts        contract.ExportOptions
	out         *bufio.Writer
	err         error
	lastStep    int
	headings    map[int]heading
	outline     []heading
	definitions map[string]int
	notes       []*note
	noteIndex   map[string]*note
	usedNoteIDs map[string]bool
	raw         rawHTML
}

func (r *renderer) write(s string) {
	for len(s) > 0 && r.err == nil {
		if r.err = r.ctx.Err(); r.err != nil {
			return
		}
		n := min(len(s), 32*1024)
		_, r.err = r.out.WriteString(s[:n])
		s = s[n:]
	}
}

func (r *renderer) escaped(s string) { r.write(html.EscapeString(s)) }

func (r *renderer) open(s string) {
	r.write(s)
	r.raw.beginContainer()
}

func (r *renderer) close(s string) {
	r.raw.endContainer()
	r.write(s)
}

// Progress is reported in at most 41 steps, regardless of document size.
func (r *renderer) progress(f float32) {
	step := int(f * 40)
	if r.opts.OnProgress != nil && (step > r.lastStep || f == 0 || f == 1) {
		r.lastStep = step
		r.opts.OnProgress(f)
	}
	if r.err == nil {
		r.err = r.ctx.Err()
	}
}

func (r *renderer) prepare() {
	used := make(map[string]bool)
	nextSuffix := make(map[string]int)
	for i := 1; i < r.ast.Len() && r.err == nil; i++ {
		r.progress(float32(i) / float32(r.ast.Len()) * .2)
		b := r.ast.Block(i)
		switch b.Kind {
		case contract.BlockHeading:
			label := plain(r.ast.Inlines(i))
			base := slug(label)
			id := base
			n := max(nextSuffix[base], 1)
			for used[id] {
				id = base + "-" + strconv.Itoa(n)
				n++
			}
			nextSuffix[base] = n
			used[id] = true
			h := heading{id: id, text: label, level: b.Level}
			r.headings[i] = h
			r.outline = append(r.outline, h)
		case contract.BlockFootnoteDef:
			key := normalizeLabel(b.Label)
			if _, exists := r.definitions[key]; !exists {
				r.definitions[key] = i
			}
		}
	}
}

func slug(s string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(s) {
		switch {
		case unicode.IsSpace(c):
			b.WriteByte('-')
		case unicode.IsLetter(c), unicode.IsNumber(c), c == '-', c == '_':
			b.WriteRune(c)
		}
	}
	if b.Len() == 0 {
		return "section"
	}
	return b.String()
}

func normalizeLabel(s string) string {
	s = strings.Join(strings.FieldsFunc(s, func(c rune) bool {
		return c == ' ' || c == '\t' || c == '\r' || c == '\n'
	}), " ")
	s = strings.ReplaceAll(strings.ToLower(s), "ß", "ss")
	return strings.ToLower(strings.ToUpper(s))
}

func plain(in []contract.Inline) string {
	var b strings.Builder
	for _, n := range in {
		switch n.Kind {
		case contract.InlineText, contract.InlineCode, contract.InlineMath, contract.InlineEscape, contract.InlineEntity, contract.InlineEmoji:
			b.WriteString(n.Text)
		case contract.InlineSoftBreak, contract.InlineLineBreak:
			b.WriteByte(' ')
		}
	}
	return b.String()
}

func (r *renderer) toc() {
	r.write("<nav class=\"toc\"><ol>\n")
	for _, h := range r.outline {
		r.write("<li class=\"toc-level-" + strconv.Itoa(h.level) + "\"><a href=\"#")
		r.escaped(h.id)
		r.write("\">")
		r.escaped(h.text)
		r.write("</a></li>\n")
	}
	r.write("</ol></nav>\n")
}

func (r *renderer) blocks(parent int) {
	for i := range r.ast.Children(parent) {
		if r.err != nil {
			return
		}
		r.progress(.2 + .75*float32(i)/float32(r.ast.Len()))
		r.block(i)
	}
}

func (r *renderer) tight(b *contract.Block) bool {
	if b.Parent <= 0 {
		return false
	}
	parent := r.ast.Block(b.Parent)
	return parent.Kind == contract.BlockItem && parent.Parent >= 0 && r.ast.Block(parent.Parent).Tight
}

func (r *renderer) block(i int) {
	b := r.ast.Block(i)
	if r.raw.literal() {
		r.blockInRaw(i, b)
		return
	}
	switch b.Kind {
	case contract.BlockParagraph:
		tight := r.tight(b)
		if !tight {
			r.open("<p>")
		}
		r.inlines(i)
		if !tight {
			r.close("</p>\n")
		}
	case contract.BlockHeading:
		tag := "h" + strconv.Itoa(b.Level)
		r.write("<" + tag + " id=\"")
		r.escaped(r.headings[i].id)
		r.write("\">")
		r.raw.beginContainer()
		r.inlines(i)
		r.close("</" + tag + ">\n")
	case contract.BlockThematicBreak:
		r.write("<hr />\n")
	case contract.BlockCode:
		code := r.lines(b)
		if strings.EqualFold(b.Lang, "mermaid") {
			r.write("<pre class=\"mermaid\">")
			r.escaped(code)
			r.write("</pre>\n")
			break
		}
		r.write("<pre><code")
		if b.Lang != "" {
			r.write(" class=\"language-")
			r.escaped(b.Lang)
			r.write("\"")
		}
		r.write(">")
		r.code(b.Lang, code)
		r.write("</code></pre>\n")
	case contract.BlockMath:
		r.write("<pre><code class=\"math\">")
		r.escaped(strings.TrimSuffix(r.lines(b), "\n"))
		r.write("</code></pre>\n")
	case contract.BlockHTML:
		r.raw.scan(r.lines(b))
	case contract.BlockQuote:
		r.open("<blockquote>\n")
		r.blocks(i)
		r.close("</blockquote>\n")
	case contract.BlockList:
		tag := "ul"
		if b.Ordered {
			tag = "ol"
		}
		r.write("<" + tag)
		if b.Ordered && b.Number != 1 {
			r.write(" start=\"" + strconv.Itoa(b.Number) + "\"")
		}
		r.write(">\n")
		r.raw.beginContainer()
		r.blocks(i)
		r.close("</" + tag + ">\n")
	case contract.BlockItem:
		r.open("<li>")
		if b.Task != contract.TaskNone {
			r.write("<input type=\"checkbox\" disabled=\"\"")
			if b.Task == contract.TaskDone {
				r.write(" checked=\"\"")
			}
			r.write(" /> ")
		}
		r.blocks(i)
		r.close("</li>\n")
	case contract.BlockTable:
		r.table(i, b)
	case contract.BlockTOC:
		r.toc()
	}
}

// Continue looking for a closing raw tag without emitting Markdown markup
// inside a discarded or text-only HTML element.
func (r *renderer) blockInRaw(i int, b *contract.Block) {
	switch b.Kind {
	case contract.BlockHTML:
		r.raw.scan(r.lines(b))
	case contract.BlockParagraph, contract.BlockHeading, contract.BlockTableCell:
		r.inlines(i)
	case contract.BlockCode, contract.BlockMath:
		if !r.raw.dropped() {
			r.escaped(r.lines(b))
		}
	case contract.BlockQuote, contract.BlockList, contract.BlockItem, contract.BlockTable, contract.BlockTableRow:
		r.blocks(i)
	}
}

func (r *renderer) lines(b *contract.Block) string {
	var s strings.Builder
	for i, line := range b.Lines {
		if i%256 == 0 && r.ctx.Err() != nil {
			r.err = r.ctx.Err()
			break
		}
		s.WriteString(strings.Repeat(" ", int(line.Padding)))
		s.WriteString(r.text.Slice(line.Source))
		s.WriteByte('\n')
	}
	return s.String()
}

func (r *renderer) code(lang, code string) {
	if r.opts.Highlight == nil {
		r.escaped(code)
		return
	}
	at := 0
	for _, token := range r.opts.Highlight(lang, code) {
		if token.Range.Start < at || token.Range.End < token.Range.Start || token.Range.End > len(code) || !validTokenKind(token.Kind) {
			continue
		}
		r.escaped(code[at:token.Range.Start])
		r.write("<span class=\"tok-" + token.Kind + "\">")
		r.escaped(code[token.Range.Start:token.Range.End])
		r.write("</span>")
		at = token.Range.End
	}
	r.escaped(code[at:])
}

func validTokenKind(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if (c < 'a' || c > 'z') && c != '-' {
			return false
		}
	}
	return true
}

func (r *renderer) table(i int, table *contract.Block) {
	r.open("<table>\n")
	body := false
	columns := len(table.Align)
	for rowIndex := range r.ast.Children(i) {
		if r.ast.Block(rowIndex).Header {
			columns = 0
			for range r.ast.Children(rowIndex) {
				columns++
			}
			break
		}
	}
	for rowIndex := range r.ast.Children(i) {
		row := r.ast.Block(rowIndex)
		if row.Header {
			r.open("<thead>\n")
		} else if !body {
			r.open("<tbody>\n")
			body = true
		}
		r.open("<tr>\n")
		col := 0
		for cell := range r.ast.Children(rowIndex) {
			if columns > 0 && col >= columns {
				break
			}
			tag := "td"
			if row.Header {
				tag = "th"
			}
			r.write("<" + tag)
			if col < len(table.Align) {
				var align string
				switch table.Align[col] {
				case contract.AlignLeft:
					align = "left"
				case contract.AlignCenter:
					align = "center"
				case contract.AlignRight:
					align = "right"
				}
				if align != "" {
					r.write(" align=\"" + align + "\"")
				}
			}
			r.write(">")
			r.raw.beginContainer()
			r.inlines(cell)
			r.close("</" + tag + ">\n")
			col++
		}
		r.close("</tr>\n")
		if row.Header {
			r.close("</thead>\n")
		}
	}
	if body {
		r.close("</tbody>\n")
	}
	r.close("</table>\n")
}
