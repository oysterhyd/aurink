// Package source is the source view: the mygo code editor over a mirror of the
// document (06 FE-1.3).
//
// editor.Buffer keeps \n line endings whatever the text had, so the mirror is not
// the document's bytes. eol knows which of its newlines are \r\n in the document,
// which maps offsets, and an edit writes back only the lines it changed. The document
// keeps the history: undo here is the document's, the same as in the live view.
package source

import (
	"slices"
	"sort"
	"strings"

	"aurink/internal/contract"
	"aurink/internal/views/kit"
	"aurink/internal/views/theme"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// maxLineDiff bounds the lines an edit compares; beyond it the whole span is replaced.
const maxLineDiff = 2048

// Editor is one document's source view.
type Editor struct {
	model    contract.DocumentModel
	st       *editor.State
	mirror   string // the buffer's text at the last sync
	version  uint64 // the buffer's version at the last sync
	crlf     []int  // mirror offsets of newlines that are \r\n in the document
	dominant string
	writing  bool
	off      func()
	t        *theme.Tokens
	motion   bool
}

// New returns the source view of a document.
func New(m contract.DocumentModel, t *theme.Tokens) *Editor {
	e := &Editor{model: m}
	text := m.Text().String()
	e.index(text)
	buf := editor.NewBuffer(text)
	buf.SetLanguage(editor.LanguageFor("a.md"))
	e.mirror, e.version = buf.String(), buf.Version()
	e.st = &editor.State{Buffer: buf, SoftWrap: true, NoAutoClose: true, ReadOnly: m.ReadOnly(),
		Font: ui.Font{Family: theme.Mono, Size: 13}, LineHeight: 1.6, OnInput: e.input}
	e.dominant = m.Meta().Dominant
	e.SetTheme(t)
	// MyGo's finder with Aurink's bar (02 §5.14, D16): its matches are drawn in
	// the text by the editor itself.
	e.st.RenderFindBar = func(c *ui.Context, bar ui.Element, f *editor.FindBar) {
		bar.Top(12).Right(20)
		kit.FindBar(c, e.t, bar, f, e.motion)
	}
	e.off = m.Subscribe(func(ev contract.DocEvent) {
		if ev.Kind != contract.DocSelectionChanged && !e.writing {
			e.pull()
		}
	})
	return e
}

// Close stops following the document.
func (e *Editor) Close() { e.off() }

// State returns the editor's state, for tests and the find bar.
func (e *Editor) State() *editor.State { return e.st }

// SetMotion lets the find bar animate; off while motion is reduced.
func (e *Editor) SetMotion(on bool) { e.motion = on }

// Find opens the find bar, with the replacement row when replace is set.
func (e *Editor) Find(replace bool) {
	if replace {
		e.st.Run(editor.FindReplace)
	} else {
		e.st.Run(editor.Find)
	}
}

// FindStep goes to the next match, or the previous for a negative dir.
func (e *Editor) FindStep(dir int) {
	if dir < 0 {
		e.st.Run(editor.FindPrevious)
	} else {
		e.st.Run(editor.FindNext)
	}
}

// SetTheme colors the editor from the tokens.
func (e *Editor) SetTheme(t *theme.Tokens) {
	e.t = t
	th := &editor.Theme{Background: t.Sheet, Text: t.Text, LineNumber: t.Faint, ActiveLineNumber: t.Muted,
		ActiveLine: t.Surface, Selection: t.Selection, InactiveSelection: t.Selection.Alpha(.5), Cursor: t.Accent,
		MatchingBracket: t.Border, Match: t.Mark, CurrentMatch: t.Warning.Alpha(.6), Occurrence: t.Surface,
		IndentGuide: t.Border, ActiveIndentGuide: t.Border, WrapGuide: t.Border, Whitespace: t.Faint,
		Error: t.Danger, Warning: t.Warning, Info: t.Accent, Hint: t.Muted, Added: t.Success, Deleted: t.Danger, Modified: t.Warning}
	th.Tokens[editor.TokenKeyword].Color = t.SynKeyword
	th.Tokens[editor.TokenFunction].Color = t.SynFunc
	th.Tokens[editor.TokenComment].Color = t.SynComment
	th.Tokens[editor.TokenString].Color = t.SynString
	e.st.Theme = th
}

// View builds the editor. An edit reaches the buffer while input is handled, so the
// frame after it writes back; Sync does it sooner for those who need it.
func (e *Editor) View(c *ui.Context) ui.Element {
	e.Sync()
	return editor.View(c, e.st).Fill()
}

// Enter shows the document's selection, scrolled to a source offset.
func (e *Editor) Enter(top int) {
	e.pull()
	sel := e.model.Selection().Primary()
	e.st.Select(e.toMirror(sel.Anchor), e.toMirror(sel.Head))
	e.st.ScrollTo(e.st.Buffer.Pos(e.toMirror(top)).Line)
}

// Leave writes back and gives the document this view's selection. It returns the
// source offset at the top of the view.
func (e *Editor) Leave() int {
	e.Sync()
	s := e.st.Selection()
	e.model.SetSelection(contract.Selection{Ranges: []contract.SelRange{{Anchor: e.toDoc(s.Anchor), Head: e.toDoc(s.Head)}}})
	first, _ := e.st.VisibleLines()
	return e.toDoc(e.st.Buffer.Offset(editor.Pos{Line: first}))
}

// input gives undo to the document.
func (e *Editor) input(ev ui.InputEvent) bool {
	if ev.Kind != ui.InputKeyDown || ev.Mods&ui.Cmd == 0 {
		return false
	}
	switch {
	case ev.Key == ui.KeyZ && ev.Mods&ui.Shift == 0:
		e.Sync()
		e.model.Undo()
	case ev.Key == ui.KeyZ || (ev.Key == ui.KeyY && ev.Mods&ui.Shift == 0):
		e.Sync()
		e.model.Redo()
	default:
		return false
	}
	return true
}

// index records which newlines of the document's text are \r\n.
func (e *Editor) index(text string) {
	e.crlf = e.crlf[:0]
	removed := 0
	for from := 0; ; {
		i := strings.Index(text[from:], "\r\n")
		if i < 0 {
			return
		}
		at := from + i
		e.crlf = append(e.crlf, at+1-removed-1)
		removed++
		from = at + 2
	}
}

// toDoc maps an offset of the mirror to one of the document. The \r of a \r\n comes
// before its newline, so an offset at the newline stays before the \r.
func (e *Editor) toDoc(off int) int { return off + sort.SearchInts(e.crlf, off) }

func (e *Editor) toMirror(off int) int {
	// The document offset of the k-th \r is crlf[k]+k.
	k := sort.Search(len(e.crlf), func(k int) bool { return e.crlf[k]+k >= off })
	return off - k
}

func lf(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }

// pull brings the buffer to the document's text, when something else changed it.
func (e *Editor) pull() {
	text := e.model.Text().String()
	e.index(text)
	want := lf(text)
	if want == e.mirror {
		return
	}
	from, oldEnd, newEnd := span(e.mirror, want)
	e.st.Buffer.Edit(editor.Edit{Start: from, End: oldEnd, Text: want[from:newEnd]})
	e.mirror, e.version = want, e.st.Buffer.Version()
}

// Sync writes the buffer's edits to the document.
func (e *Editor) Sync() {
	v := e.st.Buffer.Version()
	if v == e.version {
		return
	}
	e.version = v
	now := e.st.Buffer.String()
	if now == e.mirror {
		return
	}
	edits := e.diff(e.mirror, now)
	e.writing = true
	_, err := e.model.Apply(contract.Transaction{Edits: edits, Origin: contract.OriginCommand, Label: "editing_undo_typing"})
	e.writing = false
	if err != nil {
		// The document refused: it stays the truth.
		e.pull()
		return
	}
	e.mirror = now
	e.index(e.model.Text().String())
}

// span returns the part of two texts that differs: both start differing at from, and
// are the same again from oldEnd in a and newEnd in b.
func span(a, b string) (from, oldEnd, newEnd int) {
	for from < len(a) && from < len(b) && a[from] == b[from] {
		from++
	}
	oldEnd, newEnd = len(a), len(b)
	for oldEnd > from && newEnd > from && a[oldEnd-1] == b[newEnd-1] {
		oldEnd--
		newEnd--
	}
	return from, oldEnd, newEnd
}

// diff turns a change of the mirror into edits of the document that leave every line
// it did not touch as it was, with its own line ending.
func (e *Editor) diff(old, now string) []contract.TextEdit {
	from, oldEnd, newEnd := span(old, now)
	if !strings.Contains(old[from:oldEnd], "\n") {
		return []contract.TextEdit{e.edit(from, oldEnd, now[from:newEnd])}
	}
	// Widen to whole lines and compare those.
	from = strings.LastIndexByte(old[:from], '\n') + 1
	// The tails left out must be the same text.
	tail := min(len(old)-wholeLine(old, oldEnd), len(now)-wholeLine(now, newEnd))
	oldEnd, newEnd = len(old)-tail, len(now)-tail
	a, b := strings.SplitAfter(old[from:oldEnd], "\n"), strings.SplitAfter(now[from:newEnd], "\n")
	if len(a) > maxLineDiff || len(b) > maxLineDiff {
		return []contract.TextEdit{e.edit(from, oldEnd, now[from:newEnd])}
	}
	return e.lineEdits(from, a, b)
}

func wholeLine(s string, off int) int {
	if off > 0 && s[off-1] == '\n' {
		return off
	}
	if i := strings.IndexByte(s[off:], '\n'); i >= 0 {
		return off + i + 1
	}
	return len(s)
}

// edit replaces a span of the mirror. New newlines take the document's usual ending.
func (e *Editor) edit(from, to int, text string) contract.TextEdit {
	if e.dominant == "\r\n" {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	end := e.toDoc(to)
	// A span ending just after a newline ends after its \r\n, which toDoc covers: the
	// \r lies before the newline's offset, inside it.
	return contract.TextEdit{Range: contract.Range{Start: e.toDoc(from), End: end}, Text: text}
}

// lineEdits compares lines by longest common subsequence.
func (e *Editor) lineEdits(base int, a, b []string) []contract.TextEdit {
	n, m := len(a), len(b)
	lcs := make([]int32, (n+1)*(m+1))
	at := func(i, j int) *int32 { return &lcs[i*(m+1)+j] }
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				*at(i, j) = *at(i+1, j+1) + 1
			} else {
				*at(i, j) = max(*at(i+1, j), *at(i, j+1))
			}
		}
	}
	var edits []contract.TextEdit
	off := base
	var gone, came []string
	// flush edits a run of changed lines: line by line when their count held, which
	// keeps the endings of lines changed inside, else as one piece.
	flush := func() {
		at := off
		for _, l := range gone {
			at -= len(l)
		}
		if len(gone) == len(came) {
			for k := range gone {
				if f, oe, ne := span(gone[k], came[k]); f != oe || f != ne {
					edits = append(edits, e.edit(at+f, at+oe, came[k][f:ne]))
				}
				at += len(gone[k])
			}
		} else {
			o, t := strings.Join(gone, ""), strings.Join(came, "")
			f, oe, ne := span(o, t)
			edits = append(edits, e.edit(at+f, at+oe, t[f:ne]))
		}
		gone, came = gone[:0], came[:0]
	}
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && j < m && a[i] == b[j]:
			flush()
			off += len(a[i])
			i++
			j++
		case j < m && (i == n || *at(i, j+1) >= *at(i+1, j)):
			came = append(came, b[j])
			j++
		default:
			gone = append(gone, a[i])
			off += len(a[i])
			i++
		}
	}
	flush()
	return slices.Clip(edits)
}
