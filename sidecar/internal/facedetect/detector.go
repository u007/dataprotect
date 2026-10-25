package facedetect

import (
	"errors"
	"image"
)

// ErrDlibUnavailable is returned when face detection is requested but
// dlib/go-face is not available on this system. dlib is a C++ system
// dependency (libdlib-dev on Debian/Ubuntu) that must be installed
// separately; it cannot be vendored or bundled in a Go module.
var ErrDlibUnavailable = errors.New("face detection unavailable: dlib (libdlib-dev) is not installed")

// Face describes a detected face in an image.
type Face struct {
	// Bounding box of the face, in image coordinates.
	Rect image.Rectangle
	// Confidence score in [0, 1], where higher means more confident.
	// May be 0 if the backend does not provide confidence.
	Confidence float64
}

// FaceDetector detects faces in images.
//
// Implementations may wrap github.com/Kagami/go-face (which requires
// dlib). The StubDetector in this file always returns ErrDlibUnavailable
// so the package compiles without dlib.
type FaceDetector interface {
	// DetectFaces returns all faces found in the image.
	DetectFaces(img image.Image) ([]Face, error)
}

// StubDetector is a FaceDetector that always returns ErrDlibUnavailable.
// Use it when dlib is not installed; replace with a go-face-backed
// implementation when dlib is available.
type StubDetector struct{}

// DetectFaces implements FaceDetector for StubDetector.
func (StubDetector) DetectFaces(img image.Image) ([]Face, error) {
	return nil, ErrDlibUnavailable
}

// NewDetector creates a FaceDetector. Currently it always returns a
// StubDetector because dlib/go-face cannot be compiled in this
// environment. When libdlib-dev is installed and go-face is added to
// sidecar/go.mod, this constructor should return a go-face-backed
// detector instead.
func NewDetector() FaceDetector {
	return StubDetector{}
}

// RealDetector is a placeholder for the go-face-backed implementation.
// It is intentionally not implemented here because importing
// github.com/Kagami/go-face requires cgo and libdlib-dev, neither of
// which is available in this environment.
//
// When dlib is installed, implement RealDetector as:
//
//	type RealDetector struct { rec *face.Recognizer }
//	func (d *RealDetector) DetectFaces(img image.Image) ([]Face, error) {
//	    // use d.Recognize or d.Detect from go-face
//	}
type RealDetector struct{}
