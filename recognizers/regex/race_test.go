//go:build race

package regex

// raceEnabled reports that this binary is built with the race detector, so the
// bounded-time test scales its wall-clock budget (see boundedBudget).
const raceEnabled = true
