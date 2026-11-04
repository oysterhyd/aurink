package qa

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestTyporaParity runs tools/parity/scenarios.json in Aurink, as
// tools/parity/typora.ps1 runs it in Typora, and writes what each scenario
// leaves in the document to $AURINK_PARITY_OUT ({name: source}). Compare the
// two with tools/parity/compare.py. Skipped unless AURINK_PARITY_OUT is set.
func TestTyporaParity(t *testing.T) {
	out := os.Getenv("AURINK_PARITY_OUT")
	if out == "" {
		t.Skip("AURINK_PARITY_OUT not set")
	}
	raw, err := os.ReadFile("../../tools/parity/scenarios.json")
	if err != nil {
		t.Fatal(err)
	}
	var scenarios []struct{ Name, Doc, Keys string }
	if err := json.Unmarshal(raw, &scenarios); err != nil {
		t.Fatal(err)
	}
	only := os.Getenv("AURINK_PARITY_ONLY")
	results := map[string]string{}
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
		results[s.Name] = a.text()
	}
	b, _ := json.MarshalIndent(results, "", "  ")
	if err := os.WriteFile(out, b, 0o644); err != nil {
		t.Fatal(err)
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
