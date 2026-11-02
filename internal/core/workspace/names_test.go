package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"aurink/internal/contract"
	"aurink/internal/contracttest"
)

func TestWindowsInvalidNames(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows filename rules")
	}
	root := t.TempDir()
	for name, text := range map[string]string{"a.md": "base", "source.md": "source"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	f := &fixture{t: t, q: newQueue()}
	w := New(&contracttest.SnapshotWatcher{}, &contracttest.RecordingTrasher{}, f.q, Options{})
	f.spaces = append(f.spaces, w)
	t.Cleanup(func() { w.Close(nil); f.settle() })
	var events []contract.WorkspaceEvent
	w.Subscribe(func(event contract.WorkspaceEvent) { events = append(events, event) })
	w.SetRoot(root, func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	})
	f.settle()
	events = nil

	tests := []struct{ label, name string }{
		{"alternate_stream", "a.md:hidden"},
		{"trailing_dot", "name."},
		{"trailing_space", "name "},
		{"mixed_case_device", "cOm1.notes.md"},
		{"device_space_before_extension", "NUL .md"},
	}
	for _, char := range `<>:"|?*` {
		tests = append(tests, struct{ label, name string }{fmt.Sprintf("character_%02x", char), "bad" + string(char) + ".md"})
	}
	for char := 0; char < 32; char++ {
		tests = append(tests, struct{ label, name string }{fmt.Sprintf("control_%02d", char), "bad" + string(rune(char)) + ".md"})
	}
	devices := []string{"CON", "PRN", "AUX", "NUL"}
	for n := 1; n <= 9; n++ {
		devices = append(devices, fmt.Sprintf("COM%d", n), fmt.Sprintf("LPT%d", n))
	}
	for _, device := range devices {
		tests = append(tests,
			struct{ label, name string }{"device_" + device, device},
			struct{ label, name string }{"device_extension_" + device, strings.ToLower(device) + ".md"})
	}
	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			for _, isDir := range []bool{false, true} {
				var path string
				var got error
				called := 0
				w.Create(root, test.name, isDir, func(p string, err error) { path, got, called = p, err, called+1 })
				f.settle()
				if called != 1 || path != "" || contract.CodeOf(got) != contract.CodeInvalid {
					t.Fatalf("Create(%q, isDir=%v) = %q, %v, called %d times", test.name, isDir, path, got, called)
				}
			}
			var path string
			var got error
			called := 0
			w.Rename(filepath.Join(root, "source.md"), test.name, func(p string, err error) { path, got, called = p, err, called+1 })
			f.settle()
			if called != 1 || path != "" || contract.CodeOf(got) != contract.CodeInvalid {
				t.Fatalf("Rename(%q) = %q, %v, called %d times", test.name, path, got, called)
			}
			if len(events) != 0 {
				t.Fatalf("invalid operations emitted events: %+v", events)
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, entry := range entries {
				names = append(names, entry.Name())
			}
			if !reflect.DeepEqual(names, []string{"a.md", "source.md"}) {
				t.Fatalf("directory changed: %q", names)
			}
			for name, want := range map[string]string{"a.md": "base", "source.md": "source"} {
				if data, err := os.ReadFile(filepath.Join(root, name)); err != nil || string(data) != want {
					t.Fatalf("%s changed: %q, %v", name, data, err)
				}
			}
			if _, err := os.Stat(filepath.Join(root, "a.md:hidden")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("alternate stream exists or could not be checked: %v", err)
			}
		})
	}
}

func TestWindowsValidNames(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows filename rules")
	}
	for _, name := range []string{"COM0.md", "COM10.md", "LPT0.md", "LPT10.md", "CONSOLE.md", "NUL-file.md", "file .md"} {
		if !validName(name) {
			t.Errorf("validName(%q) = false", name)
		}
	}
}

func TestWindowsInvalidAssetName(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows filename rules")
	}
	root := t.TempDir()
	assets := NewAssets("./assets", true)
	for _, name := range []string{"a:hidden", "NUL.md", "image."} {
		if path, err := assets.ImportImage(filepath.Join(root, "doc.md"), []byte("png"), name); path != "" || contract.CodeOf(err) != contract.CodeInvalid {
			t.Errorf("ImportImage(%q) = %q, %v", name, path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "assets")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid image names created the asset directory: %v", err)
	}
}

func TestNonWindowsNameRules(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("non-Windows filename rules")
	}
	for _, name := range []string{"a.md:hidden", "x<y", "x\x01y", "file.", "file ", "NUL.md", "COM1"} {
		if !validName(name) {
			t.Errorf("validName(%q) = false", name)
		}
	}
}
