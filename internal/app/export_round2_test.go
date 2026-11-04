package app

import (
	"strings"
	"testing"
	"time"

	"aurink/internal/contract"
	"aurink/internal/mock"
)

type exportSettings struct{ mode contract.SoftBreakMode }

func (s *exportSettings) Get() contract.Settings {
	return contract.Settings{Editor: contract.EditorSettings{SoftBreak: s.mode}}
}
func (s *exportSettings) Update(fn func(*contract.Settings)) error {
	v := s.Get()
	fn(&v)
	s.mode = v.Editor.SoftBreak
	return nil
}
func (*exportSettings) Subscribe(func(contract.Settings)) func() { return func() {} }

func TestExportAndCopyUseCurrentSoftBreak(t *testing.T) {
	settings := &exportSettings{}
	files := mock.NewFileStore()
	copyHTML := clipHTML(contract.ExtDefault, settings)
	exportHTML := ExportHTML(files, mock.Immediate{}, contract.ExtDefault, settings)
	for _, tc := range []struct {
		mode contract.SoftBreakMode
		want string
	}{
		{contract.SoftBreakNewline, "one<br />\ntwo"},
		{contract.SoftBreakSpace, "one\ntwo"},
	} {
		settings.mode = tc.mode
		if got := copyHTML("one\ntwo"); !strings.Contains(got, tc.want) {
			t.Errorf("copy mode %v: %q", tc.mode, got)
		}
		done := make(chan error, 1)
		exportHTML("one\ntwo", "doc.html", contract.ExportOptions{}, func(err error) { done <- err })
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("export did not finish")
		}
		data, ok := files.Bytes("doc.html")
		if !ok || !strings.Contains(string(data), tc.want) {
			t.Errorf("export mode %v: %q", tc.mode, data)
		}
	}
}
