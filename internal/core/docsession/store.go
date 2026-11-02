package docsession

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"

	"aurink/internal/contract"
)

// SessionStore persists the shell's session state. Its methods block.
type SessionStore struct {
	path  string
	files contract.FileStore
}

var _ contract.SessionStore = (*SessionStore)(nil)

// NewSessionStore uses path as the session.json file and files for atomic writes.
func NewSessionStore(path string, files contract.FileStore) *SessionStore {
	return &SessionStore{path: path, files: files}
}

// Load returns an empty state for a missing file and backs up invalid data.
func (s *SessionStore) Load() (contract.SessionState, error) {
	empty := contract.SessionState{Version: contract.SessionStateVersion, Active: -1}
	if s.path == "" || s.files == nil {
		return empty, contract.E(contract.CodeInvalid, "docsession.SessionStore.Load", nil)
	}
	content, err := s.files.Read(s.path, contract.ReadOptions{})
	if contract.CodeOf(err) == contract.CodeNotFound {
		return empty, nil
	}
	if err != nil && contract.CodeOf(err) != contract.CodeEncoding && contract.CodeOf(err) != contract.CodeBinary {
		return empty, failure(err)
	}
	var state contract.SessionState
	var object map[string]json.RawMessage
	data := []byte(content.Text)
	if err == nil {
		err = json.Unmarshal(data, &object)
		if err == nil && object == nil {
			err = errors.New("session state is not an object")
		}
		if err == nil {
			err = json.Unmarshal(data, &state)
		}
	}
	if err != nil || state.Version < 0 || state.Version > contract.SessionStateVersion {
		if backupErr := s.backup(); backupErr != nil {
			return empty, backupErr
		}
		return empty, contract.E(contract.CodeInvalid, "docsession.SessionStore.Load", err)
	}
	return state, nil
}

func (s *SessionStore) backup() error {
	base := s.path + ".bad"
	for i := 0; ; i++ {
		path := base
		if i > 0 {
			path += "." + strconv.Itoa(i)
		}
		if _, err := os.Lstat(path); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return failure(err)
		}
		if err := os.Rename(s.path, path); err != nil {
			return failure(err)
		}
		return nil
	}
}

// Save writes the complete state through the atomic file store.
func (s *SessionStore) Save(state contract.SessionState) error {
	if s.path == "" || s.files == nil {
		return contract.E(contract.CodeInvalid, "docsession.SessionStore.Save", nil)
	}
	data, err := json.Marshal(state)
	if err != nil {
		return contract.E(contract.CodeInvalid, "docsession.SessionStore.Save", err)
	}
	if err = os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return failure(err)
	}
	_, err = s.files.Write(s.path, string(data), contract.DocMeta{Encoding: "utf-8"}, nil)
	return failureOrNil(err)
}
