package docsession

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"aurink/internal/contract"
)

// One atomic envelope keeps text and metadata from different edits inseparable.
type recoveryCopy struct {
	Format   int
	ID       contract.DocID
	Path     string
	Name     string
	Text     []byte
	BaseText []byte
	Meta     contract.DocMeta
	Stamp    contract.FileStamp
	Version  contract.Version
}

type recovered struct {
	copy     recoveryCopy
	conflict bool
	deleted  bool
}

func (s *session) copyPath() string {
	if s.manager.recovery == "" {
		return ""
	}
	return filepath.Join(s.manager.recovery, string(s.ID())+".json")
}

func (s *session) writeCopy() {
	m := s.manager
	m.enqueue(func() {
		path := s.copyPath()
		var copy *recoveryCopy
		if path != "" && !s.closed && s.state.Edited {
			copy = &recoveryCopy{Format: 1, ID: s.ID(), Path: s.state.Path, Name: s.state.Name,
				Text: []byte(s.model.Text().String()), BaseText: []byte(s.saved), Meta: s.model.Meta(), Stamp: s.stamp, Version: s.model.Version()}
		}
		m.background(func() func() {
			var err error
			if copy != nil {
				err = os.MkdirAll(filepath.Dir(path), 0o700)
				if err == nil {
					data, marshalErr := json.Marshal(copy)
					err = marshalErr
					if err == nil {
						_, err = m.files.Write(path, string(data), contract.DocMeta{Encoding: "utf-8"}, nil)
					}
				}
			}
			return func() {
				if err != nil {
					slog.Error("recovery write failed", "path", path, "error", err)
					if !s.closed {
						s.state.Err = failure(err)
						s.notify()
					}
				}
			}
		})
	})
}

func removeRecovery(path string) {
	if path != "" {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			slog.Error("recovery removal failed", "path", path, "error", err)
		}
	}
}

func (s *session) removeCopy() {
	path := s.copyPath()
	if path == "" {
		return
	}
	s.manager.enqueue(func() {
		s.manager.background(func() func() {
			removeRecovery(path)
			return func() {}
		})
	})
}

// Restore loads valid recovery envelopes without writing their source files.
// Corrupt copies are logged and retained. Repeated calls reuse open sessions.
func (m *Manager) Restore(done func([]contract.Session, error)) {
	m.enqueue(func() {
		m.background(func() func() {
			copies, err := m.readCopies()
			return func() {
				var sessions []contract.Session
				for _, r := range copies {
					c := r.copy
					if existing, ok := m.Get(c.ID); ok {
						sessions = append(sessions, existing)
						continue
					}
					if existing, ok := m.FindByPath(c.Path); c.Path != "" && ok {
						sessions = append(sessions, existing)
						continue
					}
					s, createErr := m.create(c.ID, c.Path, c.Name, string(c.Text), c.Meta, c.Stamp)
					if createErr != nil {
						err = createErr
						continue
					}
					s.saved, s.savedHash = string(c.BaseText), fingerprint(string(c.BaseText))
					s.forcedEdit, s.state.Edited = true, true
					s.state.Conflict, s.state.Deleted = r.conflict, r.deleted
					s.schedule()
					sessions = append(sessions, s)
					m.events.emit(contract.SessionsEvent{Kind: contract.SessionRestored, ID: s.ID()})
				}
				if done != nil {
					defer logPanic()
					done(sessions, failureOrNil(err))
				}
			}
		})
	})
}

func (m *Manager) readCopies() ([]recovered, error) {
	if m.recovery == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(m.recovery)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, failure(err)
	}
	var copies []recovered
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(m.recovery, entry.Name())
		// JSON escaping and the saved baseline can exceed the document size.
		content, err := m.files.Read(path, contract.ReadOptions{MaxSize: 128 << 20})
		if contract.CodeOf(err) == contract.CodeNotFound {
			continue
		}
		var c recoveryCopy
		if err == nil {
			err = json.Unmarshal([]byte(content.Text), &c)
		}
		valid := err == nil && c.Format == 1 && c.ID != "" && c.Name != "" && c.Version != 0 &&
			string(c.ID)+".json" == entry.Name() && c.Text != nil && c.BaseText != nil &&
			(c.Meta.Encoding == "utf-8" || c.Meta.Encoding == "utf-16le" || c.Meta.Encoding == "utf-16be")
		if c.Path != "" {
			abs, _, pathErr := normalized(c.Path)
			valid = valid && pathErr == nil && abs == c.Path
		}
		if !valid {
			slog.Error("corrupt recovery copy skipped", "path", path, "error", err)
			continue
		}
		r := recovered{copy: c}
		if c.Path != "" {
			stamp, statErr := m.files.Stat(c.Path)
			r.deleted = contract.CodeOf(statErr) == contract.CodeNotFound
			r.conflict = statErr != nil || stamp.Size != c.Stamp.Size || !stamp.ModTime.Equal(c.Stamp.ModTime)
			if statErr != nil && !r.deleted {
				slog.Error("recovery source check failed", "path", c.Path, "error", statErr)
			}
		}
		copies = append(copies, r)
	}
	return copies, nil
}
