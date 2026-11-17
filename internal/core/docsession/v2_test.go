package docsession

import (
	"path/filepath"
	"testing"
	"time"

	"aurink/internal/contract"
	"aurink/internal/contracttest"
	"aurink/internal/core/fsstore"
)

func TestSessionsV2(t *testing.T) {
	contracttest.TestSessionsV2(t, func(t *testing.T) contracttest.SessionV2Fixture {
		h := newHarness(t)
		watcher := &contracttest.SnapshotWatcher{}
		h.watcher = watcher
		return contracttest.SessionV2Fixture{
			SessionFixture: contracttest.SessionFixture{New: func() contract.Sessions { return h.manager() },
				Settle: h.settle, Advance: h.clock.advance, Autosave: 100 * time.Millisecond, Dir: h.dir},
			Watcher: watcher,
		}
	})
}

func TestSessionStore(t *testing.T) {
	contracttest.TestSessionStore(t, func(t *testing.T) contracttest.SessionStoreFixture {
		path := filepath.Join(t.TempDir(), "userdata", "session.json")
		return contracttest.SessionStoreFixture{File: path, New: func() contract.SessionStore {
			return NewSessionStore(path, fsstore.New())
		}}
	})
}

func TestSessionsPhase4(t *testing.T) {
	t.Skip("BE-1.8 / BE-2.11")
	contracttest.TestSessionsPhase4(t, func(t *testing.T) contracttest.SessionPhase4Fixture {
		h := newHarness(t)
		h.settings.Files.LargeFileBytes = 1024
		return contracttest.SessionPhase4Fixture{
			SessionFixture: contracttest.SessionFixture{New: func() contract.Sessions { return h.manager() },
				Settle: h.settle, Advance: h.clock.advance, Autosave: 100 * time.Millisecond, Dir: h.dir},
			LargeFileBytes: 1024,
		}
	})
}
