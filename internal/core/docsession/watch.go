package docsession

import (
	"path/filepath"
	"slices"
	"strings"

	"aurink/internal/contract"
)

type directoryWatch struct {
	path  string
	refs  int
	watch contract.Watch
	ended bool
}

func directoryKey(path string) (string, string) {
	dir := filepath.Dir(path)
	_, key, _ := normalized(dir)
	return dir, key
}

func (m *Manager) addWatch(path string) {
	if m.options.Watcher == nil || path == "" {
		return
	}
	dir, key := directoryKey(path)
	if w := m.watches[key]; w != nil {
		w.refs++
		return
	}
	w := &directoryWatch{path: dir, refs: 1}
	m.watches[key] = w
	m.startWatch(key, w)
}

func (m *Manager) startWatch(key string, w *directoryWatch) {
	w.ended = false
	m.enqueue(func() {
		if m.watches[key] != w || w.refs == 0 {
			m.background(func() func() { return func() {} })
			return
		}
		m.background(func() func() {
			watch, err := m.options.Watcher.Watch(w.path, contract.WatchOptions{}, func(batch []contract.WatchEvent, err error) {
				batch = slices.Clone(batch)
				m.dispatcher.Post(func() {
					if m.watches[key] != w || w.refs == 0 {
						return
					}
					if err != nil {
						w.ended = true
						watch := w.watch
						w.watch = nil
						if watch != nil {
							m.closeWatch(watch)
						}
						m.checkDirectory(key, nil)
						return
					}
					for _, event := range batch {
						if event.Op == contract.WatchOverflow {
							m.Recheck()
							return
						}
					}
					m.checkDirectory(key, batch)
				})
			})
			return func() {
				if m.watches[key] != w || w.refs == 0 || w.ended {
					if watch != nil {
						m.closeWatch(watch)
					}
					return
				}
				w.watch = watch
				if err != nil {
					w.ended = true
					m.checkDirectory(key, nil)
				} else {
					// Cover changes between the initial file read and watch readiness.
					m.checkDirectory(key, nil)
				}
			}
		})
	})
}

func (m *Manager) removeWatch(path string) {
	if m.options.Watcher == nil || path == "" {
		return
	}
	_, key := directoryKey(path)
	w := m.watches[key]
	if w == nil {
		return
	}
	w.refs--
	if w.refs != 0 {
		return
	}
	m.enqueue(func() {
		if m.watches[key] != w || w.refs != 0 {
			m.background(func() func() { return func() {} })
			return
		}
		delete(m.watches, key)
		watch := w.watch
		m.background(func() func() {
			if watch != nil {
				watch.Close()
			}
			return func() {}
		})
	})
}

func (m *Manager) closeWatch(watch contract.Watch) {
	m.enqueue(func() {
		m.background(func() func() { watch.Close(); return func() {} })
	})
}

// setPath updates the path index and the directory watch together.
func (m *Manager) setPath(s *session, path string) {
	old := s.state.Path
	_, oldKey, _ := normalized(old)
	_, newKey, _ := normalized(path)
	if old != path {
		s.pathEpoch++
	}
	if oldKey != newKey {
		if old != "" {
			delete(m.paths, oldKey)
			m.removeWatch(old)
		}
		if path != "" {
			m.paths[newKey] = s
			m.addWatch(path)
		}
	}
	s.state.Path = path
	if path != "" {
		s.state.Name = filepath.Base(path)
	}
}

// Moved follows a file or a containing directory moved by the application.
func (m *Manager) Moved(oldPath, newPath string) {
	oldPath, _, err := normalized(oldPath)
	if err != nil {
		return
	}
	newPath, _, err = normalized(newPath)
	if err != nil {
		return
	}
	for _, s := range slices.Clone(m.sessions) {
		if s.closed || s.state.Path == "" {
			continue
		}
		rel, err := filepath.Rel(oldPath, s.state.Path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		target := filepath.Join(newPath, rel)
		_, key, _ := normalized(target)
		if other := m.paths[key]; other != nil && other != s {
			continue
		}
		m.setPath(s, target)
		s.state.Deleted = false
		s.schedule()
		s.notify()
	}
}

// Recheck compares every open file with its last observed disk stamp and
// retries watches that ended while their directories were unavailable.
func (m *Manager) Recheck() {
	for key, watch := range m.watches {
		if watch.refs > 0 && watch.ended {
			m.startWatch(key, watch)
		}
	}
	m.checkFiles("", nil, nil)
}

func (m *Manager) checkPath(s *session) { m.checkFiles("", nil, s) }

func (m *Manager) checkDirectory(key string, batch []contract.WatchEvent) {
	m.checkFiles(key, batch, nil)
}

type checkedFile struct {
	session  *session
	path     string
	epoch    uint64
	observed contract.FileStamp
	deleted  bool
	saving   bool
	stamp    contract.FileStamp
	checked  bool
	content  contract.FileContent
	err      error
	rename   string
}

func sameStamp(a, b contract.FileStamp) bool {
	return a.Size == b.Size && a.ModTime.Equal(b.ModTime)
}

// checkFiles queues the stat and any reads with saves, so a completed own write
// is compared against its returned stamp before a watch event is interpreted.
func (m *Manager) checkFiles(dir string, batch []contract.WatchEvent, only *session) {
	saving := make(map[*session]bool)
	for _, s := range m.sessions {
		if s.state.Saving {
			saving[s] = true
		}
	}
	m.enqueue(func() {
		var files []checkedFile
		for _, s := range m.sessions {
			if s.closed || s.state.Path == "" || (only != nil && s != only) {
				continue
			}
			_, key := directoryKey(s.state.Path)
			if dir != "" && key != dir {
				continue
			}
			files = append(files, checkedFile{session: s, path: s.state.Path, epoch: s.pathEpoch,
				observed: s.observed, deleted: s.state.Deleted, saving: saving[s]})
		}
		removed, candidates := make(map[string]bool), make(map[string]string)
		for _, event := range batch {
			if event.IsDir {
				continue
			}
			if event.Op == contract.WatchRemove || event.Op == contract.WatchRename {
				path := event.Path
				if event.Op == contract.WatchRename {
					path = event.OldPath
				}
				_, key, err := normalized(path)
				if err == nil {
					removed[key] = true
				}
			}
			if event.Op == contract.WatchCreate || event.Op == contract.WatchRename {
				path, key, err := normalized(event.Path)
				_, candidateDir := directoryKey(path)
				if err == nil && candidateDir == dir {
					candidates[key] = path
				}
			}
		}
		opts, threshold := m.readOptions(), m.largeThreshold()
		m.background(func() func() {
			candidateStamps := make(map[string]contract.FileStamp)
			for key, path := range candidates {
				if stamp, err := m.files.Stat(path); err == nil {
					candidateStamps[key] = stamp
				}
			}
			for i := range files {
				f := &files[i]
				f.stamp, f.err = m.files.Stat(f.path)
				f.checked = f.err == nil
				if contract.CodeOf(f.err) != contract.CodeNotFound {
					continue
				}
				_, oldKey, _ := normalized(f.path)
				if !removed[oldKey] {
					continue
				}
				for key, stamp := range candidateStamps {
					if sameStamp(stamp, f.observed) {
						if f.rename != "" {
							f.rename = ""
							break
						}
						f.rename = candidates[key]
					}
				}
			}
			// A candidate shared by two deleted files cannot identify either one.
			uses := make(map[string]int)
			for i := range files {
				if files[i].rename != "" {
					uses[files[i].rename]++
				}
			}
			for i := range files {
				if uses[files[i].rename] > 1 {
					files[i].rename = ""
				}
			}
			for i := range files {
				f := &files[i]
				if f.err == nil && (f.deleted || !sameStamp(f.stamp, f.observed)) {
					f.content, f.err = m.files.Read(f.path, opts)
					if f.err == nil && (f.content.Stamp.Size >= threshold || int64(len(f.content.Text)) >= threshold) {
						f.content.Meta.Large = true
					}
				}
			}
			return func() {
				for _, f := range files {
					m.applyCheck(f)
				}
			}
		})
	})
}

func (m *Manager) applyCheck(f checkedFile) {
	s := f.session
	if s.closed || s.pathEpoch != f.epoch {
		return
	}
	if contract.CodeOf(f.err) == contract.CodeNotFound {
		if f.rename != "" {
			_, key, _ := normalized(f.rename)
			if other := m.paths[key]; other == nil || other == s {
				m.setPath(s, f.rename)
				s.state.Deleted = false
				s.schedule()
				s.notify()
				return
			}
		}
		s.state.Deleted = true
		s.schedule()
		s.notify()
		return
	}
	if f.err != nil {
		if f.checked && (s.state.Edited || s.state.Conflict || f.saving) &&
			(f.deleted || !sameStamp(f.stamp, f.observed)) {
			s.state.Deleted = false
			s.observed = f.stamp
			if f.saving && !s.state.Edited {
				s.forcedEdit = true
				s.updateEdited()
			}
			s.state.Conflict = true
			s.schedule()
			s.writeCopy()
		}
		s.state.Err = failure(f.err)
		s.notify()
		return
	}
	if !f.deleted && sameStamp(f.stamp, f.observed) {
		return
	}
	s.state.Deleted = false
	s.observed = f.content.Stamp
	if s.state.Edited || s.state.Conflict || f.saving {
		if f.saving && !s.state.Edited {
			s.forcedEdit = true
			s.updateEdited()
		}
		s.state.Conflict = true
		s.schedule()
		if s.state.Edited {
			s.writeCopy()
		}
		s.notify()
		return
	}
	s.replaceExternal(f.content)
}

func (s *session) replaceExternal(content contract.FileContent) {
	// The notification can arrive after an autosave has completed even when the
	// external write raced that save. Keep the just-saved text across a crash.
	if s.localSaved && s.saved != content.Text {
		s.rescue = &recoveryCopy{Format: 1, ID: s.ID(), Path: s.state.Path, Name: s.state.Name,
			Text: []byte(s.saved), BaseText: []byte(content.Text), Meta: s.model.Meta(), Stamp: content.Stamp, Version: s.model.Version()}
	}
	s.localSaved = false
	s.loading = true
	userReadOnly := s.model.ReadOnly() && !s.legacyRO
	if s.model.ReadOnly() {
		s.model.SetReadOnly(false)
	}
	s.model.SetMeta(content.Meta)
	s.model.Replace(content.Text, contract.OriginReload)
	s.legacyRO = isLegacyEncoding(content.Meta.Encoding)
	if userReadOnly || s.legacyRO {
		s.model.SetReadOnly(true)
	}
	s.loading = false
	s.state.ReadOnly = s.model.ReadOnly()
	s.state.LargeFile = content.Meta.Large
	s.stamp, s.observed, s.saved, s.savedHash = content.Stamp, content.Stamp, content.Text, fingerprint(content.Text)
	s.previous = s.model.Text()
	s.hash = fingerprint(s.previous.String())
	s.state.Conflict, s.state.Deleted, s.forcedEdit, s.converted = false, false, false, false
	s.state.Err = nil
	s.updateEdited()
	s.schedule()
	s.notify()
}
