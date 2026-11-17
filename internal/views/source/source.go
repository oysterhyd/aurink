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
	snap     editor.Snapshot  // the buffer's text at the last sync
	version  uint64           // the buffer's version at the last sync
	docVer   contract.Version // the document's version at the last sync
	crlf     []int            // mirror offsets of newlines that are \r\n in the document
	local    []int            // scratch for incremental crlf updates
	chunksA  []string         // scratch for snapSpan
	chunksB  []string         // scratch for snapSpan
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
	large := m.Meta().Large
	buf := editor.NewBuffer(text)
	if !large {
		buf.SetLanguage(editor.LanguageFor("a.md"))
	}
	e.snap = buf.Snapshot()
	e.version, e.docVer = e.snap.Version(), m.Version()
	e.st = &editor.State{Buffer: buf, SoftWrap: !large, NoAutoClose: true, ReadOnly: m.ReadOnly(),
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
		switch ev.Kind {
		case contract.DocReadOnlyChanged:
			e.st.ReadOnly = m.ReadOnly()
		case contract.DocMetaChanged:
			meta := m.Meta()
			e.dominant = meta.Dominant
			e.st.SoftWrap = !meta.Large
			if meta.Large {
				e.st.Buffer.SetLanguage(nil)
			} else {
				e.st.Buffer.SetLanguage(editor.LanguageFor("a.md"))
			}
		case contract.DocTextChanged, contract.DocReplaced:
			if !e.writing {
				e.pull()
			}
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
	if e.model.Version() == e.docVer && e.st.Buffer.Version() == e.version {
		return
	}
	text := e.model.Text().String()
	e.index(text)
	want := lf(text)
	cur := e.st.Buffer.String()
	if want != cur {
		from, oldEnd, newEnd := span(cur, want)
		e.st.Buffer.Edit(editor.Edit{Start: from, End: oldEnd, Text: want[from:newEnd]})
	}
	e.snap = e.st.Buffer.Snapshot()
	e.version, e.docVer = e.snap.Version(), e.model.Version()
}

// Sync writes the buffer's edits to the document.
func (e *Editor) Sync() {
	now := e.st.Buffer.Snapshot()
	if now.Version() == e.version {
		return
	}
	e.version = now.Version()
	from, oldEnd, newEnd, changed := e.snapSpan(e.snap, now)
	if !changed {
		e.snap = now
		return
	}
	edits, from, oldEnd, newEnd := e.diff(e.snap, now, from, oldEnd, newEnd)
	e.writing = true
	_, err := e.model.Apply(contract.Transaction{Edits: edits, Origin: contract.OriginCommand, Label: "editing_undo_typing"})
	e.writing = false
	if err != nil {
		// The document refused: it stays the truth.
		e.docVer = 0
		e.pull()
		return
	}
	oldSnap := e.snap
	e.snap = now
	e.docVer = e.model.Version()
	e.updateCRLF(oldSnap, now, from, oldEnd, newEnd)
}

// snapSpan skips shared rope chunks from both ends before comparing bytes in the
// edited chunk range, keeping Sync O(edited chunk) on multi-megabyte buffers.
func (e *Editor) snapSpan(a, b editor.Snapshot) (from, oldEnd, newEnd int, changed bool) {
	e.chunksA = e.chunksA[:0]
	a.Chunks(0, a.Len(), func(s string) bool { e.chunksA = append(e.chunksA, s); return true })
	e.chunksB = e.chunksB[:0]
	b.Chunks(0, b.Len(), func(s string) bool { e.chunksB = append(e.chunksB, s); return true })
	ia, ib := 0, 0
	for ia < len(e.chunksA) && ib < len(e.chunksB) && e.chunksA[ia] == e.chunksB[ib] {
		from += len(e.chunksA[ia])
		ia++
		ib++
	}
	if ia == len(e.chunksA) && ib == len(e.chunksB) {
		return 0, 0, 0, false
	}
	ja, jb := len(e.chunksA)-1, len(e.chunksB)-1
	tail := 0
	for ja >= ia && jb >= ib && e.chunksA[ja] == e.chunksB[jb] {
		tail += len(e.chunksA[ja])
		ja--
		jb--
	}
	midA := a.Slice(from, a.Len()-tail)
	midB := b.Slice(from, b.Len()-tail)
	f, oe, ne := span(midA, midB)
	if f == oe && f == ne {
		return 0, 0, 0, false
	}
	return from + f, from + oe, from + ne, true
}

// updateCRLF splices e.crlf over the edited mirror span instead of rescanning
// the full document text on every keystroke (09 D4).
func (e *Editor) updateCRLF(oldSnap, newSnap editor.Snapshot, from, oldEnd, newEnd int) {
	docText := e.model.Text()
	if docText.Len() == newSnap.Len() {
		e.crlf = e.crlf[:0]
		return
	}
	mLo := max(0, from-2)
	oldMHi := min(oldSnap.Len(), oldEnd+2)
	mHi := newEnd + (oldMHi - oldEnd)
	loIdx := sort.SearchInts(e.crlf, mLo)
	hiIdx := sort.SearchInts(e.crlf, oldMHi)
	tailCRLF := len(e.crlf) - hiIdx
	docLo := mLo + loIdx
	docHi := docText.Len() - (newSnap.Len() - mHi) - tailCRLF
	if docLo < 0 || docHi < docLo || docHi > docText.Len() {
		e.index(docText.String())
		return
	}
	sub := docText.Slice(contract.Range{Start: docLo, End: docHi})
	e.local = e.local[:0]
	removed := 0
	for at := 0; ; {
		i := strings.Index(sub[at:], "\r\n")
		if i < 0 {
			break
		}
		pos := at + i
		e.local = append(e.local, mLo+pos-removed)
		removed++
		at = pos + 2
	}
	mirrorDelta := newSnap.Len() - oldSnap.Len()
	for i := hiIdx; i < len(e.crlf); i++ {
		e.crlf[i] += mirrorDelta
	}
	e.crlf = slices.Replace(e.crlf, loIdx, hiIdx, e.local...)
	if newSnap.Len()+len(e.crlf) != docText.Len() {
		e.docVer = 0
		e.pull()
	}
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
func (e *Editor) diff(old, now editor.Snapshot, from, oldEnd, newEnd int) ([]contract.TextEdit, int, int, int) {
	if !strings.Contains(old.Slice(from, oldEnd), "\n") {
		return []contract.TextEdit{e.edit(from, oldEnd, now.Slice(from, newEnd))}, from, oldEnd, newEnd
	}
	// Widen to whole lines and compare those.
	from = old.LineStart(old.LineOf(from))
	// The tails left out must be the same text.
	tail := min(old.Len()-wholeLine(old, oldEnd), now.Len()-wholeLine(now, newEnd))
	oldEnd, newEnd = old.Len()-tail, now.Len()-tail
	oldPart, newPart := old.Slice(from, oldEnd), now.Slice(from, newEnd)
	a, b := strings.SplitAfter(oldPart, "\n"), strings.SplitAfter(newPart, "\n")
	if len(a) > maxLineDiff || len(b) > maxLineDiff {
		return []contract.TextEdit{e.edit(from, oldEnd, newPart)}, from, oldEnd, newEnd
	}
	return e.lineEdits(from, a, b), from, oldEnd, newEnd
}

func wholeLine(s editor.Snapshot, off int) int {
	if off > 0 && s.Slice(off-1, off) == "\n" {
		return off
	}
	end := s.LineEnd(s.LineOf(off))
	if end < s.Len() {
		return end + 1
	}
	return s.Len()
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
