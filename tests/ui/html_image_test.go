package ui_test

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aurink/internal/contract"
	"aurink/internal/core/document"
	"aurink/internal/core/editing"
	"aurink/internal/core/projection"
	"aurink/internal/views/liveedit"
	"github.com/egoist/mygo/ui"
)

func writeTestPNG(t *testing.T, path string, w, h int) {
	t.Helper()
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func pumpAll(t *testing.T, ch chan func(), count int) {
	t.Helper()
	for range count {
		select {
		case fn := <-ch:
			fn()
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for background image load")
		}
	}
}

// TestHTMLBlockSecurityRejections verifies 24 negative security, XSS, event
// handler, dangerous URI scheme, remote image, style injection, and malformed
// tag cases: every case is rejected by IsSafeHTMLBlock, falls back to pale
// source in Live mode, opens zero URLs, and preserves 100% of raw bytes when
// surrounding Markdown is edited (09 D8, FE-1.8).
func TestHTMLBlockSecurityRejections(t *testing.T) {
	cases := []struct {
		name string
		html string
	}{
		{"script_tag", "<script>alert(1)</script>"},
		{"nested_script", "<p><script>alert(1)</script></p>"},
		{"img_onerror", `<p><img src="local.png" onerror="alert(1)"></p>`},
		{"div_onclick", `<div onclick="evil()">click me</div>`},
		{"details_ontoggle", `<details ontoggle="evil()"><summary>More</summary><p>Body</p></details>`},
		{"javascript_href", `<p><a href="javascript:alert(1)">xss</a></p>`},
		{"javascript_mixed_case", `<p><a href="JaVaScRiPt:alert(1)">xss</a></p>`},
		{"javascript_dec_entity", `<p><a href="&#106;avascript:alert(1)">xss</a></p>`},
		{"javascript_hex_entity", `<p><a href="&#x6a;avascript:alert(1)">xss</a></p>`},
		{"javascript_tab_obfuscated", "<p><a href=\"java\tscript:alert(1)\">xss</a></p>"},
		{"javascript_ctrl_char", "<p><a href=\"\x01javascript:alert(1)\">xss</a></p>"},
		{"data_html_href", `<p><a href="data:text/html,<script>alert(1)</script>">xss</a></p>`},
		{"vbscript_href", `<p><a href="vbscript:msgbox(1)">xss</a></p>`},
		{"proto_relative_href", `<p><a href="//evil.example/payload">proto</a></p>`},
		{"iframe_tag", `<iframe src="https://example.com"></iframe>`},
		{"style_tag", `<style>body { display: none }</style>`},
		{"inline_style_attr", `<p style="background:url(https://evil.example/track)">styled</p>`},
		{"remote_https_img", `<p><img src="https://example.com/remote.png" alt="remote"></p>`},
		{"remote_http_img", `<p><img src="http://example.com/remote.png" alt="remote"></p>`},
		{"proto_relative_img", `<p><img src="//example.com/remote.png" alt="proto"></p>`},
		{"data_uri_img", `<p><img src="data:image/png;base64,iVBORw0KGgo=" alt="data"></p>`},
		{"svg_onload", `<svg onload="alert(1)"><circle cx="10" cy="10" r="5"/></svg>`},
		{"form_input", `<form action="https://evil.example"><input name="pw"></form>`},
		{"unclosed_tag", `<p>unclosed paragraph`},
		{"mismatched_tags", `<p><b>mismatched</p></b>`},
		{"html_comment", `<!-- hidden comment -->`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if liveedit.IsSafeHTMLBlock(tc.html) {
				t.Fatalf("IsSafeHTMLBlock(%q) = true, want false", tc.html)
			}
			doc := "# Heading\n\n" + tc.html + "\n\nTail\n"
			h := open(t, doc)
			h.caret(0) // caret on "# Heading", leaving the HTML block inactive
			if h.e.HTMLRendered(1) {
				t.Fatalf("unsafe HTML block %q was rendered instead of pale source", tc.html)
			}
			// Click and Cmd+Click inside the HTML block row: zero URLs may open.
			pt := h.e.RowText(1)
			h.tt.ClickAt(pt.X+12, pt.Y+6)
			h.tt.Frame()
			h.caret(0)
			h.tt.ClickAtWith(ui.Cmd, pt.X+12, pt.Y+6)
			h.tt.Frame()
			if urls := h.tt.OpenedURLs(); len(urls) != 0 {
				t.Fatalf("unsafe HTML block opened URLs: %v", urls)
			}

			// Edit the trailing paragraph and verify the HTML block bytes stay 100% intact.
			h.caret(len(doc) - 1)
			h.typed("!")
			want := "# Heading\n\n" + tc.html + "\n\nTail!\n"
			h.wantSrc(want)
		})
	}
}

// TestHTMLBlockSafeSubsetAndDetailsToggle verifies native rendering of safe
// README HTML blocks (centered image/link badges, headings, <details><summary>),
// disclosure toggling without mutating document bytes, and raw source reveal on
// caret entry (09 D8, FE-1.8).
func TestHTMLBlockSafeSubsetAndDetailsToggle(t *testing.T) {
	dir := t.TempDir()
	writeTestPNG(t, filepath.Join(dir, "badge.png"), 120, 28)

	badgeHTML := `<p align="center"><a href="https://example.com/docs"><img src="badge.png" alt="build passing" width="100"></a></p>`
	detailsHTML := "<details>\n<summary><strong>Release Notes</strong></summary>\n<p>Zero byte loss with <kbd>Ctrl+S</kbd>.</p>\n</details>"
	src := "# Project\n\n" + badgeHTML + "\n\n" + detailsHTML + "\n\nFooter\n"

	m := document.Factory{}.New("doc", src, contract.DocMeta{Dominant: "\n", Encoding: "utf-8"}, 0)
	ch := make(chan func(), 8)
	e := liveedit.New(liveedit.Options{
		Model:      m,
		Projector:  projection.New(),
		Editing:    editing.New(),
		Dispatcher: funcDispatcher(func(fn func()) { ch <- fn }),
		DocDir:     func() string { return dir },
	})
	t.Cleanup(e.Close)
	e.Focus()
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().Children(func() {
			e.View(c.Key("live"))
			e.Lightbox(c.Key("lb"))
		})
	}, 1000, 800)
	m.SetSelection(contract.Caret(0))
	tt.Frame()
	pumpAll(t, ch, 1)
	tt.Frame()

	if !liveedit.IsSafeHTMLBlock(badgeHTML) || !liveedit.IsSafeHTMLBlock(detailsHTML) {
		t.Fatal("expected safe HTML blocks to pass IsSafeHTMLBlock")
	}
	if !e.HTMLRendered(1) || !e.HTMLRendered(2) {
		t.Fatalf("HTMLRendered = (%v, %v), want (true, true)", e.HTMLRendered(1), e.HTMLRendered(2))
	}
	if e.HTMLDetailsOpen(2, 0) {
		t.Fatal("details without open attribute started expanded")
	}

	// Click <summary> to expand <details>: height grows, document bytes & version do not change.
	verBefore := m.Version()
	hClosed := e.RowHeight(2)
	sumPt := e.RowText(2)
	tt.ClickAt(sumPt.X+24, sumPt.Y+14)
	tt.Frame()
	if !e.HTMLDetailsOpen(2, 0) {
		t.Fatal("clicking <summary> did not expand <details>")
	}
	if hOpen := e.RowHeight(2); hOpen <= hClosed {
		t.Fatalf("expanded height %v <= closed height %v", hOpen, hClosed)
	}
	if m.Version() != verBefore || m.Text().String() != src {
		t.Fatal("toggling <details> mutated the document model")
	}

	// Cmd+Click on the centered badge opens https://example.com/docs.
	badgePt := e.RowText(1)
	// Row width is e.ColumnWidth; centered 100px badge sits around the middle of the column.
	tt.ClickAtWith(ui.Cmd, 500, badgePt.Y+14)
	tt.Frame()
	if urls := tt.OpenedURLs(); len(urls) != 1 || urls[0] != "https://example.com/docs" {
		t.Fatalf("OpenedURLs = %v, want [https://example.com/docs]", urls)
	}

	// Move caret inside <details>: reveals pale raw source; moving caret out re-renders HTML.
	detailsOff := strings.Index(src, "<summary>")
	m.SetSelection(contract.Caret(detailsOff + 2))
	tt.Frame()
	if e.HTMLRendered(2) {
		t.Fatal("active HTML block did not reveal raw source")
	}
	m.SetSelection(contract.Caret(0))
	tt.Frame()
	if !e.HTMLRendered(2) || !e.HTMLDetailsOpen(2, 0) {
		t.Fatal("leaving HTML block did not restore rendered open state")
	}
	if m.Text().String() != src {
		t.Fatal("source bytes changed")
	}
}

type funcDispatcher func(func())

func (f funcDispatcher) Post(fn func()) { f(fn) }

// TestSingleClickImageKeepsPreviewUnderSource verifies 09 D9: clicking a
// standalone image paragraph reveals its Markdown source while keeping the
// loaded image preview visible underneath (and double-clicking opens the
// Lightbox), whereas inline images mixed with text do not add a block preview.
func TestSingleClickImageKeepsPreviewUnderSource(t *testing.T) {
	dir := t.TempDir()
	writeTestPNG(t, filepath.Join(dir, "cat.png"), 140, 96)

	src := "# Gallery\n\n![cat](cat.png)\n\nSee ![inline](cat.png) here.\n\nEnd\n"
	m := document.Factory{}.New("doc", src, contract.DocMeta{Dominant: "\n", Encoding: "utf-8"}, 0)
	ch := make(chan func(), 8)
	e := liveedit.New(liveedit.Options{
		Model:      m,
		Projector:  projection.New(),
		Editing:    editing.New(),
		Dispatcher: funcDispatcher(func(fn func()) { ch <- fn }),
		DocDir:     func() string { return dir },
	})
	t.Cleanup(e.Close)
	e.Focus()
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().Children(func() {
			e.View(c.Key("live"))
			e.Lightbox(c.Key("lb"))
		})
	}, 1000, 800)
	m.SetSelection(contract.Caret(0))
	tt.Frame()
	pumpAll(t, ch, 1)
	tt.Frame()

	// Before clicking row 1: unrevealed standalone image, no separate source preview.
	if _, ok := e.PreviewImageBox(1); ok {
		t.Fatal("unrevealed standalone image reported a source-underneath preview")
	}
	if e.RowHeight(1) < 96 {
		t.Fatalf("unrevealed image row height = %v, want >= 96", e.RowHeight(1))
	}

	// Single-click the standalone image in row 1: caret enters row 1, source reveals,
	// AND PreviewImageBox(1) stays visible below the source line!
	pt := e.RowText(1)
	tt.ClickAt(pt.X+30, pt.Y+30)
	tt.Frame()
	imgStart := strings.Index(src, "![cat](cat.png)")
	imgEnd := imgStart + len("![cat](cat.png)")
	caret := m.Selection().Primary().Head
	if caret < imgStart || caret > imgEnd {
		t.Fatalf("caret = %d, want inside [%d, %d]", caret, imgStart, imgEnd)
	}
	box, ok := e.PreviewImageBox(1)
	if !ok {
		t.Fatal("standalone image paragraph lost its image preview when source was revealed")
	}
	if box.W != 140 || box.H != 96 || box.Y <= pt.Y {
		t.Fatalf("unexpected PreviewImageBox: %+v (row text Y=%v)", box, pt.Y)
	}

	// Editing the alt text keeps the preview live underneath.
	m.SetSelection(contract.Caret(imgStart + len("![cat")))
	tt.Frame()
	tt.Type(" photo")
	tt.Frame()
	if _, ok := e.PreviewImageBox(1); !ok {
		t.Fatal("preview disappeared while typing alt text")
	}

	// Double-clicking the preview image underneath opens the Lightbox!
	pbox, ok := e.PreviewImageBox(1)
	if !ok {
		t.Fatal("missing preview box before double-click")
	}
	cx, cy := pbox.X+pbox.W/2, pbox.Y+pbox.H/2
	tt.ClickAt(cx, cy)
	tt.Frame()
	tt.ClickAt(cx, cy)
	tt.Advance(400 * time.Millisecond)
	tt.Frame()
	if open, _ := e.LightboxOpen(); !open {
		t.Fatal("double-clicking the preview image under source did not open Lightbox")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Advance(400 * time.Millisecond)
	tt.Frame()

	// Moving caret to an inline image mixed with prose (row 2) does NOT create a block preview.
	inlineOff := strings.Index(m.Text().String(), "![inline]")
	m.SetSelection(contract.Caret(inlineOff + 3))
	tt.Frame()
	if _, ok := e.PreviewImageBox(2); ok {
		t.Fatal("inline image mixed with prose should not show a block preview")
	}
	// And row 1 (now inactive again) returns to its standalone image without preview.
	if _, ok := e.PreviewImageBox(1); ok {
		t.Fatal("inactive image row kept source preview")
	}
	if err := e.CheckRows(); err != nil {
		t.Fatal(err)
	}
}
