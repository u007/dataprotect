package regex

import "time"

// boundedBudget scales a wall-clock budget for the race detector, which this
// package's bounded-time test uses instead of measuring raw speed.
//
// The detector instruments every memory access, so the same input costs a
// multiple of what it costs without it. Measured on 2026-10-22: the 1 MB mixed
// run went from 1.21 s to 12.91 s, the largest multiple of the three bounded
// tests in the repository, which is why a 5 s budget tuned without the detector
// failed under it.
//
// The input is deliberately NOT shrunk for the race build. Input size is what
// makes a superlinear regression visible, and a smaller input would let a
// superlinear scan finish inside any budget. Scaling the clock keeps the same
// work under measurement.
func boundedBudget(base time.Duration) time.Duration {
	if raceEnabled {
		// 8x: the measured race runtime was 12.9 s against a 5 s base, so this
		// leaves about three times headroom for a slower CI machine. Still far
		// below the minutes a cubic regression needs, so the bound keeps its teeth.
		return 8 * base
	}
	return base
}
