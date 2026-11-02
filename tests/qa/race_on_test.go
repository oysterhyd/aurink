//go:build race

package qa

// raceOn: the race detector slows the code several times over, so time
// budgets do not apply.
const raceOn = true
