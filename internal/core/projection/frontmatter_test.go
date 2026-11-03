package projection

import (
	"reflect"
	"testing"

	"aurink/internal/contract"
)

func TestFrontMatterSubset(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		want         *contract.FrontMatterMeta
	}{
		{"empty", "# comment\n\n", &contract.FrontMatterMeta{}},
		{"scalars and flow list", "title: 中文标题 # comment\ntags: [one, 'two, three', \"四\"]\ndate: 2026-10-31\n", &contract.FrontMatterMeta{Kicker: "中文标题", Tags: []string{"one", "two, three", "四"}, Date: "2026-10-31"}},
		{"block list and fallback names", "kicker: 'It''s fine'\nkeywords:\n  - a\n  # comment\n  - \"b c\"\n", &contract.FrontMatterMeta{Kicker: "It's fine", Tags: []string{"a", "b c"}}},
		{"plain apostrophe", "title: John's notes\n", &contract.FrontMatterMeta{Kicker: "John's notes"}},
		{"priority", "kicker: second\ntitle: first\nkeywords: [x]\ntags: [y]\n", &contract.FrontMatterMeta{Kicker: "first", Tags: []string{"y"}}},
		{"quoted hash", "title: \"a # b\"\n", &contract.FrontMatterMeta{Kicker: "a # b"}},
		{"empty list", "tags: []\n", &contract.FrontMatterMeta{Tags: []string{}}},
		{"unclosed list", "tags: [a, b\n", nil},
		{"nested map", "outer:\n  inner: value\n", nil},
		{"indented top-level key", " title: value\n", nil},
		{"tab-indented top-level key", "\ttitle: value\n", nil},
		{"flow map", "outer: {a: b}\n", nil},
		{"anchor", "title: &ref thing\n", nil},
		{"alias", "title: *ref\n", nil},
		{"multiline scalar", "title: |\n  content\n", nil},
		{"unclosed quote", "title: 'a\n", nil},
		{"stray line", "something else\n", nil},
		{"orphan list", "  - item\n", nil},
		{"duplicate key", "title: a\ntitle: b\n", nil},
		{"non-scalar title", "title: [a]\n", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseFrontMatter(tc.source); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestFrontMatterSignatureIncludesMeta(t *testing.T) {
	a := contract.BlockProjection{Segments: []contract.Segment{{Kind: contract.SegObject, Object: &contract.ObjectRef{Kind: contract.ObjFrontMatter, Meta: &contract.FrontMatterMeta{Kicker: "one", Tags: []string{"a"}}}}}}
	b := contract.BlockProjection{Segments: []contract.Segment{{Kind: contract.SegObject, Object: &contract.ObjectRef{Kind: contract.ObjFrontMatter, Meta: &contract.FrontMatterMeta{Kicker: "two", Tags: []string{"a"}}}}}}
	if signature(&a) == signature(&b) {
		t.Fatal("metadata changes must invalidate the projection signature")
	}
}
