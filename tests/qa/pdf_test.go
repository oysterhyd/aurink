package qa

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"aurink/internal/contract"
	"aurink/locales"
)

// fakePrinter stands for the WebView that prints: it keeps the page it was
// given and answers with a token PDF.
type fakePrinter struct {
	mu    sync.Mutex
	pages []string
	opts  []contract.PDFOptions
}

func (f *fakePrinter) HTMLToPDF(ctx context.Context, html, baseURL string, opts contract.PDFOptions) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pages = append(f.pages, html)
	f.opts = append(f.opts, opts)
	return []byte("%PDF-1.7 fake"), nil
}

func (f *fakePrinter) printed() ([]string, []contract.PDFOptions) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.pages), slices.Clone(f.opts)
}

// Export as PDF (INT-3.1): the dialog asks for a PDF, the page the printer
// gets has the title, the rendered math and the soft breaks the editor
// shows, and the file holds what the printer gave.
func TestExportPDF(t *testing.T) {
	a := newApp(t)
	a.render.available = true
	a.open("note.md", "# Title\n\none\ntwo $x^2$\n")
	a.p.export = filepath.Join(a.dir, "out", "note")
	if err := os.MkdirAll(filepath.Dir(a.p.export), 0o755); err != nil {
		t.Fatal(err)
	}
	a.run(contract.CmdFileExportPDF)
	a.until("the notice", func() bool { return slices.Contains(a.s.Notices(), locales.ShellExported()) })
	if !slices.Equal(a.p.kinds, []string{"pdf"}) {
		t.Fatalf("the dialog was asked for %v", a.p.kinds)
	}
	data, err := os.ReadFile(a.p.export + ".pdf")
	if err != nil || string(data) != "%PDF-1.7 fake" {
		t.Fatalf("the PDF is %q, %v", data, err)
	}
	pages, opts := a.printer.printed()
	if len(pages) != 1 {
		t.Fatalf("%d pages printed", len(pages))
	}
	for _, want := range []string{"<title>note</title>", "one<br />", `class="math"`, "<svg"} {
		if !strings.Contains(pages[0], want) {
			t.Fatalf("the printed page lacks %s:\n%s", want, pages[0])
		}
	}
	if strings.Contains(pages[0], "x^2") {
		t.Fatal("the formula went to the printer as source")
	}
	if opts[0].PageSize != "A4" {
		t.Fatalf("page size %q", opts[0].PageSize)
	}
	if slices.Contains(a.s.Notices(), locales.ShellExporting()) {
		t.Fatal("the progress notice stayed")
	}
}

// A PDF breaks lines as the editor does: with soft breaks set to spaces, the
// printer gets no line break between the two lines.
func TestExportPDFFollowsSoftBreak(t *testing.T) {
	a := newApp(t)
	if err := a.store.Update(func(v *contract.Settings) { v.Editor.SoftBreak = contract.SoftBreakSpace }); err != nil {
		t.Fatal(err)
	}
	a.open("note.md", "one\ntwo\n")
	a.p.export = filepath.Join(a.dir, "note.pdf")
	a.run(contract.CmdFileExportPDF)
	a.until("the notice", func() bool { return slices.Contains(a.s.Notices(), locales.ShellExported()) })
	pages, _ := a.printer.printed()
	if len(pages) != 1 || !strings.Contains(pages[0], "one") || strings.Contains(pages[0], "one<br />") {
		t.Fatalf("the printed page breaks the line:\n%v", pages)
	}
}

// Without a menu bar (Windows, Linux) the mark in the title bar opens the File
// menu, where print and export are, as in Typora's File menu.
func TestFileMenuUnderTheMarkPrintsAndExports(t *testing.T) {
	a := newApp(t)
	a.open("note.md", "# Title\n\nwords\n")
	a.click(locales.ShellFileMenu())
	a.click(locales.ShellPrint())
	a.until("the page", func() bool { return len(a.printed) == 1 })
	a.p.export = filepath.Join(a.dir, "note.pdf")
	a.click(locales.ShellFileMenu())
	a.click(locales.ShellExportPdf())
	a.until("the notice", func() bool { return slices.Contains(a.s.Notices(), locales.ShellExported()) })
	if !slices.Equal(a.p.kinds, []string{"pdf"}) {
		t.Fatalf("the dialog was asked for %v", a.p.kinds)
	}
}

// Print (INT-3.1) gives the print preview the light, whole page.
func TestPrintGivesThePreviewTheLightPage(t *testing.T) {
	a := newApp(t)
	if err := a.store.Update(func(v *contract.Settings) { v.Appearance.Theme = "dark" }); err != nil {
		t.Fatal(err)
	}
	a.open("note.md", "# Title\n\nwords\n")
	a.run(contract.CmdFilePrint)
	a.until("the page", func() bool { return len(a.printed) == 1 })
	page := a.printed[0]
	for _, want := range []string{"<!DOCTYPE html>", "<title>note</title>", "words"} {
		if !strings.Contains(page, want) {
			t.Fatalf("the preview's page lacks %s:\n%s", want, page)
		}
	}
	if len(a.p.kinds) != 0 {
		t.Fatalf("print asked for a path: %v", a.p.kinds)
	}
}
