//go:build !race

package checksum

// raceEnabled is false here: the binary is not built with the race detector, so
// a bounded-time test uses its budget unchanged (see boundedBudget).
const raceEnabled = false
