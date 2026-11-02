package export

import (
	"html"
	"net/url"
	"strings"
	"unicode"
)

// rawHTML accepts a small set of inert HTML elements and attributes. The same
// scanner handles block HTML and the separate raw-tag nodes in inline content.
type rawHTML struct {
	r       *renderer
	inline  bool
	open    []rawTag
	blocked []string
	depth   int
}

type rawTag struct {
	name  string
	depth int
}

var safeTags = map[string]bool{
	"a": true, "abbr": true, "b": true, "blockquote": true, "br": true,
	"caption": true, "code": true, "col": true, "colgroup": true, "dd": true,
	"del": true, "details": true, "div": true, "dl": true, "dt": true,
	"em": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"hr": true, "i": true, "img": true, "ins": true, "kbd": true, "li": true,
	"mark": true, "ol": true, "p": true, "pre": true, "s": true, "samp": true,
	"small": true, "span": true, "strong": true, "sub": true, "summary": true,
	"sup": true, "table": true, "tbody": true, "td": true, "tfoot": true,
	"th": true, "thead": true, "tr": true, "u": true, "ul": true, "var": true, "wbr": true,
}

var inlineTags = map[string]bool{
	"a": true, "abbr": true, "b": true, "br": true, "code": true, "del": true,
	"em": true, "i": true, "img": true, "ins": true, "kbd": true, "mark": true,
	"s": true, "samp": true, "small": true, "span": true, "strong": true,
	"sub": true, "sup": true, "u": true, "var": true, "wbr": true,
}

var voidElements = map[string]bool{"br": true, "col": true, "hr": true, "img": true, "wbr": true}

func (s *rawHTML) dropped() bool {
	return len(s.blocked) > 0 && (s.blocked[len(s.blocked)-1] == "script" || s.blocked[len(s.blocked)-1] == "style")
}

func (s *rawHTML) literal() bool { return len(s.blocked) > 0 }

func (s *rawHTML) scan(source string) {
	for len(source) > 0 && s.r.err == nil {
		if source[0] != '<' {
			end := strings.IndexByte(source, '<')
			if end < 0 {
				end = len(source)
			}
			if !s.dropped() {
				s.r.escaped(html.UnescapeString(source[:end]))
			}
			source = source[end:]
			continue
		}
		if strings.HasPrefix(source, "<!--") {
			end := strings.Index(source[4:], "-->")
			if end < 0 {
				return
			}
			source = source[end+7:]
			continue
		}
		name, closing, selfClosing, attrs, end := parseTag(source)
		if end == 0 {
			if !s.dropped() {
				s.r.write("&lt;")
			}
			source = source[1:]
			continue
		}
		token := source[:end]
		source = source[end:]
		if len(s.blocked) > 0 {
			if closing && name == s.blocked[len(s.blocked)-1] {
				s.blocked = s.blocked[:len(s.blocked)-1]
			} else if !closing && !s.dropped() && (name == "script" || name == "style" || name == "iframe" || name == "object") {
				s.blocked = append(s.blocked, name)
			}
			continue
		}
		switch name {
		case "script", "style", "iframe", "object":
			if !closing {
				s.blocked = append(s.blocked, name)
			}
			continue
		case "embed":
			continue
		}
		if !safeTags[name] || s.inline && !inlineTags[name] {
			s.r.escaped(token)
			continue
		}
		if closing {
			s.endTag(name)
			continue
		}
		s.r.write("<" + name)
		s.attributes(name, attrs)
		if voidElements[name] {
			s.r.write(" />")
		} else if selfClosing {
			s.r.write("></" + name + ">")
		} else {
			s.r.write(">")
			s.open = append(s.open, rawTag{name: name, depth: s.depth})
		}
	}
}

func (s *rawHTML) endTag(name string) {
	for i := len(s.open) - 1; i >= 0; i-- {
		if s.open[i].name == name && s.open[i].depth == s.depth {
			for j := len(s.open) - 1; j >= i; j-- {
				s.r.write("</" + s.open[j].name + ">")
			}
			s.open = s.open[:i]
			return
		}
	}
}

func (s *rawHTML) close() {
	for i := len(s.open) - 1; i >= 0; i-- {
		s.r.write("</" + s.open[i].name + ">")
	}
	s.open = nil
	s.blocked = nil
}

// Markdown containers can surround raw tags (and vice versa). Close raw tags
// opened inside a container before closing that container, so even malformed
// input cannot cross the generated markup's nesting boundaries.
func (s *rawHTML) beginContainer() { s.depth++ }

func (s *rawHTML) endContainer() {
	for len(s.open) > 0 && s.open[len(s.open)-1].depth == s.depth {
		i := len(s.open) - 1
		s.r.write("</" + s.open[i].name + ">")
		s.open = s.open[:i]
	}
	s.depth--
}

func asciiLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func tagNameByte(c byte) bool {
	return asciiLetter(c) || c >= '0' && c <= '9' || c == '-'
}
func htmlSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' }

// parseTag finds the end outside quoted attribute values; an incomplete or
// malformed tag is emitted as text instead of being interpreted by a browser.
func parseTag(source string) (name string, closing, selfClosing bool, attrs string, end int) {
	if len(source) < 3 || source[0] != '<' {
		return
	}
	i := 1
	if source[i] == '/' {
		closing = true
		i++
	}
	if i >= len(source) || !asciiLetter(source[i]) {
		return "", false, false, "", 0
	}
	start := i
	for i < len(source) && tagNameByte(source[i]) {
		i++
	}
	name = strings.ToLower(source[start:i])
	if i == len(source) || !(htmlSpace(source[i]) || source[i] == '>' || source[i] == '/') {
		return "", false, false, "", 0
	}
	attrStart := i
	var quote byte
	for ; i < len(source); i++ {
		c := source[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			quote = c
		} else if c == '>' {
			attrs = source[attrStart:i]
			selfClosing = strings.HasSuffix(strings.TrimRightFunc(attrs, unicode.IsSpace), "/")
			return name, closing, selfClosing, attrs, i + 1
		}
	}
	return "", false, false, "", 0
}

func attributeNameByte(c byte) bool {
	return asciiLetter(c) || c >= '0' && c <= '9' || c == '-' || c == '_' || c == ':'
}

func (s *rawHTML) attributes(tag, source string) {
	seen := map[string]bool{}
	for i := 0; i < len(source); {
		for i < len(source) && (htmlSpace(source[i]) || source[i] == '/') {
			i++
		}
		start := i
		for i < len(source) && attributeNameByte(source[i]) {
			i++
		}
		if start == i {
			i++
			continue
		}
		name := strings.ToLower(source[start:i])
		for i < len(source) && htmlSpace(source[i]) {
			i++
		}
		value := ""
		if i < len(source) && source[i] == '=' {
			i++
			for i < len(source) && htmlSpace(source[i]) {
				i++
			}
			if i < len(source) && (source[i] == '\'' || source[i] == '"') {
				quote := source[i]
				i++
				start = i
				for i < len(source) && source[i] != quote {
					i++
				}
				value = source[start:i]
				if i < len(source) {
					i++
				}
			} else {
				start = i
				for i < len(source) && !htmlSpace(source[i]) {
					i++
				}
				value = source[start:i]
			}
		}
		if seen[name] || !safeAttribute(name) {
			continue
		}
		seen[name] = true
		value = html.UnescapeString(value)
		if name == "href" || name == "src" {
			value = safeURL(value, name == "src")
			if name == "src" && tag == "img" {
				value = s.r.imageURL(value)
			}
		}
		s.r.write(" " + name + "=\"")
		s.r.escaped(value)
		s.r.write("\"")
	}
}

func safeAttribute(name string) bool {
	if strings.HasPrefix(name, "on") {
		return false
	}
	switch name {
	case "href", "src", "alt", "title", "id", "class", "width", "height", "align",
		"colspan", "rowspan", "scope", "open", "start", "reversed", "role":
		return true
	}
	return strings.HasPrefix(name, "aria-") || strings.HasPrefix(name, "data-")
}

// Browsers ignore control whitespace in schemes. Decode entities and percent
// escapes for the check as well, while leaving benign URLs unchanged.
func safeURL(value string, image bool) string {
	check := value
	for range 3 {
		check = html.UnescapeString(check)
		if decoded, err := url.PathUnescape(check); err == nil {
			check = decoded
		}
	}
	check = strings.ToLower(strings.Map(func(c rune) rune {
		if unicode.IsSpace(c) || unicode.IsControl(c) {
			return -1
		}
		return c
	}, check))
	if strings.HasPrefix(check, "javascript:") || strings.HasPrefix(check, "vbscript:") ||
		strings.HasPrefix(check, "data:") && (!image || !strings.HasPrefix(check, "data:image/")) {
		return ""
	}
	return value
}
