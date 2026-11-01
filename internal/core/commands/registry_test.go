package commands

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"runtime"
	"strconv"
	"testing"

	"aurink/internal/contract"
	"aurink/internal/contracttest"
)

func TestCommands(t *testing.T) {
	contracttest.TestCommands(t, func(t *testing.T) contract.CommandRegistry { return New() })
}

func TestRegister(t *testing.T) {
	r := New()
	ids := sourceCommandIDs(t)
	cmds := make([]contract.Command, 0, len(ids))
	for id := range ids {
		cmds = append(cmds, contract.Command{ID: id})
	}
	if err := r.Register(cmds...); err != nil {
		t.Fatal(err)
	}
	if len(knownIDs) != len(ids) {
		t.Fatalf("known IDs = %d, contract declares %d", len(knownIDs), len(ids))
	}
	assertCode(t, r.Register(cmds[0]), contract.CodeExists)
	for _, id := range []contract.CommandID{"", "file.typo", "custom.save", "x."} {
		assertCode(t, r.Register(contract.Command{ID: id}), contract.CodeInvalid)
	}
	for _, batch := range [][]contract.Command{
		{{ID: "x.new"}, {ID: "bad"}},
		{{ID: "x.new"}, {ID: "x.new"}},
		{{ID: "x.new"}, cmds[0]},
	} {
		if err := r.Register(batch...); err == nil {
			t.Fatal("invalid batch registered")
		}
		if _, ok := r.Lookup("x.new"); ok {
			t.Fatal("failed registration partially changed the registry")
		}
	}
	if err := r.Register(contract.Command{ID: "x.new"}); err != nil {
		t.Fatal(err)
	}
	all := r.All()
	all[0].ID = "mutated"
	if r.All()[0].ID != cmds[0].ID {
		t.Fatal("All aliases registry state")
	}
	if _, ok := r.Lookup("missing"); ok {
		t.Fatal("Lookup found an unknown command")
	}
	if err := New().Register(); err != nil {
		t.Fatal(err)
	}
}

func TestRunFailuresAndPanics(t *testing.T) {
	sentinel := errors.New("save failed")
	typed := contract.E(contract.CodeIO, "storage.Write", sentinel)
	wrapped := fmt.Errorf("saving: %w", typed)
	for _, tc := range []struct {
		name string
		cmd  contract.Command
		code contract.Code
		is   error
	}{
		{"plain error", contract.Command{Run: func() error { return sentinel }}, contract.CodeInternal, sentinel},
		{"contract error", contract.Command{Run: func() error { return typed }}, contract.CodeIO, typed},
		{"wrapped contract error", contract.Command{Run: func() error { return wrapped }}, contract.CodeIO, sentinel},
		{"string panic", contract.Command{Run: func() error { panic("broken") }}, contract.CodeInternal, nil},
		{"error panic", contract.Command{Run: func() error { panic(typed) }}, contract.CodeInternal, typed},
		{"nil panic", contract.Command{Run: func() error { panic(nil) }}, contract.CodeInternal, nil},
		{"enabled panic", contract.Command{Enabled: func() bool { panic(sentinel) }}, contract.CodeInternal, sentinel},
		{"no handler", contract.Command{}, contract.CodeInvalid, nil},
		{"disabled", contract.Command{Enabled: func() bool { return false }, Run: func() error {
			t.Fatal("disabled command ran")
			return nil
		}}, contract.CodeInvalid, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := New()
			tc.cmd.ID = contract.CmdFileSave
			if err := r.Register(tc.cmd); err != nil {
				t.Fatal(err)
			}
			err := r.Run(tc.cmd.ID)
			assertCode(t, err, tc.code)
			if tc.is != nil && !errors.Is(err, tc.is) {
				t.Errorf("Run error %v does not wrap %v", err, tc.is)
			}
			if got := err.(*contract.Error); got.Op != "commands.Run" || got.Detail == "" {
				t.Errorf("missing error context: %+v", got)
			}
		})
	}
	r := New()
	assertCode(t, r.Run(contract.CmdFileSave), contract.CodeNotFound)
	runs := 0
	if err := r.Register(contract.Command{
		ID: contract.CmdFileSave, Enabled: func() bool { return true },
		Run: func() error { runs++; return nil },
	}); err != nil {
		t.Fatal(err)
	}
	if err := r.Run(contract.CmdFileSave); err != nil || runs != 1 {
		t.Fatalf("enabled command: error %v, runs %d", err, runs)
	}
}

func TestBindings(t *testing.T) {
	save := contract.Chord{Mods: contract.ModPrimary, Key: "S"}
	other := contract.Chord{Key: "F2"}
	third := contract.Chord{Key: "F3"}
	for _, goos := range []string{"windows", "linux", "darwin"} {
		t.Run(goos, func(t *testing.T) {
			r := New(goos)
			defaults := []contract.KeyBinding{
				{Command: contract.CmdFileSave, Chord: save},
				{Command: contract.CmdFileSave, Chord: third},
				{Command: contract.CmdFileOpen, Chord: other, Platform: goos},
				{Command: contract.CmdFileClose, Chord: other, Platform: "plan9"},
			}
			r.SetDefaults(defaults)
			defaults[0].Chord = other
			if id, ok := r.Resolve(save); !ok || id != contract.CmdFileSave {
				t.Fatal("SetDefaults did not copy its input")
			}
			foreign := []contract.KeyBinding{{Command: contract.CmdFileSave, Chord: other, Platform: "plan9"}}
			if got := r.SetUserBindings(foreign); len(got) != 0 {
				t.Fatal(got)
			}
			if len(r.KeysFor(contract.CmdFileSave)) != 2 {
				t.Fatal("foreign binding replaced defaults")
			}
			user := []contract.KeyBinding{
				{Command: contract.CmdFileSave, Chord: other, Platform: goos},
				{Command: contract.CmdFileSave, Chord: other},
				{Command: contract.CmdFileSave, Chord: third},
				{Command: contract.CmdFileClose, Chord: other},
				{Command: contract.CmdFileClose, Chord: other},
			}
			got := r.SetUserBindings(user)
			want := []contract.BindingConflict{{Chord: other, Commands: []contract.CommandID{
				contract.CmdFileOpen, contract.CmdFileSave, contract.CmdFileClose,
			}}}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("conflicts = %+v, want %+v", got, want)
			}
			if id, ok := r.Resolve(other); ok || id != "" {
				t.Fatal("conflicted chord resolves")
			}
			if _, ok := r.Resolve(save); ok {
				t.Fatal("overridden default resolves")
			}
			if keys := r.KeysFor(contract.CmdFileSave); !reflect.DeepEqual(keys, []contract.Chord{other, third}) {
				t.Fatalf("KeysFor = %v", keys)
			}
			got[0].Commands[0] = "mutated"
			got[0].Chord = save
			if !reflect.DeepEqual(r.Conflicts(), want) {
				t.Fatal("conflicts alias registry state")
			}
			keys := r.KeysFor(contract.CmdFileSave)
			keys[0] = save
			if r.KeysFor(contract.CmdFileSave)[0] != other {
				t.Fatal("KeysFor aliases registry state")
			}
			user[0].Chord = save
			r.SetDefaults(nil)
			if len(r.Conflicts()) != 1 || r.KeysFor(contract.CmdFileSave)[0] != other {
				t.Fatal("user bindings were not copied or survived incorrectly")
			}
			if got := r.SetUserBindings(nil); len(got) != 0 || len(r.KeysFor(contract.CmdFileSave)) != 0 {
				t.Fatal("clearing bindings left active keys")
			}
		})
	}
}

func TestDefaultConflictRegistration(t *testing.T) {
	r := New("windows")
	chord := contract.Chord{Mods: contract.ModPrimary, Key: "S"}
	r.SetDefaults([]contract.KeyBinding{
		{Command: contract.CmdFileSave, Chord: chord},
		{Command: contract.CmdFileSave, Chord: chord},
		{Command: contract.CmdFileOpen, Chord: chord},
	})
	if got := r.Conflicts(); len(got) != 1 || len(got[0].Commands) != 2 {
		t.Fatalf("default conflict = %+v", got)
	}
	if err := r.Register(contract.Command{ID: contract.CmdFileSave}); err != nil {
		t.Fatal(err)
	}
	if id, ok := r.Resolve(chord); ok {
		t.Fatalf("a conflicting chord resolves to %s", id)
	}
}

func TestDefaultPlatform(t *testing.T) {
	for _, r := range []*Registry{New(), New("")} {
		if r.goos != runtime.GOOS {
			t.Fatalf("platform = %s, want %s", r.goos, runtime.GOOS)
		}
	}
}

func assertCode(t *testing.T, err error, want contract.Code) {
	t.Helper()
	if _, ok := err.(*contract.Error); !ok {
		t.Fatalf("error type = %T, want *contract.Error", err)
	}
	if got := contract.CodeOf(err); got != want {
		t.Fatalf("error code = %q, want %q: %v", got, want, err)
	}
}

func sourceCommandIDs(t *testing.T) map[contract.CommandID]bool {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "../../contract/commands.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	ids := make(map[contract.CommandID]bool)
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value := spec.(*ast.ValueSpec)
			typ, ok := value.Type.(*ast.Ident)
			if !ok || typ.Name != "CommandID" {
				continue
			}
			for _, expr := range value.Values {
				literal, ok := expr.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					t.Fatal("expected a string CommandID constant")
				}
				id, err := strconv.Unquote(literal.Value)
				if err != nil {
					t.Fatal(err)
				}
				ids[contract.CommandID(id)] = true
			}
		}
	}
	if len(ids) == 0 {
		t.Fatal("no command constants found")
	}
	return ids
}
