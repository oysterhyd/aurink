package commands

import (
	"runtime"
	"strconv"
	"strings"
	"testing"

	"aurink/internal/contract"
)

func TestChordDisplay(t *testing.T) {
	for _, tc := range []struct {
		chord contract.Chord
		other string
		mac   string
	}{
		{contract.Chord{Mods: contract.ModPrimary | contract.ModShift, Key: "P"}, "Ctrl+Shift+P", "⌘⇧P"},
		{contract.Chord{Mods: contract.ModPrimary, Key: "S"}, "Ctrl+S", "⌘S"},
		{contract.Chord{Mods: contract.ModPrimary | contract.ModAlt, Key: "F"}, "Ctrl+Alt+F", "⌘⌥F"},
		{contract.Chord{Mods: contract.ModCtrlMac | contract.ModShift, Key: "Tab"}, "CtrlMac+Shift+Tab", "⌃⇧Tab"},
		{contract.Chord{Mods: contract.ModAlt, Key: "1"}, "Alt+1", "⌥1"},
		{contract.Chord{Mods: contract.ModShift, Key: "Tab"}, "Shift+Tab", "⇧Tab"},
		{contract.Chord{Key: "Escape"}, "Escape", "Escape"},
		{contract.Chord{Key: "F7"}, "F7", "F7"},
	} {
		for _, goos := range []string{"windows", "linux", "darwin"} {
			want := tc.other
			if goos == "darwin" {
				want = tc.mac
			}
			if got := FormatChord(tc.chord, goos); got != want {
				t.Errorf("FormatChord(%+v, %s) = %q, want %q", tc.chord, goos, got, want)
			}
		}
	}
}

func TestParseChord(t *testing.T) {
	for _, tc := range []struct {
		text string
		goos string
		want contract.Chord
	}{
		{"Ctrl+Shift+P", "windows", contract.Chord{Mods: contract.ModPrimary | contract.ModShift, Key: "P"}},
		{"Ctrl+Shift+P", "linux", contract.Chord{Mods: contract.ModPrimary | contract.ModShift, Key: "P"}},
		{"Ctrl+Shift+P", "darwin", contract.Chord{Mods: contract.ModCtrlMac | contract.ModShift, Key: "P"}},
		{"Primary+S", "darwin", contract.Chord{Mods: contract.ModPrimary, Key: "S"}},
		{" control + shift + pageup ", "linux", contract.Chord{Mods: contract.ModPrimary | contract.ModShift, Key: "PageUp"}},
		{"Option+Command+f", "darwin", contract.Chord{Mods: contract.ModPrimary | contract.ModAlt, Key: "F"}},
		{"⌘⇧P", "darwin", contract.Chord{Mods: contract.ModPrimary | contract.ModShift, Key: "P"}},
		{"⌃⌥Enter", "darwin", contract.Chord{Mods: contract.ModCtrlMac | contract.ModAlt, Key: "Enter"}},
		{"f12", "windows", contract.Chord{Key: "F12"}},
		{"Primary+\\", "windows", contract.Chord{Mods: contract.ModPrimary, Key: "\\"}},
		{"space", "darwin", contract.Chord{Key: "Space"}},
	} {
		got, err := ParseChord(tc.text, tc.goos)
		if err != nil || got != tc.want {
			t.Errorf("ParseChord(%q, %s) = %+v, %v, want %+v", tc.text, tc.goos, got, err, tc.want)
		}
	}
	for _, args := range [][]string{nil, {""}, {runtime.GOOS}} {
		want := contract.Chord{Mods: contract.ModPrimary, Key: "S"}
		got, err := ParseChord("Primary+S", args...)
		if err != nil || got != want || FormatChord(want, args...) != FormatChord(want, runtime.GOOS) {
			t.Errorf("default platform with %v: %+v, %v", args, got, err)
		}
	}
}

func TestInvalidChords(t *testing.T) {
	for _, goos := range []string{"windows", "linux", "darwin"} {
		for _, text := range []string{
			"", " ", "Ctrl", "Primary", "Ctrl+", "+P", "Ctrl++P", "Ctrl+Shift",
			"Ctrl+Ctrl+P", "Primary+Cmd+P", "Alt+Option+P", "⌘⌘P", "Shift+Shift+P",
			"P+Ctrl", "Super+P", "Ctrl+P+Q", "Ctrl+F0", "F13", "F01", "Esc", "Return",
			"Ctrl+Plus", "Ctrl++", "Ctrl+?", "Ctrl+é", "Ctrl+中", "Ctrl+😀", "Ctrl+PP", "ſ", "ſpace", "ı",
		} {
			got, err := ParseChord(text, goos)
			assertCode(t, err, contract.CodeInvalid)
			if got != (contract.Chord{}) {
				t.Errorf("invalid %q returned a partial chord: %+v", text, got)
			}
		}
	}
	for _, chord := range []contract.Chord{{}, {Key: "F13"}, {Key: "P", Mods: 128}} {
		if got := FormatChord(chord); got != "" {
			t.Errorf("invalid chord %+v displayed as %q", chord, got)
		}
	}
}

func TestChordRoundTrip(t *testing.T) {
	keys := strings.Fields("Enter Tab Escape Backspace Delete Up Down Left Right Home End PageUp PageDown Space")
	for _, key := range "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789`-=[]\\;',./" {
		keys = append(keys, string(key))
	}
	for i := 1; i <= 12; i++ {
		keys = append(keys, "F"+strconv.Itoa(i))
	}
	for _, goos := range []string{"windows", "linux", "darwin"} {
		for mods := contract.Mods(0); mods <= allMods; mods++ {
			for _, key := range keys {
				want := contract.Chord{Mods: mods, Key: key}
				display := FormatChord(want, goos)
				got, err := ParseChord(display, goos)
				if err != nil || got != want {
					t.Fatalf("round trip %+v on %s via %q = %+v, %v", want, goos, display, got, err)
				}
			}
		}
	}
}
