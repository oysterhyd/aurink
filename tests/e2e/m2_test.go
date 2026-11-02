//go:build e2e

package e2e

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"aurink/locales"
)

// The M2 flow in the real app and a real window (07 §6, QA-2.2), driven over
// MyGo's automation plugin: open a folder, quick-open a file, type, autosave,
// an external change while edited, Keep Mine, close, and a second start that
// brings the folder and tab back in the remembered window size.
//
//	go test -tags e2e ./tests/e2e -run TestM2Flow -v
//
// It opens windows on the desktop for about half a minute. Native file
// dialogs are not reached: the folder comes as an argument, as the system
// hands one over.

var en = locales.In("en")

type node struct {
	ID     string `json:"id"`
	Role   string `json:"role"`
	Key    any    `json:"key"`
	Label  string `json:"label"`
	Value  string `json:"value"`
	Bounds struct {
		X, Y, Width, Height float64
	} `json:"bounds"`
}

type driver struct {
	t    *testing.T
	cmd  *exec.Cmd
	conn net.Conn
	r    *bufio.Reader
	id   int
	win  int
	done chan error
}

func build(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "aurink-e2e")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	out, err := exec.Command("go", "build", "-tags", "e2e", "-o", exe, "aurink").CombinedOutput()
	if err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return exe
}

func launch(t *testing.T, exe, data string, args ...string) *driver {
	t.Helper()
	sock := filepath.Join(data, "auto.sock")
	cmd := exec.Command(exe, args...)
	cmd.Env = append(os.Environ(), "MYGO_AUTOMATION="+sock, "AURINK_USER_DATA="+data, "MYGO_LOCALE=en")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	d := &driver{t: t, cmd: cmd, done: make(chan error, 1)}
	go func() { d.done <- cmd.Wait() }()
	t.Cleanup(func() {
		if d.cmd.ProcessState == nil {
			_ = d.cmd.Process.Kill()
			<-d.done
		}
	})
	deadline := time.Now().Add(30 * time.Second)
	for {
		conn, err := net.Dial("unix", sock)
		if err == nil {
			d.conn, d.r = conn, bufio.NewReader(conn)
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no automation socket: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	d.until("a window", func() bool {
		var res struct {
			Windows []struct {
				ID int `json:"id"`
			} `json:"windows"`
		}
		d.call("windows", nil, &res)
		if len(res.Windows) > 0 {
			d.win = res.Windows[0].ID
			return true
		}
		return false
	})
	return d
}

func (d *driver) call(method string, params map[string]any, out any) {
	d.t.Helper()
	d.id++
	line, _ := json.Marshal(map[string]any{"id": d.id, "method": method, "params": params})
	if _, err := d.conn.Write(append(line, '\n')); err != nil {
		d.t.Fatal(err)
	}
	_ = d.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	answer, err := d.r.ReadBytes('\n')
	if err != nil {
		d.t.Fatalf("%s: %v", method, err)
	}
	var res struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code, Message string
		} `json:"error"`
	}
	if err := json.Unmarshal(answer, &res); err != nil {
		d.t.Fatal(err)
	}
	if res.Error != nil {
		d.t.Fatalf("%s: %s: %s", method, res.Error.Code, res.Error.Message)
	}
	if out != nil {
		if err := json.Unmarshal(res.Result, out); err != nil {
			d.t.Fatal(err)
		}
	}
}

func (d *driver) with(p map[string]any) map[string]any {
	p["window"] = d.win
	return p
}

func (d *driver) idle() { d.call("idle", d.with(map[string]any{"timeout": 10000}), nil) }

func (d *driver) tree() []node {
	d.t.Helper()
	var res struct {
		Nodes []node `json:"nodes"`
	}
	d.call("tree", d.with(map[string]any{}), &res)
	return res.Nodes
}

// find returns the node with the label (and role, if given); the rightmost
// when several match.
func (d *driver) find(label, role string) (node, bool) {
	var best node
	found := false
	for _, n := range d.tree() {
		if n.Label == label && (role == "" || n.Role == role) && (!found || n.Bounds.X > best.Bounds.X) {
			best, found = n, true
		}
	}
	return best, found
}

func (d *driver) until(what string, cond func() bool) {
	d.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			d.dump()
			d.t.Fatalf("never: %s", what)
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func (d *driver) dump() {
	var b strings.Builder
	for _, n := range d.tree() {
		fmt.Fprintf(&b, "  %s %q %v\n", n.Role, n.Label, n.Key)
	}
	d.t.Logf("the window shows:\n%s", b.String())
}

func (d *driver) press(label, role string) {
	d.t.Helper()
	d.pressOnly(label, role)
	d.idle()
}

// pressOnly presses and does not wait: the window may be gone after it.
func (d *driver) pressOnly(label, role string) {
	d.t.Helper()
	var n node
	d.until("a "+role+" "+label, func() bool {
		var ok bool
		n, ok = d.find(label, role)
		return ok
	})
	d.call("act", d.with(map[string]any{"id": n.ID, "action": "press"}), nil)
}

func (d *driver) input(events ...map[string]any) {
	d.t.Helper()
	d.call("input", d.with(map[string]any{"events": events}), nil)
	d.idle()
}

func key(k string) map[string]any        { return map[string]any{"type": "key", "key": k} }
func text(s string) map[string]any       { return map[string]any{"type": "text", "text": s} }
func shows(d *driver, label string) bool { _, ok := d.find(label, ""); return ok }

func (d *driver) exited() {
	d.t.Helper()
	select {
	case err := <-d.done:
		if err != nil {
			d.t.Fatalf("the app ended with %v", err)
		}
	case <-time.After(20 * time.Second):
		d.t.Fatal("the app did not quit")
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestM2Flow(t *testing.T) {
	exe := build(t)
	data, proj := t.TempDir(), filepath.Join(t.TempDir(), "proj")
	a := filepath.Join(proj, "a.md")
	for p, s := range map[string]string{a: "# A\n\nfirst\n", filepath.Join(proj, "notes", "b.md"): "b\n"} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A remembered window size, which the window must take (StateKey "main").
	state := `{"main": {"x": 120, "y": 90, "width": 1000, "height": 680}}`
	if err := os.WriteFile(filepath.Join(data, "window-state.json"), []byte(state), 0o644); err != nil {
		t.Fatal(err)
	}

	d := launch(t, exe, data, proj)
	var wins struct {
		Windows []struct {
			Bounds struct{ Width, Height float64 } `json:"bounds"`
		} `json:"windows"`
	}
	d.call("windows", nil, &wins)
	if w := wins.Windows[0].Bounds; w.Width < 990 || w.Width > 1010 || w.Height < 670 || w.Height > 690 {
		t.Fatalf("the window is %v×%v, not the remembered 1000×680", w.Width, w.Height)
	}
	d.until("the folder in the tree", func() bool { return shows(d, "notes") })

	// Quick open, type, autosave.
	d.input(key("CmdOrCtrl+P"))
	d.input(text("a.md"))
	d.until("the file in quick open", func() bool { return shows(d, "a.md") })
	d.input(key("Enter"))
	d.until("the tab", func() bool { _, ok := d.find("a.md", "tab"); return ok })
	d.input(key("CmdOrCtrl+End"), text(" mine"))
	d.until("the autosave", func() bool { return strings.Contains(readFile(t, a), "first mine") })

	// Edited again, and changed on disk meanwhile: the conflict banner.
	d.input(text("!"))
	if err := os.WriteFile(a, []byte("theirs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d.press(en.ShellKeepMine(), "button")
	d.until("mine on disk", func() bool { return readFile(t, a) == "# A\n\nfirst mine!\n" })

	// Close; the next start has the folder and the tab again.
	d.pressOnly(en.ShellClose(), "button")
	d.exited()
	if !strings.Contains(readFile(t, filepath.Join(data, "session.json")), "a.md") {
		t.Fatal("the session was not saved")
	}

	d = launch(t, exe, data)
	d.until("the folder again", func() bool { return shows(d, "notes") })
	d.until("the tab again", func() bool { _, ok := d.find("a.md", "tab"); return ok })
	d.pressOnly(en.ShellClose(), "button")
	d.exited()
}
