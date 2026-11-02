package liveedit

import (
	"path/filepath"
	"strings"

	"aurink/internal/contract"
)

// imageExts are the files a paste or a drop imports as images.
var imageExts = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".svg": true, ".webp": true}

func isImage(path string) bool { return imageExts[strings.ToLower(filepath.Ext(path))] }

// copyFormats is what a copy puts on the clipboard (FE-1.5): the Markdown
// source as text and as Markdown, which a paste here prefers, and the HTML
// fragment for rich editors. CopyMarkdown leaves the HTML out; CopyHTML puts
// the HTML source in as the text too.
type copyAs uint8

const (
	copyAll copyAs = iota
	copyMarkdown
	copyHTML
)

func (e *Editor) copySelection(as copyAs) bool {
	rg := contract.SelRange{Anchor: e.anchor, Head: e.head}.Range()
	if rg.Start == rg.End || !e.hasView {
		return false
	}
	md := e.text.Slice(rg)
	html := ""
	if as != copyMarkdown && e.opts.Clip != nil {
		html = e.opts.Clip(md)
	}
	plain := md
	if as == copyHTML {
		if html == "" {
			return false
		}
		plain = html
	}
	if e.opts.Clipboard == nil {
		e.services.WriteClipboard(plain)
		return true
	}
	d := contract.ClipData{Text: plain, HTML: html}
	if as != copyHTML {
		d.Markdown = md
	}
	if err := e.opts.Clipboard.Write(d); err != nil {
		e.report(err)
	}
	return true
}

// paste reads the clipboard: images and image files are imported, Markdown is
// taken as it is, HTML and RTF are converted to Markdown, and plain text is the
// fallback, which AsPlain always takes (D11, D12).
func (e *Editor) paste(asPlain bool) {
	if e.readOnly() || !e.hasView {
		return
	}
	var data contract.ClipData
	if e.opts.Clipboard != nil {
		data = e.opts.Clipboard.Read()
	} else {
		data.Text = e.services.ReadClipboard()
	}
	if !asPlain {
		if len(data.Files) > 0 {
			var images []string
			for _, f := range data.Files {
				if isImage(f) {
					images = append(images, f)
				}
			}
			if len(images) > 0 {
				e.importFiles(images, e.head)
				return
			}
		}
	}
	p := &contract.PastePayload{Plain: data.Text, AsPlain: asPlain}
	if !asPlain {
		p.Markdown = data.Markdown
		if p.Markdown == "" && e.opts.Converter != nil {
			if html := data.HTML; html != "" {
				p.HTML = html
				if md, err := e.opts.Converter.HTMLToMarkdown(html); err == nil {
					p.Markdown = md
				}
			} else if rtf := data.RTF; rtf != "" {
				p.RTF = rtf
				if md, err := e.opts.Converter.RTFToMarkdown(rtf); err == nil {
					p.Markdown = md
				}
			}
		}
		if p.Plain == "" && p.Markdown == "" {
			if len(data.PNG) > 0 {
				e.importPNG(data.PNG, e.head)
				return
			}
		}
	}
	if p.Plain == "" && p.Markdown == "" {
		return
	}
	// One paste is one undo step, however large (FE-1.5): the model never
	// coalesces a paste with typing.
	e.intent(contract.Intent{Kind: contract.IntentPaste, Paste: p})
}

// dropFiles takes files dropped on the document: images go in at off, Markdown
// and text files open (FE-1.5).
func (e *Editor) dropFiles(paths []string, at int) {
	var images []string
	for _, p := range paths {
		switch {
		case isImage(p):
			images = append(images, p)
		case e.opts.OpenFile != nil:
			switch strings.ToLower(filepath.Ext(p)) {
			case ".md", ".markdown", ".txt":
				e.opts.OpenFile(p)
			}
		}
	}
	if len(images) > 0 && !e.readOnly() {
		e.importFiles(images, at)
	}
}

// importFiles copies image files through Assets, after the document has a file
// to put them next to, and inserts them at off.
func (e *Editor) importFiles(paths []string, off int) {
	e.withPath(func(doc string) {
		if e.opts.Assets == nil {
			e.insertImages(paths, off)
			return
		}
		e.background(func() func() {
			out, err := e.opts.Assets.ImportFiles(doc, paths)
			return func() {
				if err != nil {
					e.report(err)
					return
				}
				e.insertImages(out, off)
			}
		})
	})
}

// importPNG writes a pasted image through Assets and inserts it at off.
func (e *Editor) importPNG(png []byte, off int) {
	if e.opts.Assets == nil {
		return
	}
	e.withPath(func(doc string) {
		e.background(func() func() {
			out, err := e.opts.Assets.ImportImage(doc, png, "")
			return func() {
				if err != nil {
					e.report(err)
					return
				}
				e.insertImages([]string{out}, off)
			}
		})
	})
}

// withPath runs fn with the document's file. An unsaved document is saved
// first, where the person chooses, since its images go next to it (07 FE-1.5).
func (e *Editor) withPath(fn func(doc string)) {
	doc := ""
	if e.opts.DocPath != nil {
		doc = e.opts.DocPath()
	}
	if doc != "" || e.opts.Assets == nil {
		fn(doc)
		return
	}
	if e.opts.SaveFirst == nil {
		return
	}
	e.opts.SaveFirst(func(ok bool) {
		if ok && e.opts.DocPath != nil {
			if doc := e.opts.DocPath(); doc != "" {
				fn(doc)
			}
		}
	})
}

// background runs work off the main thread and its result back on it; without
// a dispatcher, as in tests, it runs at once.
func (e *Editor) background(work func() func()) {
	if e.opts.Dispatcher == nil {
		work()()
		return
	}
	go func() {
		done := work()
		e.opts.Dispatcher.Post(func() {
			done()
			e.invalidate()
		})
	}()
}

func (e *Editor) insertImages(paths []string, off int) {
	if len(paths) == 0 || e.readOnly() {
		return
	}
	off = max(0, min(off, e.text.Len()))
	e.model.SetSelection(contract.Caret(off))
	if err := e.RunEdit(contract.EditCommand{ID: contract.CmdInsertImages, Paths: paths}); err != nil {
		e.report(err)
	}
	e.invalidate()
}

func (e *Editor) report(err error) {
	if e.opts.OnError != nil {
		e.opts.OnError(err)
	}
}
