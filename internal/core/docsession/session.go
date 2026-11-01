package docsession

import (
	"path/filepath"
	"slices"

	"aurink/internal/contract"
)

type session struct {
	manager    *Manager
	model      contract.DocumentModel
	state      contract.DocState
	stamp      contract.FileStamp
	saved      string
	savedHash  uint64
	previous   contract.Text
	hash       uint64
	off        func()
	events     listeners[contract.DocState]
	closed     bool
	loading    bool
	forcedEdit bool
	saves      int
	auto       Timer
	copyTimer  Timer
	epoch      uint64
}

var _ contract.Session = (*session)(nil)

func (s *session) ID() contract.DocID                        { return s.state.ID }
func (s *session) Model() contract.DocumentModel             { return s.model }
func (s *session) State() contract.DocState                  { return s.state }
func (s *session) OnState(fn func(contract.DocState)) func() { return s.events.subscribe(fn) }

// The additive fingerprint can be updated from changed bytes alone. Byte
// permutations can collide, so it only rejects equality; a candidate match is
// always checked exactly. Ordinary typing never scans the unchanged document.
func fingerprint(text string) uint64 {
	var hash uint64
	for i := range len(text) {
		x := uint64(text[i]) + 0x9e3779b97f4a7c15
		x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
		x = (x ^ (x >> 27)) * 0x94d049bb133111eb
		hash += x ^ (x >> 31)
	}
	return hash
}

func (s *session) changed(event contract.DocEvent) {
	if s.closed || s.loading {
		return
	}
	s.state.ReadOnly = s.model.ReadOnly()
	next := s.model.Text()
	if next.Version() != s.previous.Version() && (event.Kind == contract.DocTextChanged || event.Kind == contract.DocReplaced) {
		if next.Version() == event.Version {
			for _, c := range event.Changes {
				s.hash -= fingerprint(s.previous.Slice(contract.Range{Start: c.OldStart, End: c.OldEnd}))
				s.hash += fingerprint(next.Slice(contract.Range{Start: c.NewStart, End: c.NewEnd}))
			}
		} else {
			// A subscriber can commit several edits while document events queue.
			s.hash = fingerprint(next.String())
		}
		s.previous = next
		s.forcedEdit = false
		s.updateEdited()
		s.schedule()
	} else if event.Kind == contract.DocReadOnlyChanged {
		s.schedule()
	}
	if event.Kind != contract.DocSelectionChanged {
		s.notify()
	}
}

func (s *session) updateEdited() {
	t := s.model.Text()
	s.state.Edited = s.forcedEdit || t.Len() != len(s.saved) || s.hash != s.savedHash || t.String() != s.saved
}

func (s *session) notify() {
	if !s.closed {
		s.events.emit(s.state)
	}
}

func (s *session) stopTimers() {
	s.epoch++
	if s.auto != nil {
		s.auto.Stop()
		s.auto = nil
	}
	if s.copyTimer != nil {
		s.copyTimer.Stop()
		s.copyTimer = nil
	}
}

// canAutosave leaves a restored copy alone until its first edit: the user has not
// seen it yet, and restoring must not write the document.
func (s *session) canAutosave() bool {
	return !s.closed && s.state.Edited && !s.forcedEdit && s.state.Path != "" && !s.state.ReadOnly && !s.state.Conflict && !s.state.Deleted && s.manager.settings.Files.Autosave
}

func (s *session) schedule() {
	s.stopTimers()
	if s.closed {
		return
	}
	if !s.state.Edited {
		s.removeCopy()
		return
	}
	m := s.manager
	epoch := s.epoch
	after := func(fn func()) func() {
		return func() {
			m.dispatcher.Post(func() {
				if !s.closed && s.epoch == epoch {
					fn()
				}
			})
		}
	}
	delay := m.options.RecoveryDelay
	if delay <= 0 {
		delay = m.delay() / 2
	}
	s.copyTimer = m.options.AfterFunc(delay, after(s.writeCopy))
	if s.canAutosave() {
		s.auto = m.options.AfterFunc(m.delay(), after(func() {
			if s.canAutosave() {
				s.Save(nil)
			}
		}))
	}
}

func (s *session) Save(done func(error)) { s.save("", false, done) }
func (s *session) SaveAs(path string, done func(error)) {
	if path == "" {
		s.manager.dispatcher.Post(func() { callDone(done, contract.E(contract.CodeInvalid, "docsession.SaveAs", nil)) })
		return
	}
	s.save(path, false, done)
}
func (s *session) KeepMine(done func(error)) { s.save("", true, done) }

func (s *session) save(path string, force bool, done func(error)) {
	m := s.manager
	// Capture the immutable text and metadata before starting any worker.
	text, meta, hash := s.model.Text(), s.model.Meta(), s.hash
	s.saves++
	s.state.Saving = true
	m.enqueue(func() {
		target := path
		if target == "" {
			target = s.state.Path
		}
		target, key, err := normalized(target)
		_, oldKey, _ := normalized(s.state.Path)
		sameTarget := oldKey != "" && oldKey == key
		if s.closed {
			err = contract.E(contract.CodeCanceled, "docsession.Save", nil)
		} else if s.model.ReadOnly() && path == "" {
			err = contract.E(contract.CodeReadOnly, "docsession.Save", nil)
		} else if other := m.paths[key]; other != nil && other != s {
			err = contract.E(contract.CodeExists, "docsession.SaveAs", nil)
		} else if s.state.Conflict && !force && (path == "" || sameTarget) {
			err = contract.E(contract.CodeConflict, "docsession.Save", nil)
		}
		var expect *contract.FileStamp
		if sameTarget && !force {
			stamp := s.stamp
			expect = &stamp
		}
		copyPath := s.copyPath()
		m.background(func() func() {
			var stamp contract.FileStamp
			deleted := false
			if err == nil && expect == nil && !force {
				targetStamp, statErr := m.files.Stat(target)
				if statErr == nil {
					expect = &targetStamp
				} else if contract.CodeOf(statErr) != contract.CodeNotFound {
					err = statErr
				}
			}
			if err == nil {
				stamp, err = m.files.Write(target, text.String(), meta, expect)
				if contract.CodeOf(err) == contract.CodeConflict {
					_, statErr := m.files.Stat(target)
					deleted = contract.CodeOf(statErr) == contract.CodeNotFound
				}
			}
			if err == nil {
				removeRecovery(copyPath)
			}
			return func() {
				s.saves--
				s.state.Saving = s.saves != 0
				if s.closed {
					callDone(done, contract.E(contract.CodeCanceled, "docsession.Save", nil))
					return
				}
				if err == nil {
					if s.state.Path != "" {
						_, old, _ := normalized(s.state.Path)
						delete(m.paths, old)
					}
					m.paths[key] = s
					s.state.Path, s.state.Name = target, filepath.Base(target)
					s.stamp, s.saved, s.savedHash = stamp, text.String(), hash
					s.state.SavedAt = m.options.Clock()
					s.state.Conflict, s.state.Deleted, s.forcedEdit = false, false, false
					s.updateEdited()
				} else {
					s.state.Conflict = s.state.Conflict || contract.CodeOf(err) == contract.CodeConflict
					s.state.Deleted = s.state.Deleted || deleted
				}
				s.state.Err = failure(err)
				s.schedule()
				if s.state.Edited {
					s.writeCopy()
				}
				s.notify()
				callDone(done, failureOrNil(err))
			}
		})
	})
	s.notify()
}

func failureOrNil(err error) error {
	if err == nil {
		return nil
	}
	return failure(err)
}

func (s *session) Revert(done func(error))     { s.reload(done) }
func (s *session) LoadTheirs(done func(error)) { s.reload(done) }

func (s *session) reload(done func(error)) {
	m := s.manager
	m.enqueue(func() {
		path, opts := s.state.Path, m.readOptions()
		m.background(func() func() {
			var content contract.FileContent
			var err error
			if path == "" {
				err = contract.E(contract.CodeInvalid, "docsession.Revert", nil)
			} else {
				content, err = m.files.Read(path, opts)
			}
			return func() {
				if s.closed {
					callDone(done, contract.E(contract.CodeCanceled, "docsession.Revert", nil))
					return
				}
				if err == nil && s.model.ReadOnly() {
					err = contract.E(contract.CodeReadOnly, "docsession.Revert", nil)
				}
				if err == nil {
					s.loading = true
					s.model.SetMeta(content.Meta)
					s.model.Replace(content.Text, contract.OriginReload)
					s.loading = false
					s.stamp, s.saved, s.savedHash = content.Stamp, content.Text, fingerprint(content.Text)
					s.previous = s.model.Text()
					s.hash = fingerprint(s.previous.String())
					s.state.Conflict, s.state.Deleted, s.forcedEdit = false, false, false
					s.updateEdited()
					s.schedule()
				} else if contract.CodeOf(err) == contract.CodeNotFound {
					s.state.Deleted = true
					s.schedule()
				}
				s.state.Err = failure(err)
				s.notify()
				callDone(done, failureOrNil(err))
			}
		})
	})
}

func (s *session) Close(force bool) contract.CloseDecision {
	if s.closed {
		return contract.CloseDone
	}
	if s.state.Edited && !force {
		return contract.CloseNeedsPrompt
	}
	s.closed = true
	s.stopTimers()
	s.off()
	m := s.manager
	if s.state.Path != "" {
		_, key, _ := normalized(s.state.Path)
		delete(m.paths, key)
	}
	i := slices.Index(m.sessions, s)
	m.sessions = slices.Delete(m.sessions, i, i+1)
	s.removeCopy()
	m.events.emit(contract.SessionsEvent{Kind: contract.SessionClosed, ID: s.ID()})
	return contract.CloseDone
}
