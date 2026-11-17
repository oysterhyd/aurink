package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRollingLogRotatesAndCapsFiles(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "logs", "aurink.log")
	rl, err := NewRollingLog(logPath, 256, 5)
	if err != nil {
		t.Fatal(err)
	}
	defer rl.Close()

	// Write enough lines to trigger >6 rotations; only aurink.log + .1..4 may remain.
	for i := range 40 {
		line := fmt.Sprintf("entry-%02d %s\n", i, strings.Repeat("x", 40))
		if _, err := rl.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	if err := rl.Close(); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"aurink.log", "aurink.log.1", "aurink.log.2", "aurink.log.3", "aurink.log.4"} {
		info, err := os.Stat(filepath.Join(dir, "logs", name))
		if err != nil {
			t.Fatalf("expected %s to exist: %v", name, err)
		}
		if info.Size() == 0 || info.Size() > 256 {
			t.Fatalf("%s size = %d, want (0, 256]", name, info.Size())
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "logs", "aurink.log.5")); !os.IsNotExist(err) {
		t.Fatalf("expected aurink.log.5 to be pruned, got err=%v", err)
	}
	latest, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(latest), "entry-39") {
		t.Fatalf("active log missing latest entry: %q", string(latest))
	}
}
