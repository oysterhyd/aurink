package commands

import (
	"testing"

	"aurink/internal/contract"
)

func TestDefaultBindings(t *testing.T) {
	ids := sourceCommandIDs(t)
	for _, binding := range DefaultBindings() {
		if !ids[binding.Command] {
			t.Errorf("default uses an ID absent from contract/commands.go: %s", binding.Command)
		}
		if binding.Platform != "" && binding.Platform != "windows" && binding.Platform != "linux" && binding.Platform != "darwin" {
			t.Errorf("unexpected platform %q", binding.Platform)
		}
	}
	for _, goos := range []string{"windows", "linux", "darwin"} {
		t.Run(goos, func(t *testing.T) {
			r := New(goos)
			r.SetDefaults(DefaultBindings())
			if got := r.Conflicts(); len(got) != 0 {
				t.Fatalf("default conflicts: %+v", got)
			}
			if got := r.SetUserBindings(nil); len(got) != 0 {
				t.Fatalf("default conflicts through contract API: %+v", got)
			}
			replace, next, prev := "Primary+H", "F3", "Shift+F3"
			strike, bullet, ordered := "Alt+Shift+5", "Primary+Shift+]", "Primary+Shift+["
			if goos == "darwin" {
				replace, next, prev = "Primary+Alt+F", "Primary+G", "Primary+Shift+G"
				strike, bullet, ordered = "Ctrl+Shift+`", "Primary+Alt+U", "Primary+Alt+O"
			}
			want := []struct {
				key string
				id  contract.CommandID
			}{
				{"Primary+Shift+P", contract.CmdNavPalette},
				{"Primary+P", contract.CmdNavQuickOpen},
				{"Primary+B", contract.CmdFormatStrong},
				{"Primary+I", contract.CmdFormatEmphasis},
				{"Primary+U", contract.CmdFormatUnderline},
				{"Primary+Shift+`", contract.CmdFormatCode},
				{"Primary+Shift+X", contract.CmdFormatStrike},
				{"Primary+K", contract.CmdFormatLink},
				{"Primary+0", contract.CmdBlockParagraph},
				{"Primary+=", contract.CmdBlockHeadingUp},
				{"Primary+-", contract.CmdBlockHeadingDown},
				{strike, contract.CmdFormatStrike},
				{bullet, contract.CmdBlockBulletList},
				{ordered, contract.CmdBlockOrderedList},
				{"Primary+Shift+H", contract.CmdFormatHighlight},
				{"Primary+Shift+7", contract.CmdBlockOrderedList},
				{"Primary+Shift+8", contract.CmdBlockBulletList},
				{"Primary+Shift+9", contract.CmdBlockTaskList},
				{"Primary+Shift+Q", contract.CmdBlockQuote},
				{"Primary+Shift+K", contract.CmdBlockCode},
				{"Primary+Shift+M", contract.CmdBlockMath},
				{"Primary+T", contract.CmdTableInsert},
				{"Primary+]", contract.CmdBlockIndent},
				{"Primary+[", contract.CmdBlockOutdent},
				{"Primary+/", contract.CmdViewSource},
				{"F7", contract.CmdViewRead},
				{"F8", contract.CmdViewFocus},
				{"F9", contract.CmdViewTypewriter},
				{"Primary+Shift+=", contract.CmdViewZoomIn},
				{"Primary+Shift+-", contract.CmdViewZoomOut},
				{"Primary+Shift+0", contract.CmdViewZoomReset},
				{"Primary+Shift+L", contract.CmdViewToggleSidebar},
				{"Primary+Shift+1", contract.CmdViewOutline},
				{"Primary+Shift+F", contract.CmdNavSearchWorkspace},
				{"Primary+F", contract.CmdEditFind},
				{replace, contract.CmdEditReplace},
				{"Primary+S", contract.CmdFileSave},
				{"Primary+Shift+S", contract.CmdFileSaveAs},
				{"Primary+N", contract.CmdFileNew},
				{"Primary+O", contract.CmdFileOpen},
				{"Primary+W", contract.CmdFileClose},
				{"Primary+Shift+T", contract.CmdFileReopenClosed},
				{"Primary+Tab", contract.CmdNavNextTab},
				{"F2", contract.CmdFileRename},
				{"Primary+Shift+V", contract.CmdEditPastePlain},
				{next, contract.CmdEditFindNext},
				{prev, contract.CmdEditFindPrev},
			}
			tabMod := "Alt"
			if goos == "darwin" {
				tabMod = "Primary+Alt"
			}
			want = append(want,
				struct {
					key string
					id  contract.CommandID
				}{"Primary+1", contract.CmdBlockHeading1},
				struct {
					key string
					id  contract.CommandID
				}{"Primary+6", contract.CmdBlockHeading6},
				struct {
					key string
					id  contract.CommandID
				}{tabMod + "+1", contract.CmdNavGoToTab1},
				struct {
					key string
					id  contract.CommandID
				}{tabMod + "+9", contract.CmdNavGoToTab9},
			)
			active := 0
			for _, binding := range DefaultBindings() {
				if binding.Platform == "" || binding.Platform == goos {
					active++
				}
			}
			// want names 4 of the 15 numbered bindings.
			if active != len(want)+11 {
				t.Fatalf("%d active defaults, want %d", active, len(want)+11)
			}
			for _, binding := range want {
				chord, err := ParseChord(binding.key, goos)
				if err != nil {
					t.Fatal(err)
				}
				if id, ok := r.Resolve(chord); !ok || id != binding.id {
					t.Errorf("%s resolves to %s, %v; want %s", binding.key, id, ok, binding.id)
				}
			}
			for _, key := range []string{"Tab", "Shift+Tab", "Escape", "Primary+7", "Primary+Shift+Tab", "Primary+Z"} {
				chord, err := ParseChord(key, goos)
				if err != nil {
					t.Fatal(err)
				}
				if id, ok := r.Resolve(chord); ok {
					t.Errorf("unexpected default %s = %s", key, id)
				}
			}
			for id := range ids {
				if err := r.Register(contract.Command{ID: id}); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	bindings := DefaultBindings()
	bindings[0].Command = "mutated"
	if DefaultBindings()[0].Command != contract.CmdNavPalette {
		t.Fatal("DefaultBindings shares mutable state")
	}
}
