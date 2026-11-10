package facedetect

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"

	"github.com/Kagami/go-face"
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

// RealDetector is a FaceDetector backed by github.com/Kagami/go-face.
// It requires libdlib-dev to be installed and go-face to compile via cgo.
type RealDetector struct {
	rec *face.Recognizer
}

// DetectFaces encodes the image as JPEG and runs go-face recognition.
// go-face does not return a confidence score, so Confidence is 1.0.
func (d *RealDetector) DetectFaces(img image.Image) ([]Face, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		return nil, err
	}
	faces, err := d.rec.Recognize(buf.Bytes())
	if err != nil {
		return nil, err
	}
	out := make([]Face, len(faces))
	for i, f := range faces {
		out[i] = Face{
			Rect:       f.Rectangle,
			Confidence: 1.0,
		}
	}
	return out, nil
}

// Close releases the underlying recognizer resources.
func (d *RealDetector) Close() error {
	d.rec.Close()
	return nil
}

// newRealDetector tries to create a go-face-backed detector.
// It returns nil if dlib or the required models are unavailable,
// so callers can fall back to StubDetector.
func newRealDetector() *RealDetector {
	modelDir, err := cacheDir()
	if err != nil {
		return nil
	}
	rec, err := face.NewRecognizer(modelDir)
	if err != nil {
		return nil
	}
	return &RealDetector{rec: rec}
}

// NewDetector creates a FaceDetector, preferring a go-face-backed
// implementation when dlib and the required models are available,
// falling back to StubDetector otherwise.
func NewDetector() FaceDetector {
	if d := newRealDetector(); d != nil {
		return d
	}
	return StubDetector{}
}
