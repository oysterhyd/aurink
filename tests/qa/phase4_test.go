package qa

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"aurink/internal/contract"
	"aurink/internal/views/editorapi"
	"aurink/locales"
	"aurink/tests/corpus"
	"github.com/egoist/mygo/ui"
	"golang.org/x/text/encoding/simplifiedchinese"
)

// TestLargeFileMode verifies that documents >= LargeFileBytes open directly in
// Source mode without running the renderer, lock out Live/Reading views, show
// the large-file banner and outline notice, and edit/save/undo losslessly
// (09 D1-D3, FE-1.8).
func TestLargeFileMode(t *testing.T) {
	a := newApp(t)
	body := corpus.Generate((5<<20)+(256<<10), 11)
	raw := "# Large Document\r\n\r\n$$\r\nE = mc^2\r\n$$\r\n\r\n```mermaid\r\ngraph TD; A-->B;\r\n```\r\n\r\n" +
		strings.ReplaceAll(body, "\n", "\r\n")
	p := a.open("huge.md", raw)

	st := a.s.ActiveSession().State()
	if !st.LargeFile || !a.s.ActiveSession().Model().Meta().Large {
		t.Fatalf("LargeFile = %v, Meta.Large = %v, want both true", st.LargeFile, a.s.ActiveSession().Model().Meta().Large)
	}
	if a.s.ActiveMode() != editorapi.ModeSource {
		t.Fatalf("ActiveMode = %v, want ModeSource", a.s.ActiveMode())
	}
	if !a.shows(locales.ShellBannerLarge()) {
		t.Fatal("missing large file banner")
	}
	if n := a.render.count(); n != 0 {
		t.Fatalf("renderer ran %d times on a large file, want 0", n)
	}

	// Live, Source-toggle, Reading (F7) and Go to Heading must be disabled.
	for _, id := range []contract.CommandID{
		contract.CmdViewLive,
		contract.CmdViewSource,
		contract.CmdViewRead,
		contract.CmdNavGoToHeading,
	} {
		cmd, ok := a.reg.Lookup(id)
		if !ok {
			t.Fatalf("missing command %s", id)
		}
		if cmd.Enabled != nil && cmd.Enabled() {
			t.Fatalf("command %s is enabled on a large file", id)
		}
	}
	a.tt.Key(0, ui.KeyF7)
	a.tt.Key(ui.Cmd, ui.KeySlash)
	a.settle()
	if a.s.ActiveMode() != editorapi.ModeSource || a.s.Reading() {
		t.Fatalf("shortcuts switched mode: mode=%v reading=%v", a.s.ActiveMode(), a.s.Reading())
	}

	// Outline sidebar shows the large-file notice.
	a.run(contract.CmdViewOutline)
	if !a.shows(locales.ShellLargeNoOutline()) {
		t.Fatal("outline panel did not show large-file notice")
	}

	// Edit, save, undo, redo and verify exact CRLF byte preservation.
	src := a.s.ActiveSource()
	if src == nil {
		t.Fatal("ActiveSource is nil")
	}
	src.State().Select(0, 0)
	a.tt.Frame()
	src.State().Insert("Preface: ")
	a.tt.Frame()
	a.run(contract.CmdFileSave)
	wantEdited := "Preface: " + raw
	if got := a.disk(p); got != wantEdited {
		t.Fatalf("saved bytes mismatch: len %d vs %d", len(got), len(wantEdited))
	}

	a.tt.Key(ui.Cmd, ui.KeyZ)
	a.tt.Frame()
	if a.text() != raw {
		t.Fatal("Undo did not restore original bytes")
	}
	a.tt.Key(ui.Cmd, ui.KeyY)
	a.tt.Frame()
	if a.text() != wantEdited {
		t.Fatal("Redo did not reapply edit")
	}
	if err := a.s.Check(); err != nil {
		t.Fatal(err)
	}
}

// TestLegacyGB18030BannerAndConvert verifies that GB18030/GBK files open
// read-only with the encoding banner, block edits until explicitly converted to
// UTF-8 (or saved as a copy), and preserve CRLF line endings upon save
// (09 D6-D7, BE-2.11, FE-1.8).
func TestLegacyGB18030BannerAndConvert(t *testing.T) {
	a := newApp(t)
	md := "# 旧编码标题\r\n\r\n中文段落与 **粗体** 内容。\r\n"
	gbBytes, err := simplifiedchinese.GB18030.NewEncoder().Bytes([]byte(md))
	if err != nil {
		t.Fatal(err)
	}
	if utf8.Valid(gbBytes) {
		t.Fatal("expected non-UTF-8 GB18030 bytes")
	}
	p := filepath.Join(a.dir, "legacy.md")
	if err := os.WriteFile(p, gbBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	a.s.Open(p)
	a.settle()

	sess := a.s.ActiveSession()
	if !sess.State().ReadOnly || sess.Model().Meta().Encoding != "gb18030" {
		t.Fatalf("ReadOnly=%v Encoding=%q, want true/gb18030", sess.State().ReadOnly, sess.Model().Meta().Encoding)
	}
	if !a.shows(locales.ShellBannerEncoding()) {
		t.Fatal("missing legacy encoding banner")
	}
	a.want(a.text(), md)

	// Attempting to type in Source mode while read-only leaves document & disk untouched.
	a.run(contract.CmdViewSource)
	if !a.s.ActiveSource().State().ReadOnly {
		t.Fatal("source editor should be ReadOnly before conversion")
	}
	a.typed("NOPE")
	a.want(a.text(), md)
	onDisk, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(onDisk, gbBytes) {
		t.Fatal("legacy GB18030 bytes on disk were modified while read-only")
	}

	// Click "Convert to UTF-8 and Edit" in the banner.
	a.click(locales.ShellConvertUtf8())
	if sess.State().ReadOnly || sess.Model().Meta().Encoding != "utf-8" {
		t.Fatalf("after convert: ReadOnly=%v Encoding=%q", sess.State().ReadOnly, sess.Model().Meta().Encoding)
	}
	if a.s.ActiveSource().State().ReadOnly {
		t.Fatal("source editor remained ReadOnly after conversion")
	}
	if a.shows(locales.ShellBannerEncoding()) {
		t.Fatal("encoding banner stayed after conversion")
	}

	// Edit in Source view and save: disk becomes valid UTF-8 with CRLF preserved.
	a.s.ActiveSource().State().Select(0, 0)
	a.tt.Frame()
	a.s.ActiveSource().State().Insert("已转换 ")
	a.tt.Frame()
	a.run(contract.CmdFileSave)

	saved, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.Valid(saved) {
		t.Fatal("saved file is not valid UTF-8")
	}
	a.want(string(saved), "已转换 "+md)
}

func TestLegacyGB18030SaveAsCopy(t *testing.T) {
	a := newApp(t)
	md := "# 副本测试\n\n保持原盘不变。\n"
	gbBytes, err := simplifiedchinese.GB18030.NewEncoder().Bytes([]byte(md))
	if err != nil {
		t.Fatal(err)
	}
	orig := filepath.Join(a.dir, "orig.md")
	if err := os.WriteFile(orig, gbBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	a.s.Open(orig)
	a.settle()
	if !a.shows(locales.ShellBannerEncoding()) {
		t.Fatal("missing legacy encoding banner")
	}

	copyPath := filepath.Join(a.dir, "utf8-copy.md")
	a.p.save = copyPath
	a.click(locales.ShellSaveAs())

	// Original file keeps its GB18030 bytes; copy is UTF-8 and unlocked.
	origAfter, err := os.ReadFile(orig)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(origAfter, gbBytes) {
		t.Fatal("SaveAs mutated original GB18030 file")
	}
	a.want(a.disk(copyPath), md)
	if a.s.ActiveSession().State().ReadOnly || a.s.ActiveSession().Model().Meta().Encoding != "utf-8" {
		t.Fatal("SaveAs did not unlock session as UTF-8")
	}
}
