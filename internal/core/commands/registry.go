package commands

import (
	"fmt"
	"runtime"
	"strings"

	"aurink/internal/contract"
)

// Registry implements contract.CommandRegistry. Its methods run on the main thread.
type Registry struct {
	goos      string
	commands  map[contract.CommandID]contract.Command
	order     []contract.CommandID
	defaults  []contract.KeyBinding
	user      []contract.KeyBinding
	resolved  map[contract.Chord]contract.CommandID
	keys      map[contract.CommandID][]contract.Chord
	conflicts []contract.BindingConflict
}

var _ contract.CommandRegistry = (*Registry)(nil)

// New returns an empty registry for goos, or runtime.GOOS if omitted or empty.
func New(goos ...string) *Registry {
	return &Registry{goos: platform(goos), commands: make(map[contract.CommandID]contract.Command)}
}

func platform(goos []string) string {
	if len(goos) != 0 && goos[0] != "" {
		return goos[0]
	}
	return runtime.GOOS
}

// Register adds commands atomically, rejecting duplicate or unrecognized IDs.
// Experimental IDs must start with x. Binding conflicts do not stop a command
// from registering: a user's keymap must not take commands away.
func (r *Registry) Register(cmds ...contract.Command) error {
	seen := make(map[contract.CommandID]bool, len(cmds))
	for _, cmd := range cmds {
		if !knownIDs[cmd.ID] && !(strings.HasPrefix(string(cmd.ID), "x.") && len(cmd.ID) > 2) {
			return &contract.Error{Code: contract.CodeInvalid, Op: "commands.Register", Detail: string(cmd.ID)}
		}
		if _, exists := r.commands[cmd.ID]; exists || seen[cmd.ID] {
			return &contract.Error{Code: contract.CodeExists, Op: "commands.Register", Detail: string(cmd.ID)}
		}
		seen[cmd.ID] = true
	}
	for _, cmd := range cmds {
		r.commands[cmd.ID] = cmd
		r.order = append(r.order, cmd.ID)
	}
	return nil
}

// Lookup returns a command by ID.
func (r *Registry) Lookup(id contract.CommandID) (contract.Command, bool) {
	cmd, ok := r.commands[id]
	return cmd, ok
}

// All returns commands in registration order in an independent slice.
func (r *Registry) All() []contract.Command {
	all := make([]contract.Command, 0, len(r.order))
	for _, id := range r.order {
		all = append(all, r.commands[id])
	}
	return all
}

// Run invokes an enabled command, wrapping errors and recovering callback panics.
func (r *Registry) Run(id contract.CommandID) (err error) {
	defer func() {
		if value := recover(); value != nil {
			cause, ok := value.(error)
			if !ok {
				cause = fmt.Errorf("panic: %v", value)
			}
			err = &contract.Error{Code: contract.CodeInternal, Op: "commands.Run", Detail: string(id) + " panicked", Err: cause}
		}
	}()
	cmd, ok := r.Lookup(id)
	if !ok {
		return &contract.Error{Code: contract.CodeNotFound, Op: "commands.Run", Detail: string(id)}
	}
	if (cmd.Enabled != nil && !cmd.Enabled()) || cmd.Run == nil {
		return &contract.Error{Code: contract.CodeInvalid, Op: "commands.Run", Detail: string(id) + " is unavailable"}
	}
	if err := cmd.Run(); err != nil {
		return &contract.Error{Code: contract.CodeOf(err), Op: "commands.Run", Detail: string(id), Err: err}
	}
	return nil
}

// SetDefaults replaces the default bindings. Conflicts reports any collisions.
func (r *Registry) SetDefaults(bindings []contract.KeyBinding) {
	r.defaults = append([]contract.KeyBinding(nil), bindings...)
	r.rebuild()
}

// SetUserBindings replaces user bindings and returns all active conflicts.
// Only bindings for this platform replace a command's defaults.
func (r *Registry) SetUserBindings(bindings []contract.KeyBinding) []contract.BindingConflict {
	r.user = append([]contract.KeyBinding(nil), bindings...)
	r.rebuild()
	return r.Conflicts()
}

// Resolve returns the unique command for a chord. Conflicting chords do not resolve.
func (r *Registry) Resolve(chord contract.Chord) (contract.CommandID, bool) {
	id, ok := r.resolved[chord]
	return id, ok
}

// KeysFor returns the command's active chords in binding order, including conflicts.
func (r *Registry) KeysFor(id contract.CommandID) []contract.Chord {
	return append([]contract.Chord(nil), r.keys[id]...)
}

// Conflicts returns an independent snapshot of active binding conflicts.
// It also exposes default conflicts because SetDefaults has no error return.
func (r *Registry) Conflicts() []contract.BindingConflict {
	conflicts := make([]contract.BindingConflict, 0, len(r.conflicts))
	for _, conflict := range r.conflicts {
		conflict.Commands = append([]contract.CommandID(nil), conflict.Commands...)
		conflicts = append(conflicts, conflict)
	}
	return conflicts
}

func (r *Registry) rebuild() {
	overridden := make(map[contract.CommandID]bool)
	for _, binding := range r.user {
		if r.applies(binding) {
			overridden[binding.Command] = true
		}
	}
	owners := make(map[contract.Chord][]contract.CommandID)
	var chords []contract.Chord
	r.keys = make(map[contract.CommandID][]contract.Chord)
	add := func(binding contract.KeyBinding) {
		ids := owners[binding.Chord]
		for _, id := range ids {
			if id == binding.Command {
				return
			}
		}
		if len(ids) == 0 {
			chords = append(chords, binding.Chord)
		}
		owners[binding.Chord] = append(ids, binding.Command)
		r.keys[binding.Command] = append(r.keys[binding.Command], binding.Chord)
	}
	for _, binding := range r.defaults {
		if r.applies(binding) && !overridden[binding.Command] {
			add(binding)
		}
	}
	for _, binding := range r.user {
		if r.applies(binding) {
			add(binding)
		}
	}
	r.resolved = make(map[contract.Chord]contract.CommandID)
	r.conflicts = nil
	for _, chord := range chords {
		ids := owners[chord]
		if len(ids) == 1 {
			r.resolved[chord] = ids[0]
		} else {
			r.conflicts = append(r.conflicts, contract.BindingConflict{Chord: chord, Commands: ids})
		}
	}
}

func (r *Registry) applies(binding contract.KeyBinding) bool {
	return binding.Platform == "" || binding.Platform == r.goos
}

var knownIDs = map[contract.CommandID]bool{
	contract.CmdFileNew: true, contract.CmdFileOpen: true, contract.CmdFileOpenFolder: true,
	contract.CmdFileSave: true, contract.CmdFileSaveAs: true, contract.CmdFileRename: true,
	contract.CmdFileRevert: true, contract.CmdFileClose: true, contract.CmdFileReopenClosed: true,
	contract.CmdFileExportHTML: true, contract.CmdFileExportPDF: true, contract.CmdFilePrint: true,
	contract.CmdFileReveal: true,
	contract.CmdEditUndo:   true, contract.CmdEditRedo: true, contract.CmdEditCut: true,
	contract.CmdEditCopy: true, contract.CmdEditPaste: true, contract.CmdEditPastePlain: true,
	contract.CmdEditCopyMarkdown: true, contract.CmdEditCopyHTML: true, contract.CmdEditSelectAll: true,
	contract.CmdEditFind: true, contract.CmdEditReplace: true, contract.CmdEditFindNext: true,
	contract.CmdEditFindPrev: true,
	contract.CmdFormatStrong: true, contract.CmdFormatEmphasis: true, contract.CmdFormatUnderline: true,
	contract.CmdFormatStrike: true, contract.CmdFormatCode: true, contract.CmdFormatHighlight: true,
	contract.CmdFormatSup: true, contract.CmdFormatSub: true,
	contract.CmdFormatLink: true, contract.CmdFormatClear: true,
	contract.CmdBlockHeading1: true, contract.CmdBlockHeading2: true, contract.CmdBlockHeading3: true,
	contract.CmdBlockHeading4: true, contract.CmdBlockHeading5: true, contract.CmdBlockHeading6: true,
	contract.CmdNavGoToTab1: true, contract.CmdNavGoToTab2: true, contract.CmdNavGoToTab3: true,
	contract.CmdNavGoToTab4: true, contract.CmdNavGoToTab5: true, contract.CmdNavGoToTab6: true,
	contract.CmdNavGoToTab7: true, contract.CmdNavGoToTab8: true, contract.CmdNavGoToTab9: true,
	contract.CmdBlockParagraph: true, contract.CmdBlockHeading: true, contract.CmdBlockQuote: true,
	contract.CmdBlockBulletList: true, contract.CmdBlockOrderedList: true, contract.CmdBlockTaskList: true,
	contract.CmdBlockToggleTask: true, contract.CmdBlockCode: true, contract.CmdBlockMath: true,
	contract.CmdBlockMermaid: true, contract.CmdBlockHR: true, contract.CmdBlockTOC: true,
	contract.CmdBlockIndent: true, contract.CmdBlockOutdent: true, contract.CmdBlockMoveUp: true,
	contract.CmdBlockMoveDown: true,
	contract.CmdTableInsert:   true, contract.CmdTableRowAbove: true, contract.CmdTableRowBelow: true,
	contract.CmdTableColLeft: true, contract.CmdTableColRight: true, contract.CmdTableDeleteRow: true,
	contract.CmdTableDeleteCol: true, contract.CmdTableAlign: true, contract.CmdTableFormat: true,
	contract.CmdTableNextCell: true, contract.CmdTablePrevCell: true,
	contract.CmdInsertImages: true, contract.CmdInsertLink: true, contract.CmdInsertFootnote: true,
	contract.CmdInsertFrontMatter: true,
	contract.CmdViewToggleSidebar: true, contract.CmdViewFiles: true, contract.CmdViewOutline: true,
	contract.CmdViewSearch: true, contract.CmdViewLive: true, contract.CmdViewSource: true,
	contract.CmdViewRead: true, contract.CmdViewFocus: true, contract.CmdViewTypewriter: true,
	contract.CmdViewZoomIn: true, contract.CmdViewZoomOut: true, contract.CmdViewZoomReset: true,
	contract.CmdViewThemeLight: true, contract.CmdViewThemeDark: true, contract.CmdViewThemeSystem: true,
	contract.CmdNavQuickOpen: true, contract.CmdNavPalette: true, contract.CmdNavGoToHeading: true,
	contract.CmdNavNextTab: true, contract.CmdNavPrevTab: true, contract.CmdNavSearchWorkspace: true,
	contract.CmdAppSettings: true, contract.CmdAppAbout: true, contract.CmdAppQuit: true,
}
