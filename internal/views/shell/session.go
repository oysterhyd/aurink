package shell

import (
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"aurink/internal/contract"
	"aurink/locales"
)

// session keeps the shell's state on disk for the next start (D9, INT-2.1).
type session struct {
	// loaded is false until the last session is restored, so that the empty
	// shell of a start does not overwrite it.
	loaded bool
	// frozen stops saving while the window closes: closing the tabs one by one
	// must not record fewer tabs.
	frozen bool
	busy   bool
	again  bool
	// waiting hear about the save after the one running.
	waiting []func(error)
	last    *contract.SessionState
	// expanded is how many folders the tree had open in the last frame; a
	// change saves.
	expanded int
}

// SessionState is what a restart brings back: the folder and its open
// folders, the tabs with a file in their order, the active one, and each
// one's selection and scroll (D9, D10).
func (s *Shell) SessionState() contract.SessionState {
	st := contract.SessionState{Version: contract.SessionStateVersion, Active: -1}
	if ws := s.o.Workspace; ws != nil {
		st.Root = ws.Root()
	}
	if st.Root != "" {
		for p := range s.files.tree.Open.All() {
			st.Expanded = append(st.Expanded, p)
		}
		slices.Sort(st.Expanded)
	}
	for _, d := range s.docs {
		p := d.s.State().Path
		if p == "" {
			continue
		}
		if d == s.active {
			st.Active = len(st.Tabs)
		}
		sel := d.s.Model().Selection().Primary()
		v := contract.ViewState{Anchor: sel.Anchor, Head: sel.Head}
		v.Block, v.BlockOffset = d.live.ViewAnchor()
		st.Tabs = append(st.Tabs, contract.TabState{Path: p, View: v})
	}
	return st
}

// sessionChanged saves the session after a change of tabs, folder or tree.
// The caret and scroll go with it, and with SaveSession when the window closes.
func (s *Shell) sessionChanged() {
	if s.o.SessionStore == nil || !s.sess.loaded || s.sess.frozen {
		return
	}
	s.saveSession(nil)
}

// SaveSession saves the session now and, when freeze is true, no more after
// it until ThawSession: the window is closing and its tabs are about to go.
// done runs on the main thread once no save is running. Before the session is
// restored, or while frozen, it only waits for the save running.
func (s *Shell) SaveSession(freeze bool, done func(error)) {
	switch {
	case s.o.SessionStore == nil:
		done(nil)
	case !s.sess.loaded || s.sess.frozen:
		if s.sess.busy {
			s.sess.waiting = append(s.sess.waiting, done)
		} else {
			done(nil)
		}
	default:
		s.saveSession(done)
		s.sess.frozen = freeze
	}
}

// ThawSession saves again after a close was canceled.
func (s *Shell) ThawSession() {
	s.sess.frozen = false
	s.sessionChanged()
}

func (s *Shell) saveSession(done func(error)) {
	if s.sess.busy {
		s.sess.again = true
		if done != nil {
			s.sess.waiting = append(s.sess.waiting, done)
		}
		return
	}
	st := s.SessionState()
	if s.sess.last != nil && reflect.DeepEqual(*s.sess.last, st) {
		if done != nil {
			done(nil)
		}
		return
	}
	s.sess.busy = true
	store, post := s.o.SessionStore, s.o.Dispatcher.Post
	go func() {
		err := store.Save(st)
		post(func() {
			s.sess.busy = false
			if err == nil {
				s.sess.last = &st
			}
			if done != nil {
				done(err)
			}
			waiting := s.sess.waiting
			s.sess.waiting = nil
			if s.sess.again && s.sess.loaded {
				s.sess.again = false
				s.saveSession(func(err error) {
					for _, f := range waiting {
						f(err)
					}
				})
				return
			}
			s.sess.again = false
			for _, f := range waiting {
				f(err)
			}
		})
	}()
}

// RestoreSession opens the last session: the folder and its open folders, the
// tabs in their order with their selection and scroll, and the active one.
// Files gone since are left out and counted in one notice; tabs already open
// (recovered copies) stay, after the restored ones. done runs on the main
// thread once all is open. Settings.Files.RestoreSession off starts empty.
func (s *Shell) RestoreSession(done func()) {
	store := s.o.SessionStore
	if store == nil || !s.o.Settings.Get().Files.RestoreSession {
		s.sess.loaded = true
		done()
		return
	}
	post := s.o.Dispatcher.Post
	go func() {
		st, err := store.Load()
		post(func() {
			if err != nil {
				s.Notify(contract.Notice{Level: contract.NoticeWarning, Message: locales.ShellSessionUnreadable()})
			}
			s.applySession(st, done)
		})
	}()
}

func (s *Shell) applySession(st contract.SessionState, done func()) {
	if st.Root != "" && s.o.Workspace != nil {
		root, expanded := st.Root, st.Expanded
		s.o.Workspace.SetRoot(root, func(err error) {
			if err != nil {
				return // The folder is gone or out of reach: start without it.
			}
			s.sidebar, s.panel = true, panelFiles
			for _, p := range expanded {
				if rel, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(rel, "..") {
					s.ExpandFolder(p)
				}
			}
			s.sess.expanded = s.files.tree.Open.Len()
			s.sessionChanged()
		})
	}
	opened := make([]*doc, len(st.Tabs))
	missing := 0
	var next func(i int)
	next = func(i int) {
		if i < len(st.Tabs) {
			s.o.Sessions.Open(st.Tabs[i].Path, func(x contract.Session, err error) {
				switch {
				case err == nil:
					opened[i] = s.attach(x)
				case contract.CodeOf(err) == contract.CodeNotFound:
					missing++
				default:
					s.fail(err)
				}
				next(i + 1)
			})
			return
		}
		// The restored tabs first, in their order; the others after them.
		var docs []*doc
		for _, d := range opened {
			if d != nil {
				docs = append(docs, d)
			}
		}
		for _, d := range s.docs {
			if !slices.Contains(docs, d) {
				docs = append(docs, d)
			}
		}
		s.docs = docs
		for i, d := range opened {
			if d == nil {
				continue
			}
			v := st.Tabs[i].View
			n := d.s.Model().Text().Len()
			d.s.Model().SetSelection(contract.Selection{Ranges: []contract.SelRange{
				{Anchor: max(0, min(v.Anchor, n)), Head: max(0, min(v.Head, n))}}})
			d.live.SetViewAnchor(v.Block, v.BlockOffset)
		}
		if st.Active >= 0 && st.Active < len(opened) && opened[st.Active] != nil {
			s.activate(opened[st.Active])
		} else if len(s.docs) > 0 {
			s.activate(s.docs[0])
		}
		if missing > 0 {
			s.Notify(contract.Notice{Level: contract.NoticeWarning, Message: locales.ShellSessionMissing(missing)})
		}
		s.sess.loaded = true
		s.sessionChanged()
		done()
	}
	next(0)
}
