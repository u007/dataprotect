//go:build cgo

package facedetect

import "testing"

// TestNewDetectorReturnsReal verifies NewDetector returns a RealDetector when
// dlib and the required models are available.
func TestNewDetectorReturnsReal(t *testing.T) {
	d := NewDetector()
	if _, ok := d.(StubDetector); ok {
		t.Skip("dlib or models unavailable; StubDetector returned")
	}
	if _, ok := d.(*RealDetector); !ok {
		t.Errorf("NewDetector() = %T, want *RealDetector", d)
	}
}
