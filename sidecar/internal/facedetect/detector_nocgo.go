//go:build !cgo

package facedetect

// NewDetector returns a StubDetector when cgo is unavailable, since the
// go-face-backed RealDetector requires libdlib-dev and cgo.
func NewDetector() FaceDetector {
	return StubDetector{}
}
