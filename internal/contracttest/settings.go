package contracttest

import (
	"testing"

	"aurink/internal/contract"
)

// SettingsFixture opens a store on one settings file.
type SettingsFixture struct {
	// Open returns a new store on the file.
	Open func() contract.SettingsStore
	// Flush waits until the writes of s have reached the disk.
	Flush func(s contract.SettingsStore)
}

// TestSettings checks the SettingsStore contract. newFixture returns a store
// location that has no settings yet.
func TestSettings(t *testing.T, newFixture func(t *testing.T) SettingsFixture) {
	t.Run("defaults", func(t *testing.T) {
		got := newFixture(t).Open().Get()
		a, e, f := got.Appearance, got.Editor, got.Files
		if got.Version != contract.SettingsVersion || a.Theme != "system" || a.AccentHue != 182 ||
			a.FontSize != 16 || a.LineHeight != 1.75 || a.ContentWidth != 720 || a.ReduceMotion != "system" {
			t.Errorf("appearance %+v, version %d", a, got.Version)
		}
		if e.Exts != contract.ExtDefault || e.BulletMarker != "-" || e.StrongDelim != "**" || e.EmphasisDelim != "*" || e.TabSize <= 0 {
			t.Errorf("editor %+v", e)
		}
		if !f.Autosave || f.AutosaveDelayMs != 1500 || f.RemoteImages || f.LargeFileBytes != 5<<20 {
			t.Errorf("files %+v", f)
		}
	})
	t.Run("update persists and notifies", func(t *testing.T) {
		f := newFixture(t)
		s := f.Open()
		var seen []string
		off := s.Subscribe(func(v contract.Settings) { seen = append(seen, v.Appearance.Theme) })
		if err := s.Update(func(v *contract.Settings) { v.Appearance.Theme = "dark" }); err != nil {
			t.Fatal(err)
		}
		off()
		if err := s.Update(func(v *contract.Settings) { v.Appearance.AccentHue = 48 }); err != nil {
			t.Fatal(err)
		}
		if len(seen) != 1 || seen[0] != "dark" {
			t.Errorf("subscriber saw %v", seen)
		}
		f.Flush(s)
		got := f.Open().Get()
		if got.Appearance.Theme != "dark" || got.Appearance.AccentHue != 48 {
			t.Errorf("reopened %+v", got.Appearance)
		}
	})
	t.Run("Get returns a copy", func(t *testing.T) {
		s := newFixture(t).Open()
		binding := contract.KeyBinding{Command: contract.CmdFileSave, Chord: contract.Chord{Key: "F2"}}
		if err := s.Update(func(v *contract.Settings) { v.Keymap = []contract.KeyBinding{binding} }); err != nil {
			t.Fatal(err)
		}
		s.Get().Keymap[0].Chord.Key = "F3"
		if got := s.Get().Keymap[0].Chord.Key; got != "F2" {
			t.Errorf("the store's keymap became %q", got)
		}
	})
}
