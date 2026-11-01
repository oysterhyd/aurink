package contracttest

import (
	"errors"
	"runtime"
	"testing"

	"aurink/internal/contract"
)

// TestCommands checks docs/04 §4.6 for a registry.
func TestCommands(t *testing.T, newRegistry func(t *testing.T) contract.CommandRegistry) {
	save := contract.Chord{Mods: contract.ModPrimary, Key: "S"}
	open := contract.Chord{Mods: contract.ModPrimary, Key: "O"}
	f2 := contract.Chord{Key: "F2"}
	cmd := func(id contract.CommandID) contract.Command {
		return contract.Command{ID: id, Title: "t", Category: "c"}
	}

	t.Run("register and run", func(t *testing.T) {
		r := newRegistry(t)
		runs := 0
		c := cmd(contract.CmdFileSave)
		c.Run = func() error { runs++; return nil }
		if err := r.Register(c, cmd(contract.CmdFileOpen)); err != nil {
			t.Fatal(err)
		}
		wantCode(t, r.Register(cmd(contract.CmdFileSave)), contract.CodeExists)
		if got, ok := r.Lookup(contract.CmdFileSave); !ok || got.ID != contract.CmdFileSave {
			t.Errorf("Lookup %+v, %v", got, ok)
		}
		if len(r.All()) != 2 {
			t.Errorf("All has %d commands", len(r.All()))
		}
		if err := r.Run(contract.CmdFileSave); err != nil || runs != 1 {
			t.Errorf("Run: %v, %d runs", err, runs)
		}
		wantCode(t, r.Run(contract.CmdFileClose), contract.CodeNotFound)
	})
	t.Run("disabled commands do not run", func(t *testing.T) {
		r := newRegistry(t)
		c := cmd(contract.CmdFileSave)
		c.Enabled = func() bool { return false }
		c.Run = func() error { t.Error("ran"); return nil }
		if err := r.Register(c); err != nil {
			t.Fatal(err)
		}
		wantCode(t, r.Run(contract.CmdFileSave), contract.CodeInvalid)
	})
	t.Run("errors of a command reach the caller", func(t *testing.T) {
		r := newRegistry(t)
		want := errors.New("failed")
		c := cmd(contract.CmdFileSave)
		c.Run = func() error { return want }
		if err := r.Register(c); err != nil {
			t.Fatal(err)
		}
		if err := r.Run(contract.CmdFileSave); !errors.Is(err, want) {
			t.Errorf("Run returned %v", err)
		}
	})
	t.Run("bindings filter by platform", func(t *testing.T) {
		r := newRegistry(t)
		other := "plan9"
		if runtime.GOOS == other {
			other = "linux"
		}
		r.SetDefaults([]contract.KeyBinding{
			{Command: contract.CmdFileSave, Chord: save},
			{Command: contract.CmdFileOpen, Chord: open, Platform: runtime.GOOS},
			{Command: contract.CmdFileClose, Chord: f2, Platform: other},
		})
		if id, ok := r.Resolve(save); !ok || id != contract.CmdFileSave {
			t.Errorf("Resolve(save) = %q, %v", id, ok)
		}
		if id, ok := r.Resolve(open); !ok || id != contract.CmdFileOpen {
			t.Errorf("Resolve(open) = %q, %v", id, ok)
		}
		if id, ok := r.Resolve(f2); ok {
			t.Errorf("Resolve(f2) = %q on another platform", id)
		}
		if keys := r.KeysFor(contract.CmdFileSave); len(keys) != 1 || keys[0] != save {
			t.Errorf("KeysFor %v", keys)
		}
	})
	// A user binding replaces the defaults of its command.
	t.Run("user bindings override", func(t *testing.T) {
		r := newRegistry(t)
		r.SetDefaults([]contract.KeyBinding{{Command: contract.CmdFileSave, Chord: save}})
		if got := r.SetUserBindings([]contract.KeyBinding{{Command: contract.CmdFileSave, Chord: f2}}); len(got) != 0 {
			t.Errorf("conflicts %v", got)
		}
		if id, ok := r.Resolve(f2); !ok || id != contract.CmdFileSave {
			t.Errorf("Resolve(f2) = %q, %v", id, ok)
		}
		if id, ok := r.Resolve(save); ok {
			t.Errorf("the replaced default still resolves to %q", id)
		}
		r.SetUserBindings(nil)
		if id, ok := r.Resolve(save); !ok || id != contract.CmdFileSave {
			t.Errorf("Resolve(save) = %q, %v after clearing", id, ok)
		}
	})
	t.Run("conflicts are reported", func(t *testing.T) {
		r := newRegistry(t)
		r.SetDefaults([]contract.KeyBinding{{Command: contract.CmdFileSave, Chord: save}})
		got := r.SetUserBindings([]contract.KeyBinding{{Command: contract.CmdFileOpen, Chord: save}})
		if len(got) != 1 || got[0].Chord != save || len(got[0].Commands) != 2 {
			t.Errorf("conflicts %+v", got)
		}
	})
}
