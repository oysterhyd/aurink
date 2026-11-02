package trash

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"aurink/internal/contract"
	"github.com/egoist/mygo"
)

var desktopAvailable = runtime.GOOS != "linux" || os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""

func TestMain(m *testing.M) {
	if !desktopAvailable {
		os.Exit(m.Run())
	}
	result := make(chan int, 1)
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.WhenReady(func() { go func() { result <- m.Run(); mygo.App.Quit() }() })
	if err := mygo.App.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(<-result)
}

func TestMissing(t *testing.T) {
	err := New().Trash(filepath.Join(t.TempDir(), "missing"))
	if contract.CodeOf(err) != contract.CodeNotFound {
		t.Fatalf("missing item: %v", err)
	}
}

func TestTrashTemporaryFile(t *testing.T) {
	if !desktopAvailable {
		t.Skip("system trash requires a desktop session")
	}
	path := filepath.Join(t.TempDir(), "trash-test.txt")
	if err := os.WriteFile(path, []byte("temporary test file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := New().Trash(path); err != nil {
		t.Fatalf("trash temporary file: %v", err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("item remains at source: %v", err)
	}
}
