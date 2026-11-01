package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"aurink/internal/contract"
	"aurink/internal/contracttest"
	"aurink/internal/core/fsstore"
)

func TestSettings(t *testing.T) {
	contracttest.TestSettings(t, func(t *testing.T) contracttest.SettingsFixture {
		path := filepath.Join(t.TempDir(), "settings.json")
		return contracttest.SettingsFixture{
			Open: func() contract.SettingsStore { return open(t, path, fsstore.New(), nil, nil) },
			Flush: func(s contract.SettingsStore) {
				if err := s.(*Store).Flush(); err != nil {
					t.Fatal(err)
				}
			},
		}
	})
}

type queued struct {
	mu    sync.Mutex
	queue []func()
}

func (q *queued) Post(fn func()) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queue = append(q.queue, fn)
}

func (q *queued) flush() {
	q.mu.Lock()
	batch := q.queue
	q.queue = nil
	q.mu.Unlock()
	for _, fn := range batch {
		fn()
	}
}

type files struct {
	contract.FileStore
	read  func(string, contract.ReadOptions) (contract.FileContent, error)
	write func(string, string, contract.DocMeta, *contract.FileStamp) (contract.FileStamp, error)
}

func (f *files) Read(path string, opts contract.ReadOptions) (contract.FileContent, error) {
	if f.read != nil {
		return f.read(path, opts)
	}
	return f.FileStore.Read(path, opts)
}

func (f *files) Write(path, text string, meta contract.DocMeta, expect *contract.FileStamp) (contract.FileStamp, error) {
	return f.write(path, text, meta, expect)
}

func open(t *testing.T, path string, f contract.FileStore, d contract.Dispatcher, handler func(error)) *Store {
	t.Helper()
	if d == nil {
		d = &queued{}
	}
	s, err := New(path, f, d, handler)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Flush() })
	return s
}

func put(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func contents(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func update(t *testing.T, s *Store, fn func(*contract.Settings)) {
	t.Helper()
	if err := s.Update(fn); err != nil {
		t.Fatal(err)
	}
}

func flush(t *testing.T, s *Store) {
	t.Helper()
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
}

func TestMissingAndPartialFiles(t *testing.T) {
	t.Run("missing parent created only on update", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "user", "settings.json")
		s := open(t, path, fsstore.New(), nil, nil)
		if !reflect.DeepEqual(s.Get(), Defaults()) {
			t.Fatalf("defaults: %+v", s.Get())
		}
		flush(t, s)
		if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
			t.Fatalf("constructor/Flush created directory: %v", err)
		}
		update(t, s, func(v *contract.Settings) { v.Language = "en" })
		flush(t, s)
		if got := open(t, path, fsstore.New(), nil, nil).Get().Language; got != "en" {
			t.Fatalf("language = %q", got)
		}
	})
	t.Run("missing fields preserve defaults including nested booleans", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "settings.json")
		data := []byte(`{"version":1,"appearance":{"theme":"dark","AccentHue":0,"future":123},"editor":{"AutoPair":false,"Exts":0},"files":{"Autosave":false,"RemoteImages":true},"future":{"x":1}}`)
		put(t, path, data)
		s := open(t, path, fsstore.New(), nil, nil)
		want := Defaults()
		want.Appearance.Theme, want.Appearance.AccentHue = "dark", 0
		want.Editor.AutoPair, want.Editor.Exts = false, 0
		want.Files.Autosave, want.Files.RemoteImages = false, true
		if got := s.Get(); !reflect.DeepEqual(got, want) {
			t.Fatalf("loaded %+v, want %+v", got, want)
		}
		flush(t, s)
		if !bytes.Equal(contents(t, path), data) {
			t.Fatal("load rewrote the file")
		}
	})
	t.Run("empty objects and null sections", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "settings.json")
		put(t, path, []byte(`{"Appearance":null,"Editor":{},"Files":null}`))
		if got := open(t, path, fsstore.New(), nil, nil).Get(); !reflect.DeepEqual(got, Defaults()) {
			t.Fatalf("loaded %+v", got)
		}
	})
}

func TestCorruptFilesAreBackedUp(t *testing.T) {
	for _, input := range []string{
		``, `{`, `null`, `[]`, `{"Version":-1}`, `{"Version":"one"}`,
		`{"Appearance":{"FontSize":"large"}}`, `{"Editor":{"Reveal":-1}}`,
		string([]byte{0xff, 0xff, 0xff}), string([]byte{'{', 0, '}'}),
	} {
		t.Run(input, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			put(t, path, []byte(input))
			put(t, path+".bad", []byte("first"))
			put(t, path+".bad.1", []byte("second"))
			s := open(t, path, fsstore.New(), nil, nil)
			if !reflect.DeepEqual(s.Get(), Defaults()) {
				t.Fatalf("fallback: %+v", s.Get())
			}
			flush(t, s)
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("corrupt original still present: %v", err)
			}
			if string(contents(t, path+".bad")) != "first" || string(contents(t, path+".bad.1")) != "second" ||
				string(contents(t, path+".bad.2")) != input {
				t.Fatal("backup bytes or existing backups changed")
			}
			update(t, s, func(v *contract.Settings) { v.Appearance.Theme = "light" })
			flush(t, s)
			if got := open(t, path, fsstore.New(), nil, nil).Get().Appearance.Theme; got != "light" {
				t.Fatalf("replacement theme = %q", got)
			}
		})
	}
	t.Run("first free name", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "settings.json")
		put(t, path, []byte("broken"))
		open(t, path, fsstore.New(), nil, nil)
		if string(contents(t, path+".bad")) != "broken" {
			t.Fatal("wrong backup")
		}
	})
}

func TestNewerVersionRequiresSuccessfulUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	data := []byte(`{"Version":999,"Appearance":["a future schema"],"Future":true}`)
	put(t, path, data)
	s := open(t, path, fsstore.New(), nil, nil)
	if !reflect.DeepEqual(s.Get(), Defaults()) {
		t.Fatal("newer version did not fall back to defaults")
	}
	for _, fn := range []func(*contract.Settings){nil, func(v *contract.Settings) {
		v.Appearance.Theme = "dark"
		panic("aborted edit")
	}} {
		if err := s.Update(fn); err == nil {
			t.Fatal("invalid update succeeded")
		}
		flush(t, s)
		if !bytes.Equal(contents(t, path), data) || !reflect.DeepEqual(s.Get(), Defaults()) {
			t.Fatal("unsuccessful update changed memory or overwrote a newer file")
		}
	}
	update(t, s, func(v *contract.Settings) { v.Appearance.Theme = "dark"; v.Version = 999 })
	flush(t, s)
	got := open(t, path, fsstore.New(), nil, nil).Get()
	if got.Version != contract.SettingsVersion || got.Appearance.Theme != "dark" {
		t.Fatalf("explicit replacement: %+v", got)
	}
	if _, err := os.Stat(path + ".bad"); !os.IsNotExist(err) {
		t.Fatalf("newer file treated as corrupt: %v", err)
	}
}

func TestUpdatesCoalesceAndFlushWaits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	var writes []contract.Settings
	f := &files{FileStore: fsstore.New(), write: func(gotPath, text string, meta contract.DocMeta, expect *contract.FileStamp) (contract.FileStamp, error) {
		if len(writes) == 0 {
			close(started)
			<-release
		}
		if gotPath != path || expect != nil || meta.Encoding != "utf-8" || meta.BOM {
			t.Errorf("write arguments: %q %+v %+v", gotPath, meta, expect)
		}
		var v contract.Settings
		if err := json.Unmarshal([]byte(text), &v); err != nil {
			t.Error(err)
		}
		writes = append(writes, v)
		return contract.FileStamp{}, nil
	}}
	s := open(t, path, f, nil, nil)
	update(t, s, func(v *contract.Settings) { v.Appearance.AccentHue = 1 })
	<-started
	for hue := 2; hue <= 100; hue++ {
		update(t, s, func(v *contract.Settings) { v.Appearance.AccentHue = hue })
	}
	done, entered := make(chan error, 1), make(chan struct{})
	go func() { close(entered); done <- s.Flush() }()
	<-entered
	select {
	case err := <-done:
		t.Fatalf("Flush finished while Write was blocked: %v", err)
	default:
	}
	once.Do(func() { close(release) })
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(writes) != 2 || writes[0].Appearance.AccentHue != 1 || writes[1].Appearance.AccentHue != 100 {
		t.Fatalf("writes were not coalesced in order: %+v", writes)
	}
	update(t, s, func(v *contract.Settings) { v.Appearance.AccentHue = 101 })
	flush(t, s)
	if len(writes) != 3 || writes[2].Appearance.AccentHue != 101 {
		t.Fatal("writer did not accept another update after Flush")
	}
}

func TestSubscribersAndSnapshotOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	d := &queued{}
	s := open(t, path, fsstore.New(), d, nil)
	var seen []string
	var retained *contract.Settings
	var inUpdate bool
	s.Subscribe(func(v contract.Settings) {
		if !inUpdate {
			t.Error("notification was not synchronous")
		}
		seen = append(seen, "first")
		v.Keymap[0].Chord.Key = "mutated subscriber copy"
	})
	s.Subscribe(func(v contract.Settings) {
		seen = append(seen, "second")
		if v.Keymap[0].Chord.Key != "F2" || s.Get().Keymap[0].Chord.Key != "F2" {
			t.Error("subscriber shares keymap memory")
		}
	})
	bindings := []contract.KeyBinding{{Command: contract.CmdFileSave, Chord: contract.Chord{Key: "F2"}}}
	inUpdate = true
	update(t, s, func(v *contract.Settings) { retained = v; v.Keymap = bindings })
	inUpdate = false
	if !reflect.DeepEqual(seen, []string{"first", "second"}) {
		t.Fatalf("notifications: %v", seen)
	}
	bindings[0].Chord.Key = "mutated input"
	retained.Appearance.Theme = "dark"
	retained.Keymap[0].Chord.Key = "mutated retained edit"
	s.Get().Keymap[0].Chord.Key = "mutated Get"
	if got := s.Get(); got.Appearance.Theme != "system" || got.Keymap[0].Chord.Key != "F2" {
		t.Fatalf("store aliases a caller: %+v", got)
	}
	flush(t, s)
	d.flush()
	if got := open(t, path, fsstore.New(), nil, nil).Get(); got.Keymap[0].Chord.Key != "F2" {
		t.Fatalf("disk snapshot aliased an edit: %+v", got)
	}
}

func TestReentrantSubscriptions(t *testing.T) {
	s := open(t, filepath.Join(t.TempDir(), "settings.json"), fsstore.New(), nil, nil)
	var seen []string
	var offSecond func()
	s.Subscribe(func(v contract.Settings) {
		seen = append(seen, "first:"+v.Appearance.Theme)
		if v.Appearance.Theme == "dark" {
			offSecond()
			offSecond()
			s.Subscribe(func(v contract.Settings) { seen = append(seen, "new:"+v.Appearance.Theme) })
			update(t, s, func(v *contract.Settings) { v.Appearance.Theme = "light" })
		}
	})
	offSecond = s.Subscribe(func(contract.Settings) { t.Error("removed subscriber called") })
	update(t, s, func(v *contract.Settings) { v.Appearance.Theme = "dark" })
	if !reflect.DeepEqual(seen, []string{"first:dark", "first:light", "new:light"}) {
		t.Fatalf("reentrant delivery: %v", seen)
	}
	flush(t, s)
	if got := open(t, s.path, fsstore.New(), nil, nil).Get().Appearance.Theme; got != "light" {
		t.Fatalf("outer update overwrote nested update: %q", got)
	}
}

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buffer bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buffer, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &buffer
}

func TestSubscriberPanicIsLoggedAndIsolated(t *testing.T) {
	logs := captureLogs(t)
	s := open(t, filepath.Join(t.TempDir(), "settings.json"), fsstore.New(), nil, nil)
	var seen []int
	s.Subscribe(func(contract.Settings) { seen = append(seen, 1); panic("subscriber failure") })
	s.Subscribe(func(contract.Settings) { seen = append(seen, 2) })
	update(t, s, func(v *contract.Settings) { v.Language = "zh-Hans" })
	flush(t, s)
	if !reflect.DeepEqual(seen, []int{1, 2}) || s.Get().Language != "zh-Hans" {
		t.Fatalf("panic stopped delivery: %v", seen)
	}
	for _, text := range []string{`"component":"settings"`, `"source":"subscriber"`, "subscriber failure", `"stack":`} {
		if !strings.Contains(logs.String(), text) {
			t.Errorf("missing %q in log: %s", text, logs.String())
		}
	}
}

func TestWriteFailuresArePostedAndStateSurvives(t *testing.T) {
	for _, panicWrite := range []bool{false, true} {
		t.Run(map[bool]string{false: "error", true: "panic"}[panicWrite], func(t *testing.T) {
			logs := captureLogs(t)
			path := filepath.Join(t.TempDir(), "settings.json")
			d := &queued{}
			sentinel := contract.E(contract.CodePermission, "fake.Write", errors.New("disk denied"))
			calls := 0
			f := &files{FileStore: fsstore.New(), write: func(string, string, contract.DocMeta, *contract.FileStamp) (contract.FileStamp, error) {
				calls++
				if calls == 1 {
					if panicWrite {
						panic("writer failure")
					}
					return contract.FileStamp{}, sentinel
				}
				return contract.FileStamp{}, nil
			}}
			var reported []error
			onMain := false
			s := open(t, path, f, d, func(err error) {
				if !onMain {
					t.Error("handler did not go through the dispatcher")
				}
				reported = append(reported, err)
			})
			update(t, s, func(v *contract.Settings) { v.Language = "en" })
			err := s.Flush()
			code := contract.CodePermission
			if panicWrite {
				code = contract.CodeInternal
			} else if !errors.Is(err, sentinel) {
				t.Fatalf("lost write cause: %v", err)
			}
			var ce *contract.Error
			if !errors.As(err, &ce) || ce.Code != code || ce.Path != path || ce.Op != "settings.Write" {
				t.Fatalf("write error: %#v", err)
			}
			if len(reported) != 0 || s.Get().Language != "en" {
				t.Fatal("handler ran before dispatch or memory was rolled back")
			}
			onMain = true
			d.flush()
			if len(reported) != 1 || reported[0] != err || !strings.Contains(logs.String(), "settings write failed") {
				t.Fatalf("write failure not reported/logged: %v, %s", reported, logs.String())
			}
			update(t, s, func(v *contract.Settings) { v.Language = "zh-Hans" })
			flush(t, s)
			if calls != 2 {
				t.Fatalf("writer did not recover: %d writes", calls)
			}
		})
	}
}

func TestInvalidConstructionAndLoadErrors(t *testing.T) {
	for _, tc := range []struct {
		path string
		f    contract.FileStore
		d    contract.Dispatcher
	}{{"", fsstore.New(), &queued{}}, {"settings.json", nil, &queued{}}, {"settings.json", fsstore.New(), nil}} {
		if s, err := New(tc.path, tc.f, tc.d, nil); s != nil || contract.CodeOf(err) != contract.CodeInvalid {
			t.Fatalf("invalid constructor: %v %v", s, err)
		}
	}
	t.Run("read error does not overwrite", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "settings.json")
		put(t, path, []byte("keep"))
		cause := contract.E(contract.CodePermission, "fake.Read", os.ErrPermission)
		f := &files{read: func(string, contract.ReadOptions) (contract.FileContent, error) {
			return contract.FileContent{}, cause
		}}
		if s, err := New(path, f, &queued{}, nil); s != nil || contract.CodeOf(err) != contract.CodePermission || !errors.Is(err, cause) {
			t.Fatalf("read error: %v %v", s, err)
		}
		if string(contents(t, path)) != "keep" {
			t.Fatal("read error modified the file")
		}
	})
	t.Run("backup failure is returned", func(t *testing.T) {
		f := &files{read: func(string, contract.ReadOptions) (contract.FileContent, error) {
			return contract.FileContent{Text: "{"}, nil
		}}
		if s, err := New(filepath.Join(t.TempDir(), "missing.json"), f, &queued{}, nil); s != nil || contract.CodeOf(err) != contract.CodeIO {
			t.Fatalf("backup error: %v %v", s, err)
		}
	})
}

func TestMkdirAndErrorHandlerFailures(t *testing.T) {
	logs := captureLogs(t)
	parent := filepath.Join(t.TempDir(), "file")
	put(t, parent, []byte("not a directory"))
	f := &files{read: func(string, contract.ReadOptions) (contract.FileContent, error) {
		return contract.FileContent{}, contract.E(contract.CodeNotFound, "fake.Read", nil)
	}}
	d := &queued{}
	s := open(t, filepath.Join(parent, "settings.json"), f, d, func(error) { panic("handler failure") })
	update(t, s, func(v *contract.Settings) { v.Language = "en" })
	if err := s.Flush(); contract.CodeOf(err) != contract.CodeIO {
		t.Fatalf("mkdir failure: %v", err)
	}
	d.flush()
	if !strings.Contains(logs.String(), "handler failure") || !strings.Contains(logs.String(), `"source":"error handler"`) {
		t.Fatalf("handler panic not isolated: %s", logs.String())
	}
}

func TestNonFiniteUpdatesUseDefaults(t *testing.T) {
	s := open(t, filepath.Join(t.TempDir(), "settings.json"), fsstore.New(), nil, nil)
	update(t, s, func(v *contract.Settings) {
		v.Appearance.FontSize = float32(math.NaN())
		v.Appearance.LineHeight = float32(math.Inf(1))
		v.Appearance.ContentWidth = float32(math.Inf(-1))
	})
	flush(t, s)
	if got := s.Get().Appearance; !reflect.DeepEqual(got, Defaults().Appearance) {
		t.Fatalf("nonfinite values: %+v", got)
	}
}
