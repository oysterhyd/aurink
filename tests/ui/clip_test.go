package ui_test

import (
	"path/filepath"
	"strings"
	"testing"

	"aurink/internal/adapters/richpaste"
	"aurink/internal/contract"
	"aurink/internal/core/document"
	"aurink/internal/core/editing"
	"aurink/internal/core/projection"
	"aurink/internal/views/liveedit"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

// fakeAssets records imports and answers with paths next to the document.
type fakeAssets struct {
	docs   []string
	files  []string
	images int
}

func (a *fakeAssets) ImportFiles(doc string, files []string) ([]string, error) {
	a.docs = append(a.docs, doc)
	var out []string
	for _, f := range files {
		a.files = append(a.files, f)
		out = append(out, "assets/"+filepath.Base(f))
	}
	return out, nil
}

func (a *fakeAssets) ImportImage(doc string, png []byte, hint string) (string, error) {
	a.docs = append(a.docs, doc)
	a.images++
	return "assets/image.png", nil
}

type clipEnv struct {
	*harness
	assets *fakeAssets
	path   string
	saves  int
	opened []string
}

func openClip(t *testing.T, src, path string) *clipEnv {
	t.Helper()
	env := &clipEnv{assets: &fakeAssets{}, path: path}
	m := document.Factory{}.New("doc", src, contract.DocMeta{Dominant: "\n", Encoding: "utf-8"}, 0)
	e := liveedit.New(liveedit.Options{Model: m, Projector: projection.New(), Editing: editing.New(),
		Clip:      func(md string) string { return "<p>" + md + "</p>" },
		Converter: richpaste.New(),
		Assets:    env.assets,
		DocPath:   func() string { return env.path },
		SaveFirst: func(done func(bool)) {
			env.saves++
			env.path = filepath.FromSlash("/docs/new.md")
			done(true)
		},
		OpenFile: func(p string) { env.opened = append(env.opened, p) },
		OnError:  func(err error) { t.Errorf("background error: %v", err) }})
	t.Cleanup(e.Close)
	e.Focus()
	tt := ui.NewTester(func(c *ui.Context) { e.View(c) }, 1000, 700)
	tt.Frame()
	env.harness = &harness{t, m, e, tt}
	return env
}

func clipText(t *testing.T, d transfer.Data, f transfer.Format) string {
	t.Helper()
	b, err := d.Read(f)
	if err != nil {
		return ""
	}
	return string(b)
}

// TestCopyPutsThreeFormats: the Markdown as text and as Markdown, and the HTML
// fragment (FE-1.5).
func TestCopyPutsThreeFormats(t *testing.T) {
	h := openClip(t, "a **bold** word\n", "/docs/a.md")
	h.sel(2, 10)
	h.mod(ui.Cmd, ui.KeyC)
	d := h.tt.ClipboardData()
	if got := clipText(t, d, transfer.Text); got != "**bold**" {
		t.Fatalf("text %q", got)
	}
	if got := clipText(t, d, transfer.Markdown); got != "**bold**" {
		t.Fatalf("markdown %q", got)
	}
	if got := clipText(t, d, transfer.HTML); got != "<p>**bold**</p>" {
		t.Fatalf("html %q", got)
	}
	h.e.CopyMarkdown()
	if d := h.tt.ClipboardData(); clipText(t, d, transfer.HTML) != "" || clipText(t, d, transfer.Markdown) != "**bold**" {
		t.Fatal("copy as Markdown put HTML on the clipboard")
	}
	h.e.CopyHTML()
	if d := h.tt.ClipboardData(); clipText(t, d, transfer.Text) != "<p>**bold**</p>" {
		t.Fatalf("copy as HTML put %q as the text", clipText(t, d, transfer.Text))
	}
}

func setClip(h *clipEnv, reps ...transfer.Representation) {
	h.tt.SetClipboardData(transfer.New(transfer.NewItem(reps...)))
}

// TestPastePrefersMarkdownThenConvertsHTMLAndRTF follows D11 and D12.
func TestPastePrefersMarkdownThenConvertsHTMLAndRTF(t *testing.T) {
	h := openClip(t, "", "/docs/a.md")
	setClip(h, transfer.Bytes(transfer.Text, []byte("plain")), transfer.Bytes(transfer.Markdown, []byte("*md*")),
		transfer.Bytes(transfer.HTML, []byte("<b>html</b>")))
	h.mod(ui.Cmd, ui.KeyV)
	h.wantSrc("*md*")

	h = openClip(t, "", "/docs/a.md")
	setClip(h, transfer.Bytes(transfer.Text, []byte("html")), transfer.Bytes(transfer.HTML, []byte("<p><b>html</b></p>")))
	h.mod(ui.Cmd, ui.KeyV)
	h.wantSrc("**html**")

	h = openClip(t, "", "/docs/a.md")
	setClip(h, transfer.Bytes(transfer.Text, []byte("rtf")), transfer.Bytes("text/rtf", []byte(`{\rtf1\ansi {\b rtf}\par}`)))
	h.mod(ui.Cmd, ui.KeyV)
	h.wantSrc("**rtf**")

	// Paste as plain text takes the text even beside HTML.
	h = openClip(t, "", "/docs/a.md")
	setClip(h, transfer.Bytes(transfer.Text, []byte("*not md*")), transfer.Bytes(transfer.HTML, []byte("<b>x</b>")))
	h.e.PastePlain()
	h.tt.Frame()
	if got := h.m.Text().String(); got != `\*not md\*` && got != "*not md*" {
		t.Fatalf("pasted as plain: %q", got)
	}
	if strings.Contains(h.m.Text().String(), "**x**") {
		t.Fatal("paste as plain converted the HTML")
	}
}

// TestPasteImageImportsIt: a picture on the clipboard is written through Assets
// and inserted; an unsaved document is saved first (07 FE-1.5).
func TestPasteImageImportsIt(t *testing.T) {
	h := openClip(t, "x\n", "")
	h.caret(2)
	setClip(h, transfer.Bytes(transfer.PNG, []byte("\x89PNG fake")))
	h.mod(ui.Cmd, ui.KeyV)
	if h.saves != 1 {
		t.Fatalf("%d saves before importing into an unsaved document", h.saves)
	}
	if h.assets.images != 1 || h.assets.docs[0] != filepath.FromSlash("/docs/new.md") {
		t.Fatalf("imports %+v", h.assets)
	}
	if !strings.Contains(h.m.Text().String(), "(assets/image.png)") {
		t.Fatalf("no image inserted: %q", h.m.Text().String())
	}
}

// TestPasteImageFilesImportsThem: image files copied in a file manager.
func TestPasteImageFilesImportsThem(t *testing.T) {
	h := openClip(t, "", "/docs/a.md")
	dir := t.TempDir()
	files, err := transfer.FileData(filepath.Join(dir, "a.png"), filepath.Join(dir, "b.txt"))
	if err != nil {
		t.Fatal(err)
	}
	h.tt.SetClipboardData(files)
	h.mod(ui.Cmd, ui.KeyV)
	if len(h.assets.files) != 1 || filepath.Base(h.assets.files[0]) != "a.png" {
		t.Fatalf("imported %q", h.assets.files)
	}
	if !strings.Contains(h.m.Text().String(), "(assets/a.png)") {
		t.Fatalf("no image inserted: %q", h.m.Text().String())
	}
}

// TestDropImagesAndOpenDocuments: images dropped on the document go in at the
// caret, since MyGo does not say where they were dropped, and a Markdown file
// dropped there opens.
func TestDropImagesAndOpenDocuments(t *testing.T) {
	h := openClip(t, "one\n\ntwo\n", "/docs/a.md")
	h.caret(len("one\n\ntwo"))
	at := h.rowShowing("one")
	if !h.tt.DropFiles(at.X+40, at.Y+8, filepath.FromSlash("/pics/c.jpg"), filepath.FromSlash("/notes/n.md")) {
		t.Fatal("the document did not take the drop")
	}
	h.tt.Frame()
	src := h.m.Text().String()
	if !strings.Contains(src, "(assets/c.jpg)") {
		t.Fatalf("no image: %q", src)
	}
	if strings.Index(src, "assets/c.jpg") < strings.Index(src, "two") {
		t.Fatalf("the image went in above the drop point: %q", src)
	}
	if len(h.opened) != 1 || filepath.Base(h.opened[0]) != "n.md" {
		t.Fatalf("opened %q", h.opened)
	}
}

// TestLargePasteIsOneUndo: 2,000 pasted lines go with one undo.
func TestLargePasteIsOneUndo(t *testing.T) {
	h := openClip(t, "start\n", "/docs/a.md")
	h.caret(6)
	h.typed("x")
	big := strings.Repeat("line of text\n", 2000)
	setClip(h, transfer.Bytes(transfer.Text, []byte(big)))
	h.mod(ui.Cmd, ui.KeyV)
	if !strings.Contains(h.m.Text().String(), big) {
		t.Fatal("not pasted")
	}
	// Typing after it is a step of its own.
	h.typed("y")
	h.mod(ui.Cmd, ui.KeyZ)
	h.wantSrc("start\nx" + big)
	h.mod(ui.Cmd, ui.KeyZ)
	h.wantSrc("start\nx")
}

// TestCopyPasteRoundTrip copies a whole document and pastes it over itself:
// nothing changes (05 FE-1.5).
func TestCopyPasteRoundTrip(t *testing.T) {
	src := "# Title\n\nSome *emphasis*, `code` and a [link](https://x.org).\n\n- [ ] task\n- item\n\n> quote\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n```go\nfmt.Println(\"中文\")\n```\n"
	h := openClip(t, src, "/docs/a.md")
	h.mod(ui.Cmd, ui.KeyA)
	h.mod(ui.Cmd, ui.KeyC)
	h.mod(ui.Cmd, ui.KeyA)
	h.mod(ui.Cmd, ui.KeyV)
	h.wantSrc(src)
}
