package commands

import "aurink/internal/contract"

// DefaultBindings returns the shortcuts in docs/02-product-ux.md section 7.
// Tab, Shift+Tab and Escape are left out: their meaning depends on the focused
// view (list, table, IME, popup, reading mode), which handles them itself.
func DefaultBindings() []contract.KeyBinding {
	const primary = contract.ModPrimary
	const shifted = primary | contract.ModShift
	bindings := []contract.KeyBinding{
		{Command: contract.CmdNavPalette, Chord: contract.Chord{Mods: shifted, Key: "P"}},
		{Command: contract.CmdNavQuickOpen, Chord: contract.Chord{Mods: primary, Key: "P"}},
		{Command: contract.CmdFormatStrong, Chord: contract.Chord{Mods: primary, Key: "B"}},
		{Command: contract.CmdFormatEmphasis, Chord: contract.Chord{Mods: primary, Key: "I"}},
		{Command: contract.CmdFormatUnderline, Chord: contract.Chord{Mods: primary, Key: "U"}},
		{Command: contract.CmdFormatCode, Chord: contract.Chord{Mods: shifted, Key: "`"}},
		{Command: contract.CmdFormatStrike, Chord: contract.Chord{Mods: shifted, Key: "X"}},
		{Command: contract.CmdFormatLink, Chord: contract.Chord{Mods: primary, Key: "K"}},
		{Command: contract.CmdBlockParagraph, Chord: contract.Chord{Mods: primary, Key: "0"}},
		{Command: contract.CmdFormatHighlight, Chord: contract.Chord{Mods: shifted, Key: "H"}},
		{Command: contract.CmdBlockOrderedList, Chord: contract.Chord{Mods: shifted, Key: "7"}},
		{Command: contract.CmdBlockBulletList, Chord: contract.Chord{Mods: shifted, Key: "8"}},
		{Command: contract.CmdBlockTaskList, Chord: contract.Chord{Mods: shifted, Key: "9"}},
		{Command: contract.CmdBlockQuote, Chord: contract.Chord{Mods: shifted, Key: "Q"}},
		{Command: contract.CmdBlockCode, Chord: contract.Chord{Mods: shifted, Key: "K"}},
		{Command: contract.CmdBlockMath, Chord: contract.Chord{Mods: shifted, Key: "M"}},
		{Command: contract.CmdTableInsert, Chord: contract.Chord{Mods: primary, Key: "T"}},
		{Command: contract.CmdBlockIndent, Chord: contract.Chord{Mods: primary, Key: "]"}},
		{Command: contract.CmdBlockOutdent, Chord: contract.Chord{Mods: primary, Key: "["}},
		{Command: contract.CmdViewSource, Chord: contract.Chord{Mods: primary, Key: "/"}},
		{Command: contract.CmdViewRead, Chord: contract.Chord{Key: "F7"}},
		{Command: contract.CmdViewFocus, Chord: contract.Chord{Key: "F8"}},
		{Command: contract.CmdViewTypewriter, Chord: contract.Chord{Key: "F9"}},
		{Command: contract.CmdViewToggleSidebar, Chord: contract.Chord{Mods: shifted, Key: "L"}},
		{Command: contract.CmdViewOutline, Chord: contract.Chord{Mods: shifted, Key: "1"}},
		{Command: contract.CmdNavSearchWorkspace, Chord: contract.Chord{Mods: shifted, Key: "F"}},
		{Command: contract.CmdEditFind, Chord: contract.Chord{Mods: primary, Key: "F"}},
		{Command: contract.CmdEditReplace, Chord: contract.Chord{Mods: primary, Key: "H"}, Platform: "windows"},
		{Command: contract.CmdEditReplace, Chord: contract.Chord{Mods: primary, Key: "H"}, Platform: "linux"},
		{Command: contract.CmdEditReplace, Chord: contract.Chord{Mods: primary | contract.ModAlt, Key: "F"}, Platform: "darwin"},
		{Command: contract.CmdFileSave, Chord: contract.Chord{Mods: primary, Key: "S"}},
		{Command: contract.CmdFileSaveAs, Chord: contract.Chord{Mods: shifted, Key: "S"}},
		{Command: contract.CmdFileNew, Chord: contract.Chord{Mods: primary, Key: "N"}},
		{Command: contract.CmdFileOpen, Chord: contract.Chord{Mods: primary, Key: "O"}},
		{Command: contract.CmdFileClose, Chord: contract.Chord{Mods: primary, Key: "W"}},
		{Command: contract.CmdFileReopenClosed, Chord: contract.Chord{Mods: shifted, Key: "T"}},
		{Command: contract.CmdNavNextTab, Chord: contract.Chord{Mods: primary, Key: "Tab"}},
		{Command: contract.CmdFileRename, Chord: contract.Chord{Key: "F2"}},
		{Command: contract.CmdEditPastePlain, Chord: contract.Chord{Mods: shifted, Key: "V"}},
		{Command: contract.CmdEditFindNext, Chord: contract.Chord{Key: "F3"}, Platform: "windows"},
		{Command: contract.CmdEditFindNext, Chord: contract.Chord{Key: "F3"}, Platform: "linux"},
		{Command: contract.CmdEditFindNext, Chord: contract.Chord{Mods: primary, Key: "G"}, Platform: "darwin"},
		{Command: contract.CmdEditFindPrev, Chord: contract.Chord{Mods: contract.ModShift, Key: "F3"}, Platform: "windows"},
		{Command: contract.CmdEditFindPrev, Chord: contract.Chord{Mods: contract.ModShift, Key: "F3"}, Platform: "linux"},
		{Command: contract.CmdEditFindPrev, Chord: contract.Chord{Mods: shifted, Key: "G"}, Platform: "darwin"},
	}
	for i, id := range []contract.CommandID{
		contract.CmdBlockHeading1, contract.CmdBlockHeading2, contract.CmdBlockHeading3,
		contract.CmdBlockHeading4, contract.CmdBlockHeading5, contract.CmdBlockHeading6,
	} {
		bindings = append(bindings, contract.KeyBinding{Command: id, Chord: contract.Chord{Mods: primary, Key: string(rune('1' + i))}})
	}
	for i, id := range []contract.CommandID{
		contract.CmdNavGoToTab1, contract.CmdNavGoToTab2, contract.CmdNavGoToTab3,
		contract.CmdNavGoToTab4, contract.CmdNavGoToTab5, contract.CmdNavGoToTab6,
		contract.CmdNavGoToTab7, contract.CmdNavGoToTab8, contract.CmdNavGoToTab9,
	} {
		key := string(rune('1' + i))
		bindings = append(bindings,
			contract.KeyBinding{Command: id, Chord: contract.Chord{Mods: contract.ModAlt, Key: key}, Platform: "windows"},
			contract.KeyBinding{Command: id, Chord: contract.Chord{Mods: contract.ModAlt, Key: key}, Platform: "linux"},
			contract.KeyBinding{Command: id, Chord: contract.Chord{Mods: primary | contract.ModAlt, Key: key}, Platform: "darwin"},
		)
	}
	return bindings
}
