package docsession

import (
	"fmt"
	"math/rand/v2"
	"path/filepath"
	"testing"
	"time"
)

func TestRandomEditsSavesUndoAutosaveAndRestarts(t *testing.T) {
	for _, seed := range []uint64{1, 17, 937} {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			h := newHarness(t)
			m := h.manager()
			path := filepath.Join(h.dir, "random.md")
			put(t, path, "base\r\n中")
			s := h.open(m, path)
			source, disk := "base\r\n中", "base\r\n中"
			var undo, redo []string
			forced := false
			var deadline time.Time
			resetDeadline := func() {
				deadline = time.Time{}
				if h.settings.Files.Autosave && !forced && source != disk {
					deadline = h.clock.now.Add(100 * time.Millisecond)
				}
			}
			advance := func(d time.Duration) {
				h.advance(d)
				if !deadline.IsZero() && !deadline.After(h.clock.now) {
					disk, forced, deadline = source, false, time.Time{}
				}
			}
			rng := rand.New(rand.NewPCG(seed, seed+1))
			restarts := 0
			for step := range 500 {
				switch rng.IntN(8) {
				case 0, 1:
					boundaries := []int{0}
					for i := range source {
						if i != 0 {
							boundaries = append(boundaries, i)
						}
					}
					if len(source) != 0 {
						boundaries = append(boundaries, len(source))
					}
					a, b := rng.IntN(len(boundaries)), rng.IntN(len(boundaries))
					if a > b {
						a, b = b, a
					}
					start, end := boundaries[a], boundaries[b]
					insert := []string{"x", "中", "\r\n", "", "# title\n"}[rng.IntN(5)]
					next := source[:start] + insert + source[end:]
					edit(t, s, start, end, insert)
					undo, redo, forced = append(undo, source), nil, false
					source = next
					resetDeadline()
				case 2:
					wantCode(t, h.finish(s.Save), "")
					disk, forced = source, false
					resetDeadline()
				case 3:
					changed := s.Model().Undo()
					if changed != (len(undo) != 0) {
						t.Fatalf("seed %d step %d undo availability", seed, step)
					}
					if changed {
						redo = append(redo, source)
						source, undo, forced = undo[len(undo)-1], undo[:len(undo)-1], false
						resetDeadline()
					}
				case 4:
					changed := s.Model().Redo()
					if changed != (len(redo) != 0) {
						t.Fatalf("seed %d step %d redo availability", seed, step)
					}
					if changed {
						undo = append(undo, source)
						source, redo, forced = redo[len(redo)-1], redo[:len(redo)-1], false
						resetDeadline()
					}
				case 5:
					advance(100 * time.Millisecond)
				case 6:
					h.settings.Files.Autosave = !h.settings.Files.Autosave
					m.SetSettings(h.settings)
					resetDeadline()
				case 7:
					advance(50 * time.Millisecond)
					wasEdited := forced || source != disk
					// Abandon the old process's timers without an explicit close.
					h.clock.timers = make(map[*fakeTimer]bool)
					m = h.manager()
					got := h.restore(m)
					if wasEdited {
						if len(got) != 1 || !got[0].State().Edited {
							t.Fatalf("seed %d step %d lost edited session", seed, step)
						}
						s, forced = got[0], true
					} else {
						if len(got) != 0 {
							t.Fatalf("seed %d step %d recovered a clean session", seed, step)
						}
						s, forced = h.open(m, path), false
					}
					undo, redo = nil, nil
					resetDeadline()
					restarts++
				}
				h.settle()
				if s.Model().Text().String() != source || contents(t, path) != disk || s.State().Edited != (forced || source != disk) {
					t.Fatalf("seed %d step %d: text %q want %q; disk %q want %q; state %+v", seed, step, s.Model().Text().String(), source, contents(t, path), disk, s.State())
				}
			}
			if restarts < 40 {
				t.Fatal("insufficient restarts", restarts)
			}
		})
	}
}
