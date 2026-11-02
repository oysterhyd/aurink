package search

import (
	"context"
	"regexp"
	"regexp/syntax"
	"strings"
	"unicode"
	"unicode/utf8"

	"aurink/internal/contract"
)

// Finder searches immutable text snapshots.
type Finder struct{}

var _ contract.Finder = (*Finder)(nil)

// NewFinder returns a finder safe for concurrent use.
func NewFinder() *Finder { return &Finder{} }

type compiledQuery struct {
	query contract.Query
	re    *regexp.Regexp
	prev  *regexp.Regexp
	runes []rune
	first string
}

func compileQuery(q contract.Query) (compiledQuery, error) {
	c := compiledQuery{query: q}
	if q.Text == "" {
		return c, nil
	}
	if q.Regex {
		pattern := q.Text
		if !q.CaseSensitive {
			pattern = "(?i:" + pattern + ")"
		}
		var err error
		c.re, err = regexp.Compile(pattern)
		if err != nil {
			return compiledQuery{}, contract.E(contract.CodeInvalid, "search.Find", err)
		}
		parsed, err := syntax.Parse(pattern, syntax.Perl)
		if err != nil {
			return compiledQuery{}, contract.E(contract.CodeInvalid, "search.Find", err)
		}
		if q.WholeWord || nullable(parsed) {
			// Include the preceding rune so assertions in the query retain
			// their original line and word context after advancing.
			c.prev, err = regexp.Compile("(?s:.)(" + pattern + ")")
			if err != nil {
				return compiledQuery{}, contract.E(contract.CodeInvalid, "search.Find", err)
			}
		}
	} else if !q.CaseSensitive {
		c.runes = []rune(q.Text)
		first := c.runes[0]
		c.first = string(first)
		for r := unicode.SimpleFold(first); r != first; r = unicode.SimpleFold(r) {
			c.first += string(r)
		}
	}
	return c, nil
}

func nullable(re *syntax.Regexp) bool {
	switch re.Op {
	case syntax.OpEmptyMatch, syntax.OpBeginLine, syntax.OpEndLine, syntax.OpBeginText,
		syntax.OpEndText, syntax.OpWordBoundary, syntax.OpNoWordBoundary, syntax.OpStar, syntax.OpQuest:
		return true
	case syntax.OpCapture, syntax.OpPlus:
		return nullable(re.Sub[0])
	case syntax.OpRepeat:
		return re.Min == 0 || nullable(re.Sub[0])
	case syntax.OpConcat:
		for _, sub := range re.Sub {
			if !nullable(sub) {
				return false
			}
		}
		return true
	case syntax.OpAlternate:
		for _, sub := range re.Sub {
			if nullable(sub) {
				return true
			}
		}
	case syntax.OpLiteral:
		return len(re.Rune) == 0
	}
	return false
}

// Find returns non-overlapping matches in document order.
func (f *Finder) Find(t contract.Text, q contract.Query, limit int) ([]contract.Match, error) {
	c, err := compileQuery(q)
	if err != nil {
		return nil, err
	}
	return c.find(t.String(), limit), nil
}

func (c compiledQuery) find(src string, limit int) []contract.Match {
	return c.findContext(nil, src, limit)
}

// streamRegexp allows workspace workers to check cancellation between matches.
func (c *compiledQuery) streamRegexp() {
	if c.re != nil && c.prev == nil {
		c.prev, _ = regexp.Compile("(?s:.)(" + c.re.String() + ")")
	}
}

func (c compiledQuery) findContext(ctx context.Context, src string, limit int) []contract.Match {
	if c.query.Text == "" {
		return nil
	}
	var matches []contract.Match
	line, lineStart, scanned := 0, 0, 0
	add := func(start, end int) bool {
		if start == end || (c.query.WholeWord && !wholeWord(src, start, end)) {
			return false
		}
		part := src[scanned:start]
		line += strings.Count(part, "\n")
		if i := strings.LastIndexByte(part, '\n'); i >= 0 {
			lineStart = scanned + i + 1
		}
		scanned = start
		matches = append(matches, makeMatch(src, start, end, line, lineStart))
		return true
	}
	if c.re != nil {
		if c.prev == nil {
			// The expression cannot match empty text, so the result limit can
			// be passed directly to regexp without missing a reported match.
			n := -1
			if limit > 0 {
				n = limit
			}
			for _, r := range c.re.FindAllStringIndex(src, n) {
				if ctx != nil && ctx.Err() != nil {
					return nil
				}
				add(r[0], r[1])
			}
		} else {
			for offset := 0; offset <= len(src); {
				if ctx != nil && ctx.Err() != nil {
					return nil
				}
				start, end := c.nextRegexp(src, offset)
				if start < 0 {
					break
				}
				if add(start, end) {
					if limit > 0 && len(matches) >= limit {
						break
					}
					offset = end
				} else if start == len(src) {
					break
				} else {
					offset = nextCandidate(src, start, c.query.WholeWord && start != end)
				}
			}
		}
		return matches
	}
	for offset := 0; offset < len(src); {
		if ctx != nil && ctx.Err() != nil {
			return nil
		}
		start, end := c.nextLiteral(src, offset)
		if start < 0 {
			break
		}
		if add(start, end) {
			if limit > 0 && len(matches) >= limit {
				break
			}
			offset = end
		} else {
			// A rejected word may overlap a later valid word.
			offset = nextCandidate(src, start, c.query.WholeWord)
		}
	}
	return matches
}

func nextCandidate(src string, start int, wholeWord bool) int {
	for start < len(src) {
		r, n := utf8.DecodeRuneInString(src[start:])
		start += n
		// A later whole-word candidate cannot start after a word rune.
		if !wholeWord || !wordRune(r) {
			break
		}
	}
	return start
}

func (c compiledQuery) nextRegexp(src string, offset int) (int, int) {
	if offset == 0 {
		match := c.re.FindStringIndex(src)
		if match == nil {
			return -1, -1
		}
		return match[0], match[1]
	}
	_, n := utf8.DecodeLastRuneInString(src[:offset])
	base := offset - n
	match := c.prev.FindStringSubmatchIndex(src[base:])
	if match == nil {
		return -1, -1
	}
	return base + match[2], base + match[3]
}

func (c compiledQuery) nextLiteral(src string, offset int) (int, int) {
	if c.query.CaseSensitive {
		i := strings.Index(src[offset:], c.query.Text)
		if i < 0 {
			return -1, -1
		}
		start := offset + i
		return start, start + len(c.query.Text)
	}
	for offset < len(src) {
		i := strings.IndexAny(src[offset:], c.first)
		if i < 0 {
			return -1, -1
		}
		start := offset + i
		end := start
		matched := true
		for _, want := range c.runes {
			if end == len(src) {
				matched = false
				break
			}
			r, n := utf8.DecodeRuneInString(src[end:])
			if !equalFold(r, want) {
				matched = false
				break
			}
			end += n
		}
		if matched {
			return start, end
		}
		_, n := utf8.DecodeRuneInString(src[start:])
		offset = start + n
	}
	return -1, -1
}

func equalFold(a, b rune) bool {
	if a == b {
		return true
	}
	if a < utf8.RuneSelf && b < utf8.RuneSelf {
		if 'a' <= a && a <= 'z' {
			a -= 'a' - 'A'
		}
		if 'a' <= b && b <= 'z' {
			b -= 'a' - 'A'
		}
		return a == b
	}
	for r := unicode.SimpleFold(a); r != a; r = unicode.SimpleFold(r) {
		if r == b {
			return true
		}
	}
	return false
}

func wholeWord(src string, start, end int) bool {
	if start > 0 {
		r, _ := utf8.DecodeLastRuneInString(src[:start])
		if wordRune(r) {
			return false
		}
	}
	if end < len(src) {
		r, _ := utf8.DecodeRuneInString(src[end:])
		if wordRune(r) {
			return false
		}
	}
	return true
}

func wordRune(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }

func makeMatch(src string, start, end, line, lineStart int) contract.Match {
	lineEnd := len(src)
	if i := strings.IndexByte(src[start:], '\n'); i >= 0 {
		lineEnd = start + i
		if lineEnd > lineStart && src[lineEnd-1] == '\r' {
			lineEnd--
		}
	}
	hitStart := min(start, lineEnd) - lineStart
	hitEnd := min(end, lineEnd) - lineStart
	lineText := src[lineStart:lineEnd]
	left, right := 0, len(lineText)
	if right > contract.PreviewMax {
		visibleHit := min(hitEnd-hitStart, contract.PreviewMax)
		left = max(0, hitStart-(contract.PreviewMax-visibleHit)/2)
		right = min(len(lineText), left+contract.PreviewMax)
		left = max(0, right-contract.PreviewMax)
		for left < right && !utf8.RuneStart(lineText[left]) {
			left++
		}
		for right > left && right < len(lineText) && !utf8.RuneStart(lineText[right]) {
			right--
		}
	}
	preview := strings.Clone(lineText[left:right])
	prefix := 0
	if left > 0 {
		preview = "…" + preview
		prefix = len("…")
	}
	if right < len(lineText) {
		preview += "…"
	}
	return contract.Match{
		Range:   contract.Range{Start: start, End: end},
		Line:    line,
		Preview: preview,
		Hit: contract.Range{
			Start: prefix + min(max(hitStart-left, 0), right-left),
			End:   prefix + min(max(hitEnd-left, 0), right-left),
		},
	}
}
