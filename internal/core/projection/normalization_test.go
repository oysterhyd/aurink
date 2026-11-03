package projection

import (
	"strings"
	"testing"

	"aurink/internal/contract"
)

func TestCodeNormalization(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"`a\r\nb`", "a b"},
		{"`\na\n`", "a"},
		{"> `\n> `", " "},
		{"> ` \n>  `", "  "},
		{"> ` a\n> `", "a"},
		{"> `\r\n> a \r\n> `", "a "},
		{"| `a\\|b` |\n|---|\n", "a|b"},
		{"| `` \\| `` |\n|---|\n", "|"},
		{"> -\t\tcode\n", "code"},
	} {
		tx, a := parse(t, tc.src)
		leaf := a.LeafAt(0)
		p := New().Project(a, tx, leaf, contract.Reveal{Caret: -1})
		if got := New().DisplayText(&p); got != tc.want {
			t.Errorf("%q: %q, want %q; block %+v inlines %+v", tc.src, got, tc.want, a.Block(leaf), a.Inlines(leaf))
		}
	}
}

func TestBreakAndImageDetails(t *testing.T) {
	for _, src := range []string{"a\\\nb", "a  \r\nb"} {
		tx, a := parse(t, src)
		p := New().Project(a, tx, a.LeafAt(0), contract.Reveal{Mode: contract.RevealAlways, Caret: -1})
		want := strings.ReplaceAll(src, "\r\n", "\n")
		if got := New().DisplayText(&p); got != want {
			t.Errorf("revealed hard break %q, want %q", got, want)
		}
	}
	tx, a := parse(t, "![a\nb](x.png)")
	p := New().Project(a, tx, a.LeafAt(0), contract.Reveal{Caret: -1})
	found := false
	for _, s := range p.Segments {
		if s.Object != nil {
			found = true
			if s.Object.Alt != "a b" {
				t.Errorf("alt %q", s.Object.Alt)
			}
		}
	}
	if !found {
		t.Fatal("missing image")
	}
}

func TestM1Scope(t *testing.T) {
	for _, tc := range []struct {
		src, want string
		exts      contract.Extensions
		object    contract.ObjectKind
	}{
		{"==hi== ^up^ :smile: [[wiki]] $x$", "==hi== ^up^ :smile: [[wiki]] $x$", 0, contract.ObjNone},
		{"\\(x\\)", "\ufffc", 0, contract.ObjMath},
		{"$$\nx\n$$", "\ufffc", 0, contract.ObjMath},
		{"```mermaid\na-->b\n```", "\ufffc", 0, contract.ObjDiagram},
		{"[TOC]", "\ufffc", contract.ExtTOC, contract.ObjTOC},
		{"---\ntitle: hello\n---\n", "\ufffc", contract.ExtFrontMatter, contract.ObjFrontMatter},
	} {
		tx, a := parseExts(t, tc.src, tc.exts)
		p := New().Project(a, tx, a.LeafAt(0), contract.Reveal{Caret: -1, Exts: contract.ExtDefault, ObjectLangs: []string{"mermaid"}})
		if got := New().DisplayText(&p); got != tc.want {
			t.Errorf("source %q: %q, want %q", tc.src, got, tc.want)
		}
		objects := 0
		for _, s := range p.Segments {
			if s.Object != nil {
				objects++
				if s.Kind != contract.SegObject || s.Object.Kind != tc.object {
					t.Errorf("unexpected object: %+v", s)
				}
			}
		}
		if (objects == 1) != (tc.object != contract.ObjNone) {
			t.Errorf("source %q: got %d objects", tc.src, objects)
		}
	}
}

func TestTrailingSpaceNormalization(t *testing.T) {
	for _, src := range []string{"a \nb", "> a \n> b"} {
		tx, a := parse(t, src)
		p := New().Project(a, tx, a.LeafAt(0), contract.Reveal{Caret: -1})
		if got := New().DisplayText(&p); got != "a\nb" {
			t.Errorf("%q: %q; inlines %+v", src, got, a.Inlines(a.LeafAt(0)))
		}
	}
}
