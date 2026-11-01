package docsession

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aurink/internal/contract"
	"aurink/internal/core/document"
)

type countedFactory struct {
	strings int
	bytes   int
}

type countedModel struct {
	contract.DocumentModel
	counts *countedFactory
}

type countedText struct {
	contract.Text
	counts *countedFactory
}

func (f *countedFactory) New(id contract.DocID, text string, meta contract.DocMeta, exts contract.Extensions) contract.DocumentModel {
	return &countedModel{DocumentModel: (document.Factory{}).New(id, text, meta, exts), counts: f}
}

func (m *countedModel) Text() contract.Text {
	return countedText{Text: m.DocumentModel.Text(), counts: m.counts}
}
func (t countedText) String() string {
	t.counts.strings++
	return t.Text.String()
}
func (t countedText) Slice(r contract.Range) string {
	t.counts.bytes += r.Len()
	return t.Text.Slice(r)
}

func TestTypingDoesNotCompareWholeText(t *testing.T) {
	h := newHarness(t)
	f := &countedFactory{}
	h.factory = f
	m := h.manager()
	path := filepath.Join(h.dir, "large.md")
	put(t, path, strings.Repeat("a", 200<<10))
	s := h.open(m, path)
	f.strings, f.bytes = 0, 0
	for range 100 {
		appendText(t, s, "x")
	}
	if f.strings != 0 || f.bytes != 100 {
		t.Fatalf("insertion requested %d full strings, hashed %d bytes", f.strings, f.bytes)
	}
	// Equal-length substitutions must also avoid a scan while lengths match.
	for range 100 {
		s.Model().Undo()
	}
	f.strings, f.bytes = 0, 0
	for i := range 100 {
		edit(t, s, 0, 1, string(byte('b'+i%2)))
	}
	if f.strings != 0 || f.bytes != 200 {
		t.Fatalf("typing requested %d full strings, hashed %d bytes; want 0, 200", f.strings, f.bytes)
	}
	s.Model().Replace(strings.Repeat("a", 200<<10), contract.OriginAPI)
	if s.State().Edited || f.strings == 0 {
		t.Fatal("candidate equality was not checked exactly")
	}
}

func TestFingerprintCollisionIsNotClean(t *testing.T) {
	h := newHarness(t)
	m := h.manager()
	s := m.New()
	appendText(t, s, "ab")
	path := filepath.Join(h.dir, "a.md")
	wantCode(t, h.finish(func(done func(error)) { s.SaveAs(path, done) }), "")
	edit(t, s, 0, 2, "ba")
	if !s.State().Edited {
		t.Fatal("equal lengths and colliding fingerprints hid different content")
	}
	s.Model().Undo()
	if s.State().Edited {
		t.Fatal("undo to the saved content is dirty")
	}
}

func TestSettingsLimitsAndAutosaveEligibility(t *testing.T) {
	h := newHarness(t)
	h.settings.Files.LargeFileBytes = 3
	m := h.manager()
	path := filepath.Join(h.dir, "large.md")
	put(t, path, "1234")
	called := 0
	m.Open(path, func(s contract.Session, err error) {
		called++
		wantCode(t, err, contract.CodeTooLarge)
		if s != nil {
			t.Fatal("large file opened")
		}
	})
	h.settle()
	if called != 1 || len(m.All()) != 0 {
		t.Fatal("open callback or sessions", called)
	}
	put(t, path, "a")
	s := h.open(m, path)
	appendText(t, s, "b")
	// Already-posted timer callbacks must obey a subsequent settings change.
	h.clock.advance(100 * time.Millisecond)
	h.settings.Files.Autosave = false
	m.SetSettings(h.settings)
	h.settle()
	if contents(t, path) != "a" {
		t.Fatal("a stale timer ignored disabled autosave")
	}
	h.settings.Files.Autosave, h.settings.Files.AutosaveDelayMs = true, 200
	m.SetSettings(h.settings)
	h.advance(199 * time.Millisecond)
	if contents(t, path) != "a" {
		t.Fatal("settings delay ignored")
	}
	h.advance(time.Millisecond)
	if contents(t, path) != "ab" {
		t.Fatal("settings autosave did not run")
	}
	appendText(t, s, "c")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	wantCode(t, h.finish(s.Revert), contract.CodeNotFound)
	if !s.State().Deleted || !s.State().Edited {
		t.Fatal("deleted state", s.State())
	}
	h.advance(time.Second)
	if _, err := h.files.FileStore.Stat(path); contract.CodeOf(err) != contract.CodeNotFound {
		t.Fatal("autosave recreated a deleted file")
	}
	wantCode(t, h.finish(s.KeepMine), "")
	if contents(t, path) != "abc" || s.State().Deleted {
		t.Fatal("KeepMine did not recreate the file")
	}
}
