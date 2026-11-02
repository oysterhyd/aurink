package clipboard

import (
	"path/filepath"
	"reflect"
	"testing"

	"aurink/internal/contract"
)

func TestRoundTrip(t *testing.T) {
	in := contract.ClipData{Text: "**b**", Markdown: "**b**", HTML: "<p><strong>b</strong></p>", RTF: `{\rtf1 b}`, PNG: []byte{0x89, 'P'}}
	if got := FromData(ToData(in)); !reflect.DeepEqual(got, in) {
		t.Fatalf("got %+v", got)
	}
	files := contract.ClipData{Files: []string{filepath.Join(t.TempDir(), "a.png")}}
	if got := FromData(ToData(files)); !reflect.DeepEqual(got.Files, files.Files) {
		t.Fatalf("files %+v", got.Files)
	}
	if got := FromData(ToData(contract.ClipData{})); !reflect.DeepEqual(got, contract.ClipData{}) {
		t.Fatalf("empty %+v", got)
	}
}
