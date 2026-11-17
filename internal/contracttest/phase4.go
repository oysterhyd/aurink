package contracttest

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aurink/internal/contract"
)

// SessionPhase4Fixture builds session managers configured with a known
// LargeFileBytes threshold so large-file mode and legacy encoding conversion
// can be tested deterministically (docs/09-phase4.md §5 D1–D2, D6–D7).
type SessionPhase4Fixture struct {
	SessionFixture
	// LargeFileBytes is the threshold configured on the manager's settings.
	LargeFileBytes int64
}

// TestLargeDocument checks docs/09-phase4.md D2: a document created with
// DocMeta.Large == true skips Markdown syntax parsing while preserving every
// contract.DocumentModel and contract.AST invariant across edits, Replace,
// Undo and Redo.
func TestLargeDocument(t *testing.T, f contract.DocumentFactory) {
	src := "---\ntitle: note\n---\n\n# Heading\n\nParagraph with **bold**, $x^2$, [^1] and [ref].\n\n[^1]: note\n[ref]: /u\n"
	meta := contract.DocMeta{EOL: contract.EOLLF, Dominant: "\n", Encoding: "utf-8", Large: true}
	m := f.New("large", src, meta, contract.ExtDefault)
	if m == nil {
		t.Fatal("New returned nil")
	}
	if !m.Meta().Large {
		t.Fatal("Meta().Large is false")
	}
	checkPlainAST := func(want string, wantVer contract.Version) {
		t.Helper()
		a := m.AST()
		if a.Version() != wantVer {
			t.Fatalf("AST version %d, want %d", a.Version(), wantVer)
		}
		checkAST(t, want, a)
		if a.LeafCount() != 1 {
			t.Fatalf("large document parsed %d leaves, want 1 plain leaf", a.LeafCount())
		}
		if len(a.Outline()) != 0 {
			t.Fatalf("large document produced outline: %+v", a.Outline())
		}
		if _, ok := a.FrontMatter(); ok {
			t.Fatal("large document parsed Front Matter")
		}
		if len(a.Footnotes()) != 0 || len(a.LinkDefs()) != 0 {
			t.Fatalf("large document parsed definitions: notes=%v links=%v", a.Footnotes(), a.LinkDefs())
		}
		for _, in := range a.Inlines(a.LeafAt(0)) {
			if in.Kind != contract.InlineText {
				t.Fatalf("large document parsed inline syntax %v", in.Kind)
			}
		}
	}
	checkPlainAST(src, m.Version())

	// Empty large document also satisfies AST invariants.
	empty := f.New("empty-large", "", meta, contract.ExtDefault)
	checkAST(t, "", empty.AST())
	if empty.AST().LeafCount() != 1 {
		t.Fatalf("empty large doc has %d leaves", empty.AST().LeafCount())
	}

	// Edits, Replace, Undo and Redo keep text, selection and plain AST in sync.
	var events []contract.DocEvent
	defer m.Subscribe(func(ev contract.DocEvent) { events = append(events, ev) })()

	v1, err := m.Apply(insertAt(len("# Heading"), " 2"))
	if err != nil {
		t.Fatal(err)
	}
	want1 := strings.Replace(src, "# Heading", "# Heading 2", 1)
	if m.Text().String() != want1 {
		t.Fatalf("text %q, want %q", m.Text().String(), want1)
	}
	checkPlainAST(want1, v1)
	if len(events) != 1 || events[0].Kind != contract.DocTextChanged || len(events[0].Dirty) == 0 {
		t.Fatalf("events after edit: %+v", events)
	}

	v2 := m.Replace("", contract.OriginReload)
	checkPlainAST("", v2)

	if !m.Undo() || m.Text().String() != want1 {
		t.Fatalf("after first undo: %q", m.Text().String())
	}
	checkPlainAST(want1, m.Version())

	if !m.Undo() || m.Text().String() != src {
		t.Fatalf("after second undo: %q", m.Text().String())
	}
	checkPlainAST(src, m.Version())

	if !m.Redo() || m.Text().String() != want1 {
		t.Fatalf("after redo: %q", m.Text().String())
	}
	checkPlainAST(want1, m.Version())
}

// gb18030Sample encodes "# 中文\r\n\r\n段落，。\U00010000\r\n" in GB18030 (including
// both 2-byte GBK sequences and a 4-byte supplementary-plane codepoint).
var gb18030Sample = []byte{
	'#', ' ', 0xD6, 0xD0, 0xCE, 0xC4, '\r', '\n',
	'\r', '\n',
	0xB6, 0xCE, 0xC2, 0xE4, 0xA3, 0xAC, 0xA1, 0xA3,
	0x90, 0x30, 0x81, 0x30, '\r', '\n',
}

const gb18030Text = "# 中文\r\n\r\n段落，。\U00010000\r\n"

// TestFileStoreGB18030 checks docs/09-phase4.md D6: valid GB18030 / GBK files
// decode with Meta.Encoding == "gb18030", direct writes in gb18030 are refused,
// and malformed or binary byte streams are still rejected.
func TestFileStoreGB18030(t *testing.T, newFixture func(t *testing.T) FileStoreFixture) {
	t.Run("valid GB18030 decodes and refuses lossy writeback", func(t *testing.T) {
		f := newFixture(t)
		path := f.Path("legacy.md")
		f.Put(path, gb18030Sample)
		got, err := f.Store.Read(path, contract.ReadOptions{})
		if err != nil {
			t.Fatalf("Read GB18030: %v", err)
		}
		if got.Text != gb18030Text || got.Meta.Encoding != "gb18030" || got.Meta.BOM || got.Meta.EOL != contract.EOLCRLF || got.Meta.Dominant != "\r\n" {
			t.Fatalf("got text=%q meta=%+v", got.Text, got.Meta)
		}
		_, err = f.Store.Write(path, got.Text, got.Meta, &got.Stamp)
		wantCode(t, err, contract.CodeEncoding)
		if !bytes.Equal(f.Bytes(path), gb18030Sample) {
			t.Fatal("failed Write altered the original GB18030 file")
		}
		utf8Meta := got.Meta
		utf8Meta.Encoding = "utf-8"
		if _, err := f.Store.Write(path, got.Text, utf8Meta, &got.Stamp); err != nil {
			t.Fatalf("Write after UTF-8 conversion: %v", err)
		}
		if string(f.Bytes(path)) != gb18030Text {
			t.Fatalf("UTF-8 write produced %q, want %q", f.Bytes(path), gb18030Text)
		}
	})

	t.Run("invalid byte sequences return CodeEncoding", func(t *testing.T) {
		for name, bad := range map[string][]byte{
			"truncated lead byte":  {0xD6, 0xD0, 0x81},
			"invalid 0xFF byte":    []byte("hello\xffworld"),
			"truncated 4-byte seq": {0x90, 0x30, 0x81},
			"bad second byte 0x20": {0x81, 0x20, 'a', 'b'},
		} {
			t.Run(name, func(t *testing.T) {
				f := newFixture(t)
				path := f.Path("bad.md")
				f.Put(path, bad)
				_, err := f.Store.Read(path, contract.ReadOptions{})
				wantCode(t, err, contract.CodeEncoding)
			})
		}
	})

	t.Run("GB18030 with NUL is still binary", func(t *testing.T) {
		f := newFixture(t)
		path := f.Path("bin.md")
		f.Put(path, []byte{0xD6, 0xD0, 0x00, 0xCE, 0xC4})
		_, err := f.Store.Read(path, contract.ReadOptions{})
		wantCode(t, err, contract.CodeBinary)
	})
}

// TestSessionsPhase4 checks docs/09-phase4.md D1–D2 and D6–D7: files at or
// above LargeFileBytes open in large-file mode (without rejecting at 5 MB) and
// survive edits, autosave and crash recovery; GB18030 files open read-only and
// ConvertToUTF8 turns them into editable UTF-8 sessions.
func TestSessionsPhase4(t *testing.T, newFixture func(t *testing.T) SessionPhase4Fixture) {
	open := func(t *testing.T, f SessionPhase4Fixture, m contract.Sessions, path string) contract.Session {
		t.Helper()
		var got contract.Session
		var gotErr error
		m.Open(path, func(s contract.Session, err error) { got, gotErr = s, err })
		f.Settle()
		if gotErr != nil {
			t.Fatalf("Open(%s): %v", path, gotErr)
		}
		return got
	}

	t.Run("large file opens in large mode, autosaves and recovers", func(t *testing.T) {
		f := newFixture(t)
		m := f.New()
		smallPath := filepath.Join(f.Dir, "small.md")
		largePath := filepath.Join(f.Dir, "large.md")
		smallBody := "# Small\n\ntext\n"
		largeBody := "# Large\n\n" + strings.Repeat("0123456789abcdef\n", int(f.LargeFileBytes/16)+8)
		if err := os.WriteFile(smallPath, []byte(smallBody), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(largePath, []byte(largeBody), 0o644); err != nil {
			t.Fatal(err)
		}

		small := open(t, f, m, smallPath)
		if small.State().LargeFile || small.Model().Meta().Large {
			t.Fatalf("small file marked large: state=%+v meta=%+v", small.State(), small.Model().Meta())
		}

		large := open(t, f, m, largePath)
		if !large.State().LargeFile || !large.Model().Meta().Large {
			t.Fatalf("large file not marked large: state=%+v meta=%+v", large.State(), large.Model().Meta())
		}
		if len(large.Model().AST().Outline()) != 0 || large.Model().AST().LeafCount() != 1 {
			t.Fatal("large session parsed Markdown blocks")
		}

		if _, err := large.Model().Apply(insertAt(0, "prefix\n")); err != nil {
			t.Fatal(err)
		}
		// Advance past recovery delay (half autosave) but before autosave to test crash recovery.
		f.Advance(f.Autosave / 2)
		f.Settle()

		m2 := f.New()
		var restored []contract.Session
		m2.Restore(func(ss []contract.Session, err error) {
			if err != nil {
				t.Error(err)
			}
			restored = ss
		})
		f.Settle()
		if len(restored) != 1 || !restored[0].State().LargeFile || !restored[0].Model().Meta().Large {
			t.Fatalf("restored large session: %+v", restored)
		}
		if want := "prefix\n" + largeBody; restored[0].Model().Text().String() != want {
			t.Fatal("restored large text mismatch")
		}

		// Explicit save writes exact bytes to disk.
		var saveErr error
		restored[0].Save(func(err error) { saveErr = err })
		f.Settle()
		if saveErr != nil {
			t.Fatal(saveErr)
		}
		disk, err := os.ReadFile(largePath)
		if err != nil || string(disk) != "prefix\n"+largeBody {
			t.Fatalf("saved disk len=%d err=%v", len(disk), err)
		}
	})

	t.Run("GB18030 opens read-only and ConvertToUTF8 enables UTF-8 editing and saving", func(t *testing.T) {
		f := newFixture(t)
		m := f.New()
		path := filepath.Join(f.Dir, "gbk.md")
		if err := os.WriteFile(path, gb18030Sample, 0o644); err != nil {
			t.Fatal(err)
		}

		s := open(t, f, m, path)
		if !s.State().ReadOnly || !s.Model().ReadOnly() || s.State().Edited || s.Model().Meta().Encoding != "gb18030" {
			t.Fatalf("initial state=%+v meta=%+v", s.State(), s.Model().Meta())
		}
		_, err := s.Model().Apply(insertAt(0, "x"))
		wantCode(t, err, contract.CodeReadOnly)

		// Autosave timer must never overwrite the original GB18030 bytes while read-only.
		f.Advance(f.Autosave * 2)
		f.Settle()
		raw, _ := os.ReadFile(path)
		if !bytes.Equal(raw, gb18030Sample) {
			t.Fatal("read-only GB18030 file was modified on disk")
		}

		var states []contract.DocState
		defer s.OnState(func(st contract.DocState) { states = append(states, st) })()

		s.ConvertToUTF8()
		if s.State().ReadOnly || s.Model().ReadOnly() || !s.State().Edited {
			t.Fatalf("after ConvertToUTF8: state=%+v model.ReadOnly=%v", s.State(), s.Model().ReadOnly())
		}
		meta := s.Model().Meta()
		if meta.Encoding != "utf-8" || meta.BOM || meta.EOL != contract.EOLCRLF || meta.Dominant != "\r\n" {
			t.Fatalf("converted meta=%+v", meta)
		}
		if len(states) == 0 {
			t.Fatal("ConvertToUTF8 emitted no state notification")
		}

		// Calling ConvertToUTF8 again on a UTF-8 session changes nothing.
		before := len(states)
		s.ConvertToUTF8()
		if len(states) != before {
			t.Fatal("second ConvertToUTF8 emitted redundant state notification")
		}

		if _, err := s.Model().Apply(insertAt(len(gb18030Text), "新增\r\n")); err != nil {
			t.Fatalf("edit after ConvertToUTF8: %v", err)
		}
		f.Advance(f.Autosave)
		f.Settle()

		saved, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if want := gb18030Text + "新增\r\n"; string(saved) != want {
			t.Fatalf("saved UTF-8 %q, want %q", saved, want)
		}
		if s.State().Edited {
			t.Fatal("still marked Edited after autosave")
		}
	})
}
