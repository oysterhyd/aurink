package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"aurink/internal/contract"
)

type migration func(map[string]json.RawMessage) error

// Entry n upgrades version n to n+1 before decoding into the current schema.
var migrations = []migration{
	func(map[string]json.RawMessage) error { return nil },
}

func migrate(raw map[string]json.RawMessage, version, target int, steps []migration) error {
	for version < target {
		if version >= len(steps) || steps[version] == nil {
			return fmt.Errorf("missing migration from version %d", version)
		}
		if err := steps[version](raw); err != nil {
			return fmt.Errorf("migration from version %d: %w", version, err)
		}
		version++
	}
	for key := range raw {
		if strings.EqualFold(key, "Version") {
			delete(raw, key)
		}
	}
	raw["Version"] = json.RawMessage(strconv.Itoa(target))
	return nil
}

func (s *Store) load(steps []migration) error {
	content, err := s.files.Read(s.path, contract.ReadOptions{})
	if err != nil {
		switch contract.CodeOf(err) {
		case contract.CodeNotFound:
			return nil
		case contract.CodeEncoding, contract.CodeBinary:
			return s.backup()
		default:
			return failure(contract.CodeOf(err), "settings.Load", s.path, err)
		}
	}
	var raw map[string]json.RawMessage
	var header struct{ Version int }
	data := []byte(content.Text)
	if json.Unmarshal(data, &raw) != nil || raw == nil ||
		json.Unmarshal(data, &header) != nil || header.Version < 0 {
		return s.backup()
	}
	if header.Version > contract.SettingsVersion {
		// No startup write is queued, including when Flush is called.
		return nil
	}
	if err := migrate(raw, header.Version, contract.SettingsVersion, steps); err != nil {
		return failure(contract.CodeInvalid, "settings.Migrate", s.path, err)
	}
	data, err = json.Marshal(raw)
	if err != nil {
		return failure(contract.CodeInvalid, "settings.Migrate", s.path, err)
	}
	v := Defaults()
	if err := json.Unmarshal(data, &v); err != nil {
		return s.backup()
	}
	normalize(&v)
	s.value = v
	return nil
}

func (s *Store) backup() error {
	base := s.path + ".bad"
	for number := 0; ; number++ {
		path := base
		if number > 0 {
			path += "." + strconv.Itoa(number)
		}
		if _, err := os.Lstat(path); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return failure(contract.CodeIO, "settings.Backup", path, err)
		}
		if err := os.Rename(s.path, path); err != nil {
			return failure(contract.CodeIO, "settings.Backup", s.path, err)
		}
		return nil
	}
}
