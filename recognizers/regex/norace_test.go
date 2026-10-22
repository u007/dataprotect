//go:build !race

package regex

// raceEnabled is false here: the binary is not built with the race detector, so
// the bounded-time test uses its budget unchanged (see boundedBudget).
const raceEnabled = false
