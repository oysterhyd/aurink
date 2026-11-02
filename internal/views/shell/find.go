package shell

import (
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"aurink/internal/contract"
	"aurink/internal/views/editorapi"
	"aurink/internal/views/kit"
	"aurink/internal/views/motion"
	"aurink/internal/views/theme"
	"github.com/egoist/mygo/ui"
)

// findMax bounds the matches the bar shows and steps through, as MyGo's own
// finder does; Replace All replaces every one regardless.
const findMax = 100000

// findAsync is the size past which the live finder searches off the main
// thread, so that typing a query into a large document does not stall.
const findAsync = 512 << 10

// lfind is the live view's find bar (FE-2.5): the source view uses MyGo's.
type lfind struct {
	open, replacing         bool
	query, replace          string
	caseSens, whole, regex  bool
	focusQuery              bool
	matches                 []contract.Range
	current                 int
	err                     error
	ran                     contract.Query
	ranVer                  contract.Version
	searched, stale, follow bool
	gen                     int
	running                 bool
}

func (f *lfind) q() contract.Query {
	return contract.Query{Text: f.query, CaseSensitive: f.caseSens, WholeWord: f.whole, Regex: f.regex}
}

// openFind opens the find bar of a live document, its query seeded from a
// selection within one line.
func (s *Shell) openFind(d *doc, replace bool) {
	f := &d.find
	if sel := d.s.Model().Selection().Primary().Range(); !sel.Empty() {
		if text := d.s.Model().Text().Slice(sel); !strings.ContainsAny(text, "\r\n") {
			f.query = text
		}
	}
	f.open, f.focusQuery, f.stale, f.follow = true, true, true, true
	f.replacing = f.replacing || replace
	s.invalidate()
}

// findStep goes to the next or previous match, opening the bar first if it is
// closed.
func (s *Shell) findStep(d *doc, dir int) {
	if !d.find.open || d.find.query == "" {
		s.openFind(d, false)
		return
	}
	lf := &liveFinder{s: s, d: d}
	s.refreshFind(d)
	if dir < 0 {
		lf.Previous()
	} else {
		lf.Next()
	}
}

func (s *Shell) findBar(c *ui.Context, t *theme.Tokens, d *doc) {
	if !d.find.open {
		return
	}
	s.refreshFind(d)
	bar := ui.Column(c.Key("find")).Absolute().Top(12).Right(20).HandleInput(func(ui.InputEvent) bool { return false })
	if s.live {
		bar.Transition(ui.ElementTransition{Animation: &motion.Snappy, Enter: &ui.Motion{Y: -8, Opacity: 0}, Exit: &ui.Motion{Y: -8, Opacity: 0}})
	}
	kit.FindBar(c, t, bar, &liveFinder{s, d, c.Modifiers()}, s.live)
	// What the bar typed or toggled searches at once.
	s.refreshFind(d)
}

// refreshFind searches again when the query, its options or the text changed.
// A changed query selects the match at or after the caret, as it is typed.
func (s *Shell) refreshFind(d *doc) {
	f := &d.find
	m := d.s.Model()
	q := f.q()
	if q != f.ran {
		f.stale, f.follow = true, true
	}
	if !f.stale && f.ranVer == m.Version() || f.running {
		return
	}
	f.stale = false
	f.ran, f.ranVer = q, m.Version()
	f.gen++
	if q.Text == "" || s.o.Finder == nil {
		f.matches, f.err, f.searched = nil, nil, true
		s.showFind(d, -1)
		return
	}
	text := m.Text()
	if text.Len() < findAsync || s.o.Dispatcher == nil {
		found, err := s.o.Finder.Find(text, q, findMax)
		s.found(d, found, err)
		return
	}
	gen, finder, post := f.gen, s.o.Finder, s.o.Dispatcher.Post
	f.running = true
	go func() {
		found, err := finder.Find(text, q, findMax)
		post(func() {
			f.running = false
			if gen == f.gen {
				s.found(d, found, err)
			}
			s.invalidate()
		})
	}()
}

func (s *Shell) found(d *doc, found []contract.Match, err error) {
	f := &d.find
	f.err, f.searched = err, true
	f.matches = f.matches[:0]
	for _, m := range found {
		f.matches = append(f.matches, m.Range)
	}
	sel := d.s.Model().Selection().Primary().Range()
	cur := -1
	for i, m := range f.matches {
		if m == sel {
			cur = i
			break
		}
	}
	if f.follow && len(f.matches) > 0 {
		f.follow = false
		if cur < 0 {
			cur = nearest(f.matches, sel.Start)
		}
		f.current = cur
		d.live.RevealRange(f.matches[cur], editorapi.AlignCenter)
	} else {
		f.follow = f.follow && len(f.matches) == 0
	}
	s.showFind(d, cur)
}

func (s *Shell) showFind(d *doc, cur int) {
	d.find.current = cur
	d.live.SetFindHighlights(d.find.matches, cur)
}

// nearest returns the first match at or after off, round to the first.
func nearest(ms []contract.Range, off int) int {
	i := sort.Search(len(ms), func(i int) bool { return ms[i].Start >= off })
	if i == len(ms) {
		return 0
	}
	return i
}

// liveFinder is the live document's find bar as kit.FindBar shows it.
type liveFinder struct {
	s    *Shell
	d    *doc
	mods ui.Modifiers // held as the bar was built, for Shift+Enter and Primary+Enter
}

func (l *liveFinder) Query() *string       { return &l.d.find.query }
func (l *liveFinder) Replacement() *string { return &l.d.find.replace }
func (l *liveFinder) CaseSensitive() *bool { return &l.d.find.caseSens }
func (l *liveFinder) WholeWord() *bool     { return &l.d.find.whole }
func (l *liveFinder) Regexp() *bool        { return &l.d.find.regex }
func (l *liveFinder) Replacing() bool      { return l.d.find.replacing && l.CanReplace() }
func (l *liveFinder) SetReplacing(on bool) { l.d.find.replacing = on }
func (l *liveFinder) CanReplace() bool     { return !l.d.s.Model().ReadOnly() }
func (l *liveFinder) Err() error           { return l.d.find.err }

func (l *liveFinder) Matches() (int, int) {
	if len(l.d.find.matches) == 0 {
		return 0, -1
	}
	return len(l.d.find.matches), l.d.find.current
}

func (l *liveFinder) Next()     { l.step(1) }
func (l *liveFinder) Previous() { l.step(-1) }

// step goes to the match after the selection, or before it, round the ends.
func (l *liveFinder) step(dir int) {
	f := &l.d.find
	n := len(f.matches)
	if n == 0 {
		return
	}
	sel := l.d.s.Model().Selection().Primary().Range()
	var i int
	switch {
	case f.current >= 0 && f.current < n && f.matches[f.current] == sel:
		i = (f.current + dir + n) % n
	case dir > 0:
		i = nearest(f.matches, sel.End)
	default:
		i = sort.Search(n, func(i int) bool { return f.matches[i].End > sel.Start }) - 1
		if i < 0 {
			i = n - 1
		}
	}
	l.s.showFind(l.d, i)
	l.d.live.RevealRange(f.matches[i], editorapi.AlignCenter)
	l.s.invalidate()
}

// Replace replaces the current match when it is selected, and goes to the
// next; otherwise it goes to the next match first.
func (l *liveFinder) Replace() {
	f := &l.d.find
	m := l.d.s.Model()
	sel := m.Selection().Primary().Range()
	if !l.CanReplace() || f.current < 0 || f.current >= len(f.matches) || f.matches[f.current] != sel {
		l.Next()
		return
	}
	repl := l.replacements([]contract.Range{sel})[0]
	after := contract.Caret(sel.Start + len(repl))
	if _, err := m.Apply(contract.Transaction{Edits: []contract.TextEdit{{Range: sel, Text: repl}},
		Selection: &after, Origin: contract.OriginCommand}); err != nil {
		l.s.fail(err)
		return
	}
	f.stale = true
	l.s.refreshFind(l.d)
	if len(f.matches) > 0 {
		i := nearest(f.matches, sel.Start+len(repl))
		l.s.showFind(l.d, i)
		l.d.live.RevealRange(f.matches[i], editorapi.AlignCenter)
	}
	l.s.invalidate()
}

// ReplaceAll replaces every match, however many, as one step of undo.
func (l *liveFinder) ReplaceAll() {
	f := &l.d.find
	m := l.d.s.Model()
	if !l.CanReplace() || f.query == "" || l.s.o.Finder == nil {
		return
	}
	all, err := l.s.o.Finder.Find(m.Text(), f.q(), 0)
	if err != nil || len(all) == 0 {
		return
	}
	rs := make([]contract.Range, len(all))
	for i, x := range all {
		rs[i] = x.Range
	}
	repls := l.replacements(rs)
	edits := make([]contract.TextEdit, len(rs))
	for i, r := range rs {
		edits[i] = contract.TextEdit{Range: r, Text: repls[i]}
	}
	if _, err := m.Apply(contract.Transaction{Edits: edits, Origin: contract.OriginCommand}); err != nil {
		l.s.fail(err)
		return
	}
	f.stale = true
	l.s.refreshFind(l.d)
	l.s.invalidate()
}

// replacements returns what each match becomes: the replacement as typed, or
// for a regular expression with $1 and ${name} expanded from the match.
func (l *liveFinder) replacements(rs []contract.Range) []string {
	f := &l.d.find
	out := make([]string, len(rs))
	for i := range out {
		out[i] = f.replace
	}
	if !f.regex || !strings.Contains(f.replace, "$") {
		return out
	}
	expr := f.query
	if !f.caseSens {
		expr = "(?i)" + expr
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return out
	}
	text := l.d.s.Model().Text().String()
	// The submatches of the expression over the whole text, as \b and ^ need
	// the text around a match.
	subs := map[int][]int{}
	for _, idx := range re.FindAllStringSubmatchIndex(text, -1) {
		subs[idx[0]] = idx
	}
	for i, r := range rs {
		if idx, ok := subs[r.Start]; ok && idx[1] == r.End {
			out[i] = string(re.ExpandString(nil, f.replace, text, idx))
		}
	}
	return out
}

func (l *liveFinder) Close() {
	f := &l.d.find
	f.open, f.matches, f.current = false, nil, -1
	l.d.live.ClearFindHighlights()
	l.s.focusNext = true
	l.s.invalidate()
}

func (l *liveFinder) QueryInput(in ui.Element) ui.Element {
	f := &l.d.find
	if f.focusQuery {
		f.focusQuery = false
		in.Focus().SetTextSelection(0, utf8.RuneCountInString(f.query))
	}
	switch {
	case in.Submitted() && l.mods&ui.Shift != 0:
		l.Previous()
	case in.Submitted():
		l.Next()
	}
	return l.escape(in)
}

func (l *liveFinder) ReplacementInput(in ui.Element) ui.Element {
	switch {
	case in.Submitted() && l.mods&ui.Cmd != 0:
		l.ReplaceAll()
	case in.Submitted():
		l.Replace()
	}
	return l.escape(in)
}

// escape closes the bar on Escape, which a text field takes as its own key.
func (l *liveFinder) escape(in ui.Element) ui.Element {
	return in.OnTextKey(func(k ui.TextKey) bool {
		if k.Key != ui.KeyEscape || k.Mods != 0 {
			return false
		}
		l.Close()
		return true
	})
}

// SetFind types a query into the live find bar of the active tab, as tools
// and tests do.
func (s *Shell) SetFind(q string) {
	if d := s.active; d != nil && d.find.open {
		d.find.query = q
		s.invalidate()
	}
}

// CloseFind closes the live find bar of the active tab, as Escape does.
func (s *Shell) CloseFind() {
	if d := s.active; d != nil && d.find.open {
		(&liveFinder{s: s, d: d}).Close()
	}
}
