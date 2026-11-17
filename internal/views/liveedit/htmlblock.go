package liveedit

import (
	"io"
	"net/url"
	"strconv"
	"strings"

	"aurink/internal/contract"
	"aurink/internal/views/editorapi"
	"github.com/egoist/mygo/ui"
	"golang.org/x/net/html"
)

// Safe HTML block subset renderer (09 D8, FE-1.8): inactive BlockHTML rows
// whose markup strictly matches the safe whitelist render as native blocks
// (aligned paragraphs/headings/badges, blockquotes, rules, and collapsible
// <details><summary> disclosures) while preserving 100% of raw bytes on disk
// and falling back to pale monospace source whenever active or non-whitelisted.

type htmlDetailsKey struct {
	id  contract.BlockID
	idx int
}

type htmlNode struct {
	tag      string // "" for text node
	text     string
	align    string // "", "left", "center", "right"
	href     string
	src      string
	alt      string
	title    string
	width    int
	height   int
	hasOpen  bool
	children []*htmlNode
}

type htmlBlockLayout struct {
	height    float32
	items     []htmlDrawItem
	summaries []htmlSummaryHit
	links     []htmlLinkHit
	images    []imageBox
}

type htmlItemKind uint8

const (
	htmlDrawText htmlItemKind = iota
	htmlDrawHR
	htmlDrawQuoteBar
	htmlDrawDetailsBox
	htmlDrawChevron
)

type htmlDrawItem struct {
	kind   htmlItemKind
	rect   ui.Rect
	layout *ui.TextLayout
	muted  bool
	open   bool
	images []htmlInlineImg
}

type htmlInlineImg struct {
	objIndex int
	src      string
	alt      string
}

type htmlSummaryHit struct {
	key  htmlDetailsKey
	open bool
	rect ui.Rect
}

type htmlLinkHit struct {
	href   string
	layout *ui.TextLayout
	x, y   float32
	uStart int
	uEnd   int
}

var htmlBlockTags = map[string]bool{
	"p": true, "div": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"blockquote": true, "details": true, "summary": true, "hr": true,
}

var htmlInlineTags = map[string]bool{
	"br": true, "a": true, "img": true,
	"strong": true, "b": true, "em": true, "i": true,
	"code": true, "u": true, "s": true, "del": true,
	"kbd": true, "mark": true, "sub": true, "sup": true,
}

var htmlVoidTags = map[string]bool{
	"br": true, "hr": true, "img": true,
}

// IsSafeHTMLBlock reports whether raw strictly conforms to the D8 safe HTML
// subset without unknown tags, event/style attributes, remote images, or
// malformed/unclosed tags.
func IsSafeHTMLBlock(raw string) bool {
	_, ok := parseSafeHTML(raw)
	return ok
}

func parseSafeHTML(raw string) ([]*htmlNode, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || !strings.Contains(trimmed, "<") || !strings.Contains(trimmed, ">") {
		return nil, false
	}
	root := &htmlNode{tag: "root"}
	stack := []*htmlNode{root}
	elems := 0

	z := html.NewTokenizer(strings.NewReader(trimmed))
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			if z.Err() != io.EOF {
				return nil, false
			}
			// Reject truncated tags or unclosed comments at EOF (e.g. "<p>hi</p><b").
			if strings.Contains(string(z.Raw()), "<") || len(stack) != 1 || elems == 0 {
				return nil, false
			}
			if !validateHTMLStructure(root.children, false) {
				return nil, false
			}
			return root.children, true

		case html.CommentToken, html.DoctypeToken:
			return nil, false

		case html.TextToken:
			txt := string(z.Text())
			cur := stack[len(stack)-1]
			cur.children = append(cur.children, &htmlNode{text: txt})

		case html.StartTagToken, html.SelfClosingTagToken:
			rawName, hasAttr := z.TagName()
			tag := strings.ToLower(string(rawName))
			if !htmlBlockTags[tag] && !htmlInlineTags[tag] {
				return nil, false
			}
			if tt == html.SelfClosingTagToken && !htmlVoidTags[tag] {
				return nil, false
			}
			parent := stack[len(stack)-1]
			if !canContainTag(parent.tag, tag) {
				return nil, false
			}
			node := &htmlNode{tag: tag}
			seen := map[string]bool{}
			for hasAttr {
				var k, v []byte
				k, v, hasAttr = z.TagAttr()
				attr := strings.ToLower(string(k))
				if attr == "" || seen[attr] || strings.HasPrefix(attr, "on") || attr == "style" {
					return nil, false
				}
				seen[attr] = true
				if !applySafeAttr(node, attr, string(v)) {
					return nil, false
				}
			}
			if tag == "img" && node.src == "" {
				return nil, false
			}
			if tag == "a" && node.href == "" {
				return nil, false
			}
			elems++
			parent.children = append(parent.children, node)
			if !htmlVoidTags[tag] {
				stack = append(stack, node)
			}

		case html.EndTagToken:
			rawName, hasAttr := z.TagName()
			if hasAttr {
				return nil, false
			}
			tag := strings.ToLower(string(rawName))
			if htmlVoidTags[tag] || len(stack) <= 1 || stack[len(stack)-1].tag != tag {
				return nil, false
			}
			stack = stack[:len(stack)-1]
		}
	}
}

func canContainTag(parent, child string) bool {
	if child == "summary" {
		return parent == "details"
	}
	if htmlInlineTags[parent] {
		// Inline elements may only contain inline elements (and no nested <a>).
		if parent == "a" && child == "a" {
			return false
		}
		return htmlInlineTags[child]
	}
	switch parent {
	case "root", "div", "blockquote", "details":
		return true
	case "p", "h1", "h2", "h3", "h4", "h5", "h6", "summary":
		return htmlInlineTags[child]
	}
	return false
}

func validateHTMLStructure(nodes []*htmlNode, inDetails bool) bool {
	elemIdx := 0
	for _, n := range nodes {
		if n.tag == "" {
			continue
		}
		if n.tag == "summary" && (!inDetails || elemIdx != 0) {
			return false
		}
		elemIdx++
		if !validateHTMLStructure(n.children, n.tag == "details") {
			return false
		}
	}
	return true
}

func applySafeAttr(n *htmlNode, attr, val string) bool {
	switch attr {
	case "align":
		switch n.tag {
		case "p", "div", "h1", "h2", "h3", "h4", "h5", "h6":
			a := strings.ToLower(strings.TrimSpace(val))
			if a != "left" && a != "center" && a != "right" {
				return false
			}
			n.align = a
			return true
		}
		return false
	case "open":
		if n.tag != "details" {
			return false
		}
		v := strings.ToLower(strings.TrimSpace(val))
		if v != "" && v != "open" {
			return false
		}
		n.hasOpen = true
		return true
	case "href":
		if n.tag != "a" || !isSafeHTMLHref(val) {
			return false
		}
		n.href = strings.TrimSpace(val)
		return true
	case "src":
		if n.tag != "img" || !isSafeHTMLImgSrc(val) {
			return false
		}
		n.src = strings.TrimSpace(val)
		return true
	case "alt":
		if n.tag != "img" {
			return false
		}
		n.alt = val
		return true
	case "title":
		if n.tag != "img" && n.tag != "a" {
			return false
		}
		n.title = val
		return true
	case "width":
		if n.tag != "img" {
			return false
		}
		px, ok := parseHTMLDim(val)
		n.width = px
		return ok
	case "height":
		if n.tag != "img" {
			return false
		}
		px, ok := parseHTMLDim(val)
		n.height = px
		return ok
	}
	return false
}

func parseHTMLDim(s string) (int, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.TrimSuffix(s, "px")
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 || n > 4096 {
		return 0, false
	}
	return n, true
}

func hasControlByte(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] <= 0x1f || s[i] == 0x7f {
			return true
		}
	}
	return false
}

func stripASCIIWhite(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] > ' ' && s[i] != 0x7f {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func isSafeHTMLHref(raw string) bool {
	if hasControlByte(raw) {
		return false
	}
	s := strings.TrimSpace(raw)
	if s == "" || strings.HasPrefix(s, "//") || strings.HasPrefix(s, `\\`) {
		return false
	}
	compact := strings.ToLower(stripASCIIWhite(s))
	if strings.HasPrefix(compact, "//") || strings.HasPrefix(compact, `\\`) {
		return false
	}
	// Check explicit URI scheme before any '/', '?', or '#'.
	cut := strings.IndexAny(compact, "/?#")
	head := compact
	if cut >= 0 {
		head = compact[:cut]
	}
	if colon := strings.IndexByte(head, ':'); colon >= 0 {
		scheme := head[:colon]
		if scheme != "http" && scheme != "https" && scheme != "mailto" {
			return false
		}
		u, err := url.Parse(s)
		if err != nil {
			return false
		}
		if (scheme == "http" || scheme == "https") && u.Host == "" {
			return false
		}
		return true
	}
	if strings.HasPrefix(s, "#") {
		return len(s) > 1
	}
	_, err := url.Parse(s)
	return err == nil
}

func isSafeHTMLImgSrc(raw string) bool {
	if hasControlByte(raw) {
		return false
	}
	s := strings.TrimSpace(raw)
	if s == "" || strings.HasPrefix(s, "//") || strings.HasPrefix(s, `\\`) || strings.HasPrefix(s, "#") {
		return false
	}
	compact := strings.ToLower(stripASCIIWhite(s))
	if strings.Contains(compact, "://") || strings.HasPrefix(compact, "//") || strings.HasPrefix(compact, `\\`) {
		return false
	}
	// Reject any URI scheme (data:, javascript:, vbscript:, file:, http:, https:, blob:, about:, mailto:);
	// allow a Windows local drive prefix like "C:/..." or "C:\...".
	cut := strings.IndexAny(compact, `/\?#`)
	head := compact
	if cut >= 0 {
		head = compact[:cut]
	}
	if colon := strings.IndexByte(head, ':'); colon >= 0 {
		isDrive := colon == 1 && len(s) >= 3 && ((s[0] >= 'a' && s[0] <= 'z') || (s[0] >= 'A' && s[0] <= 'Z')) && (s[2] == '/' || s[2] == '\\')
		if !isDrive {
			return false
		}
	}
	return true
}

func uiAlignOf(align string) ui.Align {
	switch align {
	case "center":
		return ui.Center
	case "right":
		return ui.End
	}
	return ui.Start
}

type htmlLayoutBuilder struct {
	e          *Editor
	blockID    contract.BlockID
	detailsSeq int
	out        htmlBlockLayout
}

func (e *Editor) layoutHTMLBlock(b *contract.Block, width float32) *htmlBlockLayout {
	raw := e.text.Slice(b.Range)
	nodes, ok := parseSafeHTML(raw)
	if !ok {
		return nil
	}
	lb := htmlLayoutBuilder{e: e, blockID: b.ID}
	y := lb.layoutFlow(nodes, 0, 0, max(width, 1), "", false)
	lb.out.height = max(y, 4)
	return &lb.out
}

func (lb *htmlLayoutBuilder) detailsOpen(defaultOpen bool) (htmlDetailsKey, bool) {
	key := htmlDetailsKey{id: lb.blockID, idx: lb.detailsSeq}
	lb.detailsSeq++
	if lb.e.htmlOpen != nil {
		if open, ok := lb.e.htmlOpen[key]; ok {
			return key, open
		}
	}
	return key, defaultOpen
}

func (lb *htmlLayoutBuilder) layoutFlow(nodes []*htmlNode, x, y, width float32, inheritedAlign string, muted bool) float32 {
	ty := lb.e.ty
	var inlineBuf []*htmlNode
	flushInline := func() {
		if len(inlineBuf) == 0 {
			return
		}
		if hasVisibleInline(inlineBuf) {
			y = lb.emitRichRun(inlineBuf, x, y, width, ty.BodyFont(), inheritedAlign, muted)
			y += ty.Body * 0.45
		}
		inlineBuf = inlineBuf[:0]
	}

	for _, n := range nodes {
		if n.tag == "" || htmlInlineTags[n.tag] {
			inlineBuf = append(inlineBuf, n)
			continue
		}
		flushInline()
		align := inheritedAlign
		if n.align != "" {
			align = n.align
		}
		switch n.tag {
		case "hr":
			y += ty.Body * 0.35
			lb.out.items = append(lb.out.items, htmlDrawItem{
				kind: htmlDrawHR,
				rect: ui.Rect{X: x, Y: y + 4, W: width, H: 2},
			})
			y += 10 + ty.Body*0.35
		case "p":
			if hasVisibleInline(n.children) {
				y = lb.emitRichRun(n.children, x, y, width, ty.BodyFont(), align, muted)
				y += ty.Body * 0.5
			}
		case "h1", "h2", "h3", "h4", "h5", "h6":
			lvl := int(n.tag[1] - '0')
			if y > 0 {
				y += ty.Body * 0.35
			}
			if hasVisibleInline(n.children) {
				y = lb.emitRichRun(n.children, x, y, width, ty.HeadingFont(lvl), align, muted || lvl == 6)
				y += ty.Body * 0.4
			}
		case "div":
			y = lb.layoutFlow(n.children, x, y, width, align, muted)
		case "blockquote":
			startY := y
			innerX := x + quoteIndent
			innerW := max(width-quoteIndent, 1)
			endY := lb.layoutFlow(n.children, innerX, y, innerW, align, true)
			if endY > startY {
				barH := max(endY-startY-ty.Body*0.35, 12)
				lb.out.items = append(lb.out.items, htmlDrawItem{
					kind: htmlDrawQuoteBar,
					rect: ui.Rect{X: x, Y: startY, W: quoteBar, H: barH},
				})
				y = endY + ty.Body*0.25
			}
		case "details":
			y = lb.layoutDetails(n, x, y, width, align, muted)
		}
	}
	flushInline()
	if y > ty.Body*0.45 && len(lb.out.items) > 0 {
		y -= ty.Body * 0.35
	}
	return y
}

func (lb *htmlLayoutBuilder) layoutDetails(n *htmlNode, x, y, width float32, align string, muted bool) float32 {
	ty := lb.e.ty
	key, open := lb.detailsOpen(n.hasOpen)
	const padX, padY, chevW = 12, 8, 18

	var summaryNodes []*htmlNode
	var bodyNodes []*htmlNode
	for _, ch := range n.children {
		if ch.tag == "summary" && summaryNodes == nil {
			summaryNodes = ch.children
			continue
		}
		bodyNodes = append(bodyNodes, ch)
	}
	if !hasVisibleInline(summaryNodes) {
		summaryNodes = []*htmlNode{{text: "Details"}}
	}

	boxIdx := len(lb.out.items)
	lb.out.items = append(lb.out.items, htmlDrawItem{kind: htmlDrawDetailsBox})

	sumFont := ty.BodyFont()
	sumFont.Weight = 600
	sumStart := y + padY
	lb.out.items = append(lb.out.items, htmlDrawItem{
		kind: htmlDrawChevron,
		rect: ui.Rect{X: x + padX, Y: sumStart, W: 12, H: sumFont.Size * sumFont.LineHeight},
		open: open,
	})
	sumEnd := lb.emitRichRun(summaryNodes, x+padX+chevW, sumStart, max(width-2*padX-chevW, 1), sumFont, "left", muted)
	sumH := max(sumEnd-sumStart, sumFont.Size*sumFont.LineHeight) + 2*padY
	lb.out.summaries = append(lb.out.summaries, htmlSummaryHit{
		key:  key,
		open: open,
		rect: ui.Rect{X: x, Y: y, W: width, H: sumH},
	})

	curY := y + sumH
	if open && len(bodyNodes) > 0 {
		lb.out.items = append(lb.out.items, htmlDrawItem{
			kind: htmlDrawHR,
			rect: ui.Rect{X: x + padX, Y: curY, W: max(width-2*padX, 1), H: 1},
		})
		curY += padY
		curY = lb.layoutFlow(bodyNodes, x+padX, curY, max(width-2*padX, 1), align, muted)
		curY += padY
	}
	lb.out.items[boxIdx].rect = ui.Rect{X: x, Y: y, W: width, H: curY - y}
	return curY + ty.Body*0.45
}

func hasVisibleInline(nodes []*htmlNode) bool {
	for _, n := range nodes {
		switch {
		case n.tag == "img" || n.tag == "br":
			return true
		case n.tag == "":
			if strings.TrimSpace(n.text) != "" {
				return true
			}
		default:
			if hasVisibleInline(n.children) {
				return true
			}
		}
	}
	return false
}

type htmlInlineState struct {
	style contract.Style
	href  string
}

func (lb *htmlLayoutBuilder) emitRichRun(nodes []*htmlNode, x, y, width float32, font ui.Font, align string, muted bool) float32 {
	e := lb.e
	m := rowMetrics{font: font, width: width, muted: muted}
	var spans []ui.Span
	var imgs []htmlInlineImg
	var links []htmlLinkHit
	uOff := 0
	objSeq := 0
	atLineStart := true
	pendingSpace := false

	flushSpace := func(st htmlInlineState) {
		if !pendingSpace || atLineStart {
			pendingSpace = false
			return
		}
		pendingSpace = false
		seg := contract.Segment{Style: st.style, Link: st.href}
		spans = append(spans, e.styled(" ", &seg, &m))
		uOff++
	}

	var walk func(ns []*htmlNode, st htmlInlineState)
	walk = func(ns []*htmlNode, st htmlInlineState) {
		for _, n := range ns {
			switch n.tag {
			case "":
				s := n.text
				for i := 0; i < len(s); {
					if s[i] == ' ' || s[i] == '\t' || s[i] == '\r' || s[i] == '\n' {
						pendingSpace = true
						i++
						continue
					}
					j := i + 1
					for j < len(s) && s[j] != ' ' && s[j] != '\t' && s[j] != '\r' && s[j] != '\n' {
						j++
					}
					flushSpace(st)
					word := s[i:j]
					seg := contract.Segment{Style: st.style, Link: st.href}
					u0 := uOff
					spans = append(spans, e.styled(word, &seg, &m))
					uOff += utf16Len(word)
					if st.href != "" {
						links = append(links, htmlLinkHit{href: st.href, x: x, y: y, uStart: u0, uEnd: uOff})
					}
					atLineStart = false
					i = j
				}
			case "br":
				pendingSpace = false
				spans = append(spans, ui.Span{Text: "\n"})
				uOff++
				atLineStart = true
			case "img":
				flushSpace(st)
				iw, ih := lb.sizeHTMLImage(n, width)
				spans = append(spans, ui.Span{Text: "\uFFFC", ObjectWidth: max(iw, 1), ObjectHeight: max(ih, 1)})
				imgs = append(imgs, htmlInlineImg{objIndex: objSeq, src: n.src, alt: n.alt})
				if st.href != "" {
					links = append(links, htmlLinkHit{href: st.href, x: x, y: y, uStart: uOff, uEnd: uOff + 1})
				}
				objSeq++
				uOff++
				atLineStart = false
			default:
				next := st
				switch n.tag {
				case "strong", "b":
					next.style |= contract.StyleStrong
				case "em", "i":
					next.style |= contract.StyleEmphasis
				case "code":
					next.style |= contract.StyleCode
				case "u":
					next.style |= contract.StyleUnderline
				case "s", "del":
					next.style |= contract.StyleStrike
				case "kbd":
					next.style |= contract.StyleKbd
				case "mark":
					next.style |= contract.StyleHighlight
				case "sub":
					next.style |= contract.StyleSub
				case "sup":
					next.style |= contract.StyleSup
				case "a":
					next.style |= contract.StyleLink
					next.href = n.href
				}
				walk(n.children, next)
			}
		}
	}
	walk(nodes, htmlInlineState{})
	if len(spans) == 0 {
		return y
	}
	lay := ui.ShapeRichTextWith(spans, font, ui.TextShape{Width: width, Align: uiAlignOf(align)})
	e.Stats.Shapes++
	e.Stats.FrameShapes++
	_, h := lay.Size()
	if h <= 0 {
		h = e.lineHeight(font, width)
	}
	for i := range links {
		links[i].layout = lay
		lb.out.links = append(lb.out.links, links[i])
	}
	objs := lay.Objects()
	for _, im := range imgs {
		if im.objIndex < len(objs) && e.image(im.src).src != nil {
			o := objs[im.objIndex].Rect
			lb.out.images = append(lb.out.images, imageBox{
				src: im.src,
				box: ui.Rect{X: x + o.X, Y: y + o.Y, W: o.W, H: o.H},
			})
		}
	}
	lb.out.items = append(lb.out.items, htmlDrawItem{
		kind:   htmlDrawText,
		rect:   ui.Rect{X: x, Y: y, W: width, H: h},
		layout: lay,
		muted:  muted,
		images: imgs,
	})
	return y + h
}

func (lb *htmlLayoutBuilder) sizeHTMLImage(n *htmlNode, maxW float32) (w, h float32) {
	st := lb.e.image(n.src)
	var natW, natH float32
	if st.src != nil {
		natW, natH = ui.ImageSize(st.src)
	}
	switch {
	case n.width > 0 && n.height > 0:
		w, h = float32(n.width), float32(n.height)
	case n.width > 0:
		w = float32(n.width)
		if natW > 0 && natH > 0 {
			h = natH * w / natW
		} else {
			h = max(w*0.5, 24)
		}
	case n.height > 0:
		h = float32(n.height)
		if natW > 0 && natH > 0 {
			w = natW * h / natH
		} else {
			w = max(h*2, 48)
		}
	case natW > 0 && natH > 0:
		w, h = natW, natH
	default:
		w, h = placeholderW, placeholderH
	}
	if w > maxW && w > 0 {
		h = h * maxW / w
		w = maxW
	}
	return max(w, 1), max(h, 1)
}

func (rc *rowContext) paintHTMLBlock(hb *htmlBlockLayout) {
	e, p, t := rc.e, rc.p, rc.t
	x0, y0 := rc.textX(), rc.textY()
	for i := range hb.items {
		it := &hb.items[i]
		box := ui.Rect{X: x0 + it.rect.X, Y: y0 + it.rect.Y, W: it.rect.W, H: it.rect.H}
		switch it.kind {
		case htmlDrawDetailsBox:
			p.Fill(box, t.Surface.Alpha(0.55), 8)
			p.Stroke(box, t.Border, 8, 1)
		case htmlDrawChevron:
			cx, cy := box.X+5, box.Y+box.H/2
			tri := new(ui.Path)
			if it.open {
				tri.MoveTo(cx-4, cy-2).LineTo(cx+4, cy-2).LineTo(cx, cy+3).Close()
			} else {
				tri.MoveTo(cx-2, cy-4).LineTo(cx+3, cy).LineTo(cx-2, cy+4).Close()
			}
			p.FillPath(tri, t.Muted)
		case htmlDrawHR:
			p.Fill(box, t.Faint.Alpha(0.35), 0)
		case htmlDrawQuoteBar:
			p.Fill(box, t.Faint.Alpha(0.35), 0)
		case htmlDrawText:
			col := t.Text
			if it.muted {
				col = t.Muted
			}
			p.TextLayout(it.layout, box.X, box.Y, col)
			objs := it.layout.Objects()
			for _, im := range it.images {
				if im.objIndex >= len(objs) {
					continue
				}
				o := objs[im.objIndex].Rect
				ib := ui.Rect{X: box.X + o.X, Y: box.Y + o.Y, W: o.W, H: o.H}
				st := e.image(im.src)
				if st.src != nil && e.flying(rc.row, im.src) {
					continue
				}
				if st.src != nil {
					p.Image(st.src, ib, ui.Contain)
					continue
				}
				p.Fill(ib, t.Surface, 8)
				p.Stroke(ib, t.Border, 8, 1)
				if st.failed {
					lbl := im.alt
					if lbl == "" {
						lbl = im.src
					}
					p.TextLayout(e.label(lbl, 12), ib.X+12, ib.Y+12, t.Muted)
				}
			}
		}
	}
}

func (e *Editor) htmlLinkAt(r int, l *laid, x, y float32) string {
	if l.html == nil {
		return ""
	}
	lx := x - e.contentX() - l.m.x
	ly := y - e.rowTextY(r)
	for i := range l.html.links {
		lk := &l.html.links[i]
		for _, sr := range lk.layout.SelectionRects(ui.TextInputRange{Start: lk.uStart, End: lk.uEnd}) {
			rx, ry := lk.x+sr.X, lk.y+sr.Y
			if lx >= rx && lx < rx+sr.W && ly >= ry && ly < ry+sr.H {
				return lk.href
			}
		}
	}
	return ""
}

func (e *Editor) htmlClick(r int, ev ui.InputEvent) bool {
	l := e.current(r)
	if l.html == nil {
		return false
	}
	lx := ev.X - e.contentX() - l.m.x
	ly := ev.Y - e.rowTextY(r)
	for _, s := range l.html.summaries {
		if lx >= s.rect.X && lx < s.rect.X+s.rect.W && ly >= s.rect.Y && ly < s.rect.Y+s.rect.H {
			if e.htmlOpen == nil {
				e.htmlOpen = map[htmlDetailsKey]bool{}
			}
			e.htmlOpen[s.key] = !s.open
			e.dropForms(r)
			e.invalidate()
			return true
		}
	}
	if ev.Mods&ui.Cmd != 0 || e.mode == editorapi.ModeRead {
		if href := e.htmlLinkAt(r, l, ev.X, ev.Y); href != "" {
			if strings.HasPrefix(href, "#") {
				slug := strings.TrimPrefix(href, "#")
				for _, h := range e.outline() {
					if h.Slug == slug {
						e.RevealOffset(h.Range.Start, editorapi.AlignTop)
						return true
					}
				}
			} else {
				e.openLink(href)
				return true
			}
		}
	}
	return false
}

// HTMLRendered reports whether row r is currently rendered as a safe HTML
// block rather than pale source, for tests.
func (e *Editor) HTMLRendered(r int) bool {
	if r < 0 || r >= len(e.rows) {
		return false
	}
	return e.current(r).html != nil
}

// HTMLDetailsOpen reports whether the idx-th <details> in row r is expanded,
// for tests.
func (e *Editor) HTMLDetailsOpen(r, idx int) bool {
	if r < 0 || r >= len(e.rows) {
		return false
	}
	l := e.current(r)
	if l.html == nil {
		return false
	}
	for _, s := range l.html.summaries {
		if s.key.idx == idx {
			return s.open
		}
	}
	return false
}
