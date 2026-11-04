package editing

import (
	"testing"

	"aurink/internal/contract"
)

func TestEmptyTaskEnter(t *testing.T) {
	intentGoldens(t, "empty task", contract.Intent{Kind: contract.IntentNewline},
		golden{"- [ ] ¦", "¦"},
		golden{"1. [x] ¦", "¦"},
		golden{"> + [ ] ¦", "> ¦"},
		golden{"- [ ] a\n- [ ] ¦", "- [ ] a\n\n¦"},
		golden{"- parent\n  - [ ] ¦", "- parent\n- [ ] ¦"},
		golden{"- [ ]¦", "- [ ]\n- ¦"},
		golden{"- [x]¦", "- [x]\n- ¦"},
		golden{"1. [X]¦", "1. [X]\n2. ¦"})
}
