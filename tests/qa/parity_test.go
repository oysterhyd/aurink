package qa

import (
	"encoding/json"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestTyporaParity runs tools/parity/scenarios.json in Aurink and compares
// what each scenario leaves in the document with what Typora 1.0 left
// (tools/parity/typora.json, recorded by tools/parity/typora.ps1 on the
// owner's machine, 2026-11-04). A scenario that differs must be listed in
// tools/parity/accepted.json: "deviation", decided not to follow Typora, or
// "pending", a card not done yet. A pending one that matches must leave the
// list, so the list only shrinks. With AURINK_PARITY_OUT set, the results are
// also written there ({name: source}) for tools/parity/compare.py.
func TestTyporaParity(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("the scenarios are Typora for Windows keys (Ctrl, Home, End); on macOS those keys mean other things")
	}
	read := func(name string, v any) {
		raw, err := os.ReadFile("../../tools/parity/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, v); err != nil {
			t.Fatal(name, err)
		}
	}
	var scenarios []struct{ Name, Doc, Keys string }
	var typora map[string]string
	var accepted map[string]struct{ Kind, Why string }
	read("scenarios.json", &scenarios)
	read("typora.json", &typora)
	read("accepted.json", &accepted)
	only := os.Getenv("AURINK_PARITY_ONLY")
	results := map[string]string{}
	norm := func(s string) string { return strings.TrimRight(strings.ReplaceAll(s, "\r\n", "\n"), "\n") }
	for _, s := range scenarios {
		if only != "" && s.Name != only {
			continue
		}
		a := newApp(t)
		a.open("par_"+s.Name+".md", s.Doc)
		a.caret(0)
		for _, st := range paritySteps(t, s.Keys) {
			if st.text != "" {
				a.typed(st.text)
			} else {
				a.key(st.mods, st.key)
			}
			a.settle()
		}
		got := a.text()
		results[s.Name] = got
		want, recorded := typora[s.Name]
		if !recorded {
			t.Errorf("%s: no Typora result recorded; run tools/parity/typora.ps1", s.Name)
			continue
		}
		acc, listed := accepted[s.Name]
		same := norm(got) == norm(want)
		switch {
		case !same && !listed:
			t.Errorf("%s: %q\n  typora %q\n  aurink %q", s.Name, s.Keys, want, got)
		case same && listed && acc.Kind == "pending":
			t.Errorf("%s: now as in Typora; take it off tools/parity/accepted.json", s.Name)
		}
	}
	if out := os.Getenv("AURINK_PARITY_OUT"); out != "" {
		b, _ := json.MarshalIndent(results, "", "  ")
		if err := os.WriteFile(out, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

type parityStep struct {
	mods ui.Modifiers
	key  ui.Key
	text string
}

var parityNamed = map[string]ui.Key{
	"ENTER": ui.KeyEnter, "BS": ui.KeyBackspace, "DEL": ui.KeyDelete, "TAB": ui.KeyTab, "HOME": ui.KeyHome,
	"END": ui.KeyEnd, "UP": ui.KeyUp, "DOWN": ui.KeyDown, "LEFT": ui.KeyLeft, "RIGHT": ui.KeyRight, "ESC": ui.KeyEscape,
}

// paritySteps reads the keys of a scenario: text, and {MODS+KEY} tokens
// with C (Ctrl), S (Shift) and A (Alt), as tools/parity/typora.ps1 does.
func paritySteps(t *testing.T, keys string) []parityStep {
	var steps []parityStep
	for keys != "" {
		if keys[0] != '{' {
			r := []rune(keys)[0]
			steps = append(steps, parityStep{text: string(r)})
			keys = keys[len(string(r)):]
			continue
		}
		end := strings.IndexByte(keys[2:], '}') + 2
		tok := keys[1:end]
		keys = keys[end+1:]
		var st parityStep
		if i := strings.IndexByte(tok, '+'); i > 0 && strings.Trim(tok[:i], "CSA") == "" {
			for _, m := range tok[:i] {
				switch m {
				case 'C':
					st.mods |= ui.Ctrl
				case 'S':
					st.mods |= ui.Shift
				case 'A':
					st.mods |= ui.Alt
				}
			}
			tok = tok[i+1:]
		}
		if k, ok := parityNamed[tok]; ok {
			st.key = k
		} else {
			switch c := tok[0]; {
			case c >= 'a' && c <= 'z':
				st.key = ui.KeyA + ui.Key(c-'a')
			case c >= '0' && c <= '9':
				st.key = ui.Key0 + ui.Key(c-'0')
			default:
				k, ok := map[byte]ui.Key{'-': ui.KeyMinus, '=': ui.KeyEqual, '[': ui.KeyBracketLeft, ']': ui.KeyBracketRight, '`': ui.KeyBackquote}[c]
				if !ok {
					t.Fatalf("no key for %q", tok)
				}
				st.key = k
			}
		}
		steps = append(steps, st)
	}
	return steps
}
