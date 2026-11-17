package docsession

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aurink/internal/contract"
	"golang.org/x/text/encoding/simplifiedchinese"
)

type countingFactory struct {
	inner contract.DocumentFactory
	large int
	small int
}

func (c *countingFactory) New(id contract.DocID, text string, meta contract.DocMeta, exts contract.Extensions) contract.DocumentModel {
	if meta.Large {
		c.large++
	} else {
		c.small++
	}
	return c.inner.New(id, text, meta, exts)
}

func TestFivePointFiveMegabytesOpensInLargeModeAndOver64MBFails(t *testing.T) {
	h := newHarness(t)
	cf := &countingFactory{inner: h.factory}
	h.factory = cf
	m := h.manager()

	largePath := filepath.Join(h.dir, "5_5mb.md")
	// 5.5 MiB > default 5 MiB LargeFileBytes, well under 64 MiB hard cap.
	line := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\n" // 65 bytes
	body := "# Big\n\n" + strings.Repeat(line, (5500<<10)/len(line))
	if err := os.WriteFile(largePath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	var s contract.Session
	var openErr error
	m.Open(largePath, func(got contract.Session, err error) { s, openErr = got, err })
	h.settle()
	if openErr != nil {
		t.Fatalf("5.5 MB Open failed: %v", openErr)
	}
	if cf.large != 1 || cf.small != 0 {
		t.Fatalf("factory counts: large=%d small=%d", cf.large, cf.small)
	}
	if !s.State().LargeFile || !s.Model().Meta().Large || s.Model().AST().LeafCount() != 1 {
		t.Fatalf("unexpected state=%+v meta=%+v leaves=%d", s.State(), s.Model().Meta(), s.Model().AST().LeafCount())
	}

	if _, err := s.Model().Apply(contract.Transaction{
		Edits:  []contract.TextEdit{{Range: contract.Range{Start: 0, End: 5}, Text: "# BIG"}},
		Origin: contract.OriginSource,
	}); err != nil {
		t.Fatal(err)
	}
	var saveErr error
	s.Save(func(err error) { saveErr = err })
	h.settle()
	if saveErr != nil {
		t.Fatal(saveErr)
	}
	disk, err := os.ReadFile(largePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(disk, []byte("# BIG\n\n")) || len(disk) != len(body) {
		t.Fatalf("saved 5.5 MB file corrupted: len=%d want=%d", len(disk), len(body))
	}

	// Over 64 MiB exceeds FileStore's hard limit and returns CodeTooLarge.
	hugePath := filepath.Join(h.dir, "65mb.md")
	f, err := os.Create(hugePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate((64 << 20) + 1); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	var hugeErr error
	m.Open(hugePath, func(_ contract.Session, err error) { hugeErr = err })
	h.settle()
	if contract.CodeOf(hugeErr) != contract.CodeTooLarge {
		t.Fatalf("65 MB Open error = %v (code %q), want CodeTooLarge", hugeErr, contract.CodeOf(hugeErr))
	}
}

func TestGB18030SaveAsAndExternalReload(t *testing.T) {
	h := newHarness(t)
	m := h.manager()

	text := "# 旧编码标题\r\n\r\n| 列一 | 列二 |\r\n| --- | --- |\r\n| 甲 | 乙 |\r\n\r\n```go\nvar x = \"中文\"\n```\r\n"
	gbkBytes, err := simplifiedchinese.GB18030.NewEncoder().Bytes([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	origPath := filepath.Join(h.dir, "orig.md")
	if err := os.WriteFile(origPath, gbkBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	var s contract.Session
	m.Open(origPath, func(got contract.Session, err error) {
		if err != nil {
			t.Fatalf("Open GB18030: %v", err)
		}
		s = got
	})
	h.settle()
	if !s.State().ReadOnly || s.Model().Meta().Encoding != "gb18030" {
		t.Fatalf("opened state=%+v meta=%+v", s.State(), s.Model().Meta())
	}

	// External replacement of an unedited GB18030 file with UTF-8 unlocks it.
	utf8Updated := text + "\r\nUTF-8 外部更新\r\n"
	if err := os.WriteFile(origPath, []byte(utf8Updated), 0o644); err != nil {
		t.Fatal(err)
	}
	m.Recheck()
	h.settle()
	if s.State().ReadOnly || s.Model().Meta().Encoding != "utf-8" || s.Model().Text().String() != utf8Updated {
		t.Fatalf("after external UTF-8 reload: state=%+v meta=%+v", s.State(), s.Model().Meta())
	}

	// External replacement back to GB18030 locks it read-only again.
	if err := os.WriteFile(origPath, gbkBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	m.Recheck()
	h.settle()
	if !s.State().ReadOnly || s.Model().Meta().Encoding != "gb18030" {
		t.Fatalf("after external GB18030 reload: state=%+v meta=%+v", s.State(), s.Model().Meta())
	}

	// SaveAs on a read-only GB18030 session writes the new path as UTF-8,
	// unlocks the session, and leaves the original GB18030 file untouched.
	copyPath := filepath.Join(h.dir, "copy-utf8.md")
	var saveAsErr error
	s.SaveAs(copyPath, func(err error) { saveAsErr = err })
	h.settle()
	if saveAsErr != nil {
		t.Fatalf("SaveAs from GB18030: %v", saveAsErr)
	}
	if s.State().ReadOnly || s.Model().Meta().Encoding != "utf-8" || s.State().Path != copyPath {
		t.Fatalf("after SaveAs: state=%+v meta=%+v", s.State(), s.Model().Meta())
	}
	origDisk, _ := os.ReadFile(origPath)
	if !bytes.Equal(origDisk, gbkBytes) {
		t.Fatal("original GB18030 file was modified by SaveAs")
	}
	copyDisk, _ := os.ReadFile(copyPath)
	if string(copyDisk) != text {
		t.Fatalf("SaveAs UTF-8 content = %q, want %q", copyDisk, text)
	}
}
