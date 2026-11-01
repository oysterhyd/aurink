package commands

import (
	"strings"

	"aurink/internal/contract"
)

const allMods = contract.ModPrimary | contract.ModCtrlMac | contract.ModAlt | contract.ModShift

// ParseChord parses a textual chord or a FormatChord display string.
// Primary always means ModPrimary. Ctrl means Control on darwin and Primary
// elsewhere. Cmd/Command, Alt/Option, Shift, and CtrlMac are also accepted.
// Names are case insensitive; keys use the names documented by contract.Chord.
// The platform defaults to runtime.GOOS if omitted or empty.
func ParseChord(text string, goos ...string) (contract.Chord, error) {
	invalid := func() (contract.Chord, error) {
		return contract.Chord{}, &contract.Error{Code: contract.CodeInvalid, Op: "commands.ParseChord", Detail: text}
	}
	parts := strings.Split(strings.NewReplacer("⌘", "Cmd+", "⌃", "CtrlMac+", "⌥", "Alt+", "⇧", "Shift+").Replace(strings.TrimSpace(text)), "+")
	var chord contract.Chord
	for _, part := range parts[:len(parts)-1] {
		var mod contract.Mods
		switch strings.ToLower(strings.TrimSpace(part)) {
		case "primary", "cmd", "command":
			mod = contract.ModPrimary
		case "ctrl", "control":
			mod = contract.ModPrimary
			if platform(goos) == "darwin" {
				mod = contract.ModCtrlMac
			}
		case "ctrlmac":
			mod = contract.ModCtrlMac
		case "alt", "option":
			mod = contract.ModAlt
		case "shift":
			mod = contract.ModShift
		default:
			return invalid()
		}
		if chord.Mods&mod != 0 {
			return invalid()
		}
		chord.Mods |= mod
	}
	key, ok := canonicalKey(strings.TrimSpace(parts[len(parts)-1]))
	if !ok {
		return invalid()
	}
	chord.Key = key
	return chord, nil
}

// FormatChord displays a chord with macOS symbols on darwin and names elsewhere.
// CtrlMac remains explicit outside darwin so parsing preserves every modifier.
// Invalid chords produce an empty string. The platform defaults to runtime.GOOS.
func FormatChord(chord contract.Chord, goos ...string) string {
	key, ok := canonicalKey(chord.Key)
	if !ok || chord.Mods & ^allMods != 0 {
		return ""
	}
	mac := platform(goos) == "darwin"
	var result strings.Builder
	for _, modifier := range []struct {
		mod    contract.Mods
		name   string
		symbol string
	}{
		{contract.ModPrimary, "Ctrl+", "⌘"},
		{contract.ModCtrlMac, "CtrlMac+", "⌃"},
		{contract.ModAlt, "Alt+", "⌥"},
		{contract.ModShift, "Shift+", "⇧"},
	} {
		if chord.Mods&modifier.mod != 0 {
			if mac {
				result.WriteString(modifier.symbol)
			} else {
				result.WriteString(modifier.name)
			}
		}
	}
	result.WriteString(key)
	return result.String()
}

func canonicalKey(key string) (string, bool) {
	for _, char := range key {
		if char > 127 {
			return "", false
		}
	}
	upper := strings.ToUpper(key)
	if len(upper) == 1 && ((upper[0] >= 'A' && upper[0] <= 'Z') ||
		(upper[0] >= '0' && upper[0] <= '9') || strings.ContainsAny(upper, "`-=[]\\;',./")) {
		return upper, true
	}
	switch upper {
	case "F1", "F2", "F3", "F4", "F5", "F6", "F7", "F8", "F9", "F10", "F11", "F12":
		return upper, true
	}
	key, ok := namedKeys[upper]
	return key, ok
}

var namedKeys = map[string]string{
	"ENTER": "Enter", "TAB": "Tab", "ESCAPE": "Escape", "BACKSPACE": "Backspace",
	"DELETE": "Delete", "UP": "Up", "DOWN": "Down", "LEFT": "Left", "RIGHT": "Right",
	"HOME": "Home", "END": "End", "PAGEUP": "PageUp", "PAGEDOWN": "PageDown", "SPACE": "Space",
}
