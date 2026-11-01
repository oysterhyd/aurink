package e2e

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aurink/internal/contract"
	"aurink/internal/core/docsession"
	"aurink/internal/core/document"
	"aurink/internal/core/fsstore"
	"aurink/internal/core/settings"
)

const recoveredText = "start\r\nunsaved 中\r\n"

type pipeDispatcher chan func()

func (d pipeDispatcher) Post(fn func()) { d <- fn }

type manualTimer struct{ fn func() }

func (t *manualTimer) Stop() bool {
	active := t.fn != nil
	t.fn = nil
	return active
}

type recoveryStore struct {
	contract.FileStore
	persisted chan struct{}
}

func (s recoveryStore) Write(path, text string, meta contract.DocMeta, expect *contract.FileStamp) (contract.FileStamp, error) {
	stamp, err := s.FileStore.Write(path, text, meta, expect)
	if err == nil && strings.HasSuffix(path, ".json") {
		s.persisted <- struct{}{}
	}
	return stamp, err
}

type restoredResult struct {
	Text  string
	State contract.DocState
	Meta  contract.DocMeta
}

func TestRecoveryProcess(t *testing.T) {
	mode := os.Getenv("AURINK_DOCSESSION_HELPER")
	if mode == "" {
		return
	}
	if err := runRecoveryProcess(mode, os.Getenv("AURINK_DOCSESSION_DIR")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(0)
}

func runRecoveryProcess(mode, dir string) error {
	dispatcher := make(pipeDispatcher, 64)
	persisted := make(chan struct{}, 8)
	files := recoveryStore{FileStore: fsstore.New(), persisted: persisted}
	value := settings.Defaults()
	value.Files.Autosave = false
	var timers []*manualTimer
	m := docsession.New(files, document.Factory{}, dispatcher, filepath.Join(dir, "recovery"), docsession.Options{
		Settings: &value, RecoveryDelay: time.Millisecond,
		AfterFunc: func(_ time.Duration, fn func()) docsession.Timer {
			timer := &manualTimer{fn: fn}
			timers = append(timers, timer)
			return timer
		},
	})
	if mode == "restore" {
		finished := false
		var result []restoredResult
		var restoreErr error
		m.Restore(func(sessions []contract.Session, err error) {
			restoreErr = err
			for _, s := range sessions {
				result = append(result, restoredResult{Text: s.Model().Text().String(), State: s.State(), Meta: s.Model().Meta()})
			}
			finished = true
		})
		for !finished {
			(<-dispatcher)()
		}
		if restoreErr != nil {
			return restoreErr
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	}
	var session contract.Session
	var openErr error
	finished := false
	m.Open(filepath.Join(dir, "document.md"), func(s contract.Session, err error) { session, openErr, finished = s, err, true })
	for !finished {
		(<-dispatcher)()
	}
	if openErr != nil {
		return openErr
	}
	end := session.Model().Text().Len()
	_, err := session.Model().Apply(contract.Transaction{Origin: contract.OriginUser,
		Edits: []contract.TextEdit{{Range: contract.Range{Start: end, End: end}, Text: "unsaved 中\r\n"}}})
	if err != nil {
		return err
	}
	for _, timer := range timers {
		if timer.fn != nil {
			timer.fn()
		}
	}
	for {
		select {
		case fn := <-dispatcher:
			fn()
		case <-persisted:
			// The FileStore acknowledged its atomic, synced write. The parent can
			// now terminate us without depending on a wall-clock delay.
			fmt.Fprintln(os.Stdout, "READY")
			_, err := io.Copy(io.Discard, os.Stdin)
			return err
		}
	}
}

func TestHardKillAndRestartRecovery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "document.md")
	if err := os.WriteFile(path, []byte("start\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := func(mode string) *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=^TestRecoveryProcess$")
		cmd.Env = append(os.Environ(), "AURINK_DOCSESSION_HELPER="+mode, "AURINK_DOCSESSION_DIR="+dir)
		return cmd
	}
	child := command("edit")
	stdin, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	child.Stderr = &stderr
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if child.ProcessState == nil {
			_ = child.Process.Kill()
			_ = child.Wait()
		}
	})
	ready, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || ready != "READY\n" {
		t.Fatalf("helper readiness %q: %v", ready, err)
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := child.Wait(); err == nil {
		t.Fatal("helper exited normally instead of being killed")
	}
	output, err := command("restore").Output()
	if err != nil {
		t.Fatalf("restart: %v; helper stderr: %s", err, stderr.String())
	}
	var got []restoredResult
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("restart output %q: %v", output, err)
	}
	if len(got) != 1 || got[0].Text != recoveredText || got[0].State.Path != path || !got[0].State.Edited || got[0].State.Conflict || got[0].Meta.Encoding != "utf-8" {
		t.Fatalf("recovered %+v", got)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "start\r\n" {
		t.Fatalf("recovery changed document bytes %q: %v", data, err)
	}
}
