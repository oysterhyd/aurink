//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// answer runs dialog.ps1 against the app's next native dialog and logs what
// the dialog showed.
func (d *driver) answer(args ...string) string {
	d.t.Helper()
	script, _ := filepath.Abs("dialog.ps1")
	cmd := exec.Command("powershell.exe", append([]string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script,
		"-ProcId", itoa(d.cmd.Process.Pid)}, args...)...)
	out, err := cmd.CombinedOutput()
	d.t.Logf("dialog:\n%s", out)
	if err != nil {
		d.dump()
		d.t.Fatalf("dialog: %v", err)
	}
	return string(out)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

// send gives keys without waiting for the window to settle: a dialog may be
// about to open over it.
func (d *driver) send(events ...map[string]any) {
	d.t.Helper()
	d.call("input", d.with(map[string]any{"events": events}), nil)
}

// TestNativeDialogs drives the real Open, Save, Export and unsaved-changes
// dialogs of Windows (07 §6 INT-2.1: the owner checks the dialogs).
func TestNativeDialogs(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("dialog.ps1 uses Windows UI Automation")
	}
	exe := build(t)
	data, dir := t.TempDir(), t.TempDir()
	other := filepath.Join(dir, "other.md")
	if err := os.WriteFile(other, []byte("# Other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d := launch(t, exe, data)
	d.idle()

	// Save: a new document asks where.
	saved := filepath.Join(dir, "saved.md")
	d.input(key("CmdOrCtrl+N"))
	d.input(text("hello"))
	d.send(key("CmdOrCtrl+S"))
	d.answer("-Path", saved)
	d.until("the file saved", func() bool { b, err := os.ReadFile(saved); return err == nil && string(b) == "hello" })
	d.until("the tab named", func() bool { _, ok := d.find("saved.md", "tab"); return ok })

	// Open.
	d.send(key("CmdOrCtrl+O"))
	d.answer("-Path", other)
	d.until("the opened tab", func() bool { _, ok := d.find("other.md", "tab"); return ok })

	// Export as HTML, from the palette; the dialog offers name.html.
	page := filepath.Join(dir, "page.html")
	d.input(key("CmdOrCtrl+Shift+P"))
	d.input(text(en.ShellExportHtml()[:6]))
	d.send(key("Enter"))
	if out := d.answer("-Path", page); !strings.Contains(out, "default name: other.html") {
		t.Fatalf("the export dialog did not offer other.html")
	}
	d.until("the page", func() bool {
		b, err := os.ReadFile(page)
		return err == nil && strings.Contains(string(b), "<h1") && strings.Contains(string(b), "Other")
	})

	// Closing an unsaved document asks: Cancel keeps it, Don't Save closes it.
	d.input(key("CmdOrCtrl+N"))
	d.input(text("draft"))
	d.send(key("CmdOrCtrl+W"))
	if out := d.answer("-Button", en.ShellCancel()); !strings.Contains(out, en.ShellSave()) || !strings.Contains(out, en.ShellDiscard()) {
		t.Fatalf("the close dialog lacks Save or Discard")
	}
	d.idle()
	tabs := func() int {
		n := 0
		for _, x := range d.tree() {
			if x.Role == "tab" {
				n++
			}
		}
		return n
	}
	if tabs() != 3 {
		d.dump()
		t.Fatalf("%d tabs after Cancel, want 3", tabs())
	}
	d.send(key("CmdOrCtrl+W"))
	d.answer("-Button", en.ShellDiscard())
	d.until("the draft closed", func() bool { return tabs() == 2 })

	d.pressOnly(en.ShellClose(), "button")
	d.exited()
}
