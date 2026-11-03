package effects

import (
	"testing"
	"time"
)

// The aurora drifts for 12 s and then stops asking for frames (02 §4.4 rule
// 4); with motion reduced it never moves.
func TestAuroraPhase(t *testing.T) {
	for _, c := range []struct {
		elapsed time.Duration
		still   bool
		phase   float32
		more    bool
	}{
		{0, false, 0, true},
		{8 * time.Second, false, 1, true},
		{11 * time.Second, false, -1, true},
		{12 * time.Second, false, .5, false},
		{time.Minute, false, .5, false},
		{5 * time.Second, true, 0, false},
		{0, true, 0, false},
	} {
		phase, more := AuroraPhase(c.elapsed, c.still)
		if more != c.more || (c.phase >= 0 && abs(phase-c.phase) > 1e-3) {
			t.Errorf("%v still %v: phase %v more %v", c.elapsed, c.still, phase, more)
		}
	}
	// It moves while it drifts.
	a, _ := AuroraPhase(2*time.Second, false)
	b, _ := AuroraPhase(3*time.Second, false)
	if a == b {
		t.Error("the aurora does not move")
	}
}

func abs(f float32) float32 {
	if f < 0 {
		return -f
	}
	return f
}
