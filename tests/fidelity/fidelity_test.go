// Package fidelity checks the whole read, edit and write path against bytes.
package fidelity

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"

	"aurink/internal/contract"
	"aurink/internal/core/docsession"
	"aurink/internal/core/document"
	"aurink/internal/core/fsstore"
	"aurink/internal/core/settings"
	"aurink/tests/corpus"
)

type queue chan func()

func (q queue) Post(fn func()) { q <- fn }

type rig struct {
	q queue
	m *docsession.Manager
}

func newRig(t *testing.T) *rig {
	t.Helper()
	q := make(queue, 64)
	value := settings.Defaults()
	value.Files.Autosave = false
	value.Files.LargeFileBytes = 256 << 20
	return &rig{q, docsession.New(fsstore.New(), document.Factory{}, q, "", docsession.Options{Settings: &value})}
}

// wait runs main-thread work until done has been called.
func (r *rig) wait(finished *bool) {
	for !*finished {
		(<-r.q)()
	}
}

func (r *rig) open(t *testing.T, path string) contract.Session {
	t.Helper()
	var s contract.Session
	finished := false
	r.m.Open(path, func(x contract.Session, err error) {
		if err != nil {
			t.Fatalf("open %s: %v", path, err)
		}
		s, finished = x, true
	})
	r.wait(&finished)
	return s
}

func (r *rig) save(t *testing.T, s contract.Session) {
	t.Helper()
	finished := false
	s.Save(func(err error) {
		if err != nil {
			t.Fatalf("save: %v", err)
		}
		finished = true
	})
	r.wait(&finished)
}

func corpusFiles(t *testing.T) []corpus.Entry {
	t.Helper()
	entries, err := corpus.Index()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("empty corpus")
	}
	return entries
}

// TestCorpusOpenSaveIsByteIdentical is the first line of 05's QA-1.1 card.
func TestCorpusOpenSaveIsByteIdentical(t *testing.T) {
	dir := t.TempDir()
	n := 0
	for _, e := range corpusFiles(t) {
		data, err := os.ReadFile(filepath.Join(corpus.Root(), filepath.FromSlash(e.Path)))
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, fmt.Sprintf("%d.md", n))
		n++
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		r := newRig(t)
		s := r.open(t, path)
		r.save(t, s)
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, data) {
			t.Errorf("%s: saved %d bytes differ from %d bytes at %d", e.Path, len(got), len(data), firstDiff(got, data))
		}
		s.Close(true)
	}
	t.Logf("%d files", n)
}

func firstDiff(a, b []byte) int {
	for i := range min(len(a), len(b)) {
		if a[i] != b[i] {
			return i
		}
	}
	return min(len(a), len(b))
}

// step is one operation in fractions of the document, so that a smaller script
// still runs when failures are minimized.
type step struct {
	op         int // 0 edit, 1 undo, 2 redo, 3 save and compare
	from, span float64
	text       string
}

var inserts = []string{"x", "中", "😀", "\n", "\r\n", "\n\n", "# ", "- ", "> ", "**", "*", "`", "```\n", "| a |\n|---|\n", "[a](b)", "[^n]", "\\", "&amp;", " ", "    ", "\t", ""}

func script(seed uint64, n int) []step {
	rng := rand.New(rand.NewPCG(seed, seed^0x5bd1))
	steps := make([]step, n)
	for i := range steps {
		s := step{from: rng.Float64(), text: inserts[rng.IntN(len(inserts))]}
		switch k := rng.IntN(100); {
		case k < 74:
			if rng.IntN(3) == 0 {
				s.span = rng.Float64() * rng.Float64() * 0.05
			}
		case k < 86:
			s.op = 1
		case k < 94:
			s.op = 2
		default:
			s.op = 3
		}
		steps[i] = s
	}
	return steps
}

func runeStart(s string, i int) int {
	i = max(0, min(i, len(s)))
	for i > 0 && i < len(s) && !utf8.RuneStart(s[i]) {
		i--
	}
	return i
}

type undoStep struct {
	at       int
	old, new string
}

// run replays a script against a real session and a plain string. It returns the
// first difference, in words, or "".
func run(t *testing.T, r *rig, path, start string, encode func(string) []byte, steps []step) string {
	t.Helper()
	if err := os.WriteFile(path, encode(start), 0o644); err != nil {
		t.Fatal(err)
	}
	s := r.open(t, path)
	defer s.Close(true)
	m := s.Model()
	model := start
	var undo, redo []undoStep
	for i, st := range steps {
		switch st.op {
		case 0:
			a := runeStart(model, int(st.from*float64(len(model))))
			z := runeStart(model, a+int(st.span*float64(len(model))))
			if a == z && st.text == "" {
				continue
			}
			if _, err := m.Apply(contract.Transaction{Edits: []contract.TextEdit{{Range: contract.Range{Start: a, End: z}, Text: st.text}}, Origin: contract.OriginAPI}); err != nil {
				return fmt.Sprintf("step %d: apply: %v", i, err)
			}
			undo = append(undo, undoStep{a, model[a:z], st.text})
			redo = nil
			model = model[:a] + st.text + model[z:]
		case 1:
			if got := m.Undo(); got != (len(undo) > 0) {
				if !got {
					undo = nil // a bounded history forgot the oldest steps
					continue
				}
				return fmt.Sprintf("step %d: undo returned %v with %d steps", i, got, len(undo))
			}
			if n := len(undo); n > 0 {
				u := undo[n-1]
				undo = undo[:n-1]
				redo = append(redo, u)
				model = model[:u.at] + u.old + model[u.at+len(u.new):]
			}
		case 2:
			if got := m.Redo(); got != (len(redo) > 0) {
				return fmt.Sprintf("step %d: redo returned %v with %d steps", i, got, len(redo))
			}
			if n := len(redo); n > 0 {
				u := redo[n-1]
				redo = redo[:n-1]
				undo = append(undo, u)
				model = model[:u.at] + u.new + model[u.at+len(u.old):]
			}
		case 3:
			r.save(t, s)
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if want := encode(model); !bytes.Equal(got, want) {
				return fmt.Sprintf("step %d: disk differs at byte %d of %d", i, firstDiff(got, want), len(want))
			}
			if s.State().Edited {
				return fmt.Sprintf("step %d: edited after saving", i)
			}
		}
		if got := m.Text().String(); got != model {
			return fmt.Sprintf("step %d: text differs at %d: %d bytes, want %d", i, firstDiff([]byte(got), []byte(model)), len(got), len(model))
		}
	}
	r.save(t, s)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, encode(model)) {
		return "final disk differs"
	}
	return ""
}

// minimize removes steps while the script still fails, one chunk size at a time.
func minimize(t *testing.T, r *rig, path, start string, encode func(string) []byte, steps []step) []step {
	t.Helper()
	for chunk := len(steps) / 2; chunk >= 1; chunk /= 2 {
		for i := 0; i+chunk <= len(steps); {
			try := append(append([]step{}, steps[:i]...), steps[i+chunk:]...)
			if run(t, r, path, start, encode, try) != "" {
				steps = try
			} else {
				i += chunk
			}
		}
	}
	return steps
}

// TestRandomEditsAgainstString is the second line of the card and the §8 property:
// three seeds of ten thousand steps each, on documents with every line ending and
// a byte order mark.
func TestRandomEditsAgainstString(t *testing.T) {
	stepsPerSeed := 10000
	if testing.Short() {
		stepsPerSeed = 800
	}
	starts := []struct {
		name, text string
		bom        bool
	}{
		{"mixed", "# 标题\n\n段落 **粗** *斜* `码`\n\n- a\n- b\n\n> q\n\n```go\nx\n```\n\n| a | b |\n|---|---|\n| c | d |\n", false},
		{"crlf", "# t\r\n\r\npara\r\n\r\n- a\r\n- b\r\n", false},
		{"bom", "first\n\nsecond\n", true},
	}
	for seed, start := range starts {
		t.Run(start.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "a.md")
			encode := func(s string) []byte {
				if start.bom {
					return append([]byte{0xef, 0xbb, 0xbf}, s...)
				}
				return []byte(s)
			}
			r := newRig(t)
			steps := script(uint64(seed)+1, stepsPerSeed)
			if msg := run(t, r, path, start.text, encode, steps); msg != "" {
				small := minimize(t, r, path, start.text, encode, steps)
				t.Fatalf("%s\nminimal script of %d steps, from %q: %+v", msg, len(small), start.text, small)
			}
		})
	}
}

func TestMinimizeShrinks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.md")
	r := newRig(t)
	// Saving under a wrong encoding fails as soon as a save runs after an edit.
	wrong := func(s string) []byte {
		if len(s) > 3 {
			return []byte(s + "!")
		}
		return []byte(s)
	}
	steps := []step{{from: 0, text: "abcd"}, {op: 1}, {op: 2}, {op: 3}, {from: 0, text: "z"}}
	small := minimize(t, r, path, "", wrong, steps)
	if len(small) >= len(steps) {
		t.Fatalf("still %d steps", len(small))
	}
}
