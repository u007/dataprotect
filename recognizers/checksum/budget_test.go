package checksum

import "time"

// boundedBudget scales a wall-clock budget for the race detector, which this
// package's bounded-time tests use instead of measuring raw speed.
//
// The detector instruments every memory access, so the same input costs a
// multiple of what it costs without it. Measured on 2026-10-22: the 1 MB digit
// run went from 1.16 s to 7.65 s and the 4 MB IBAN run from 1.28 s to 5.87 s,
// so a 5 s budget tuned without the detector failed under it.
//
// The input is deliberately NOT shrunk for the race build. Input size is what
// makes a superlinear regression visible: the old cubic window search took
// minutes on the 1 MB run, but the same search on a quarter of the input would
// finish well inside any budget and the test would go green while the algorithm
// was still cubic. Scaling the clock keeps the same work under measurement; the
// unbounded case is the one that has to be caught.
func boundedBudget(base time.Duration) time.Duration {
	if raceEnabled {
		// 8x: the measured race runtimes were 5.9 s and 7.7 s against a 5 s base,
		// so this leaves about five times headroom for a slower CI machine. Still
		// far below the minutes a cubic regression needs, so the bound keeps its
		// teeth.
		return 8 * base
	}
	return base
}
