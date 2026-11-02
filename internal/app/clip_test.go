package app

import (
	"strings"
	"testing"

	"aurink/internal/contract"
)

func TestClipHTMLIsAFragment(t *testing.T) {
	html := clipHTML(contract.ExtDefault)("a **bold** [link](https://x.org)")
	for _, want := range []string{"<strong>bold</strong>", `href="https://x.org"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("no %q in %q", want, html)
		}
	}
	if strings.Contains(html, "<html") || strings.Contains(html, "<style") {
		t.Fatalf("a whole page, not a fragment: %q", html)
	}
}
