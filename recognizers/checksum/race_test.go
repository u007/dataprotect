//go:build race

package checksum

// raceEnabled reports that this binary is built with the race detector, so the
// bounded-time tests scale their wall-clock budget (see boundedBudget).
const raceEnabled = true
