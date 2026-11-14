//go:build cgo

package facedetect

import (
	"bytes"
	"image"
	"image/jpeg"

	"github.com/Kagami/go-face"
)

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

// NewDetector creates a FaceDetector, preferring a go-face-backed
// implementation when dlib and the required models are available,
// falling back to StubDetector otherwise.
func NewDetector() FaceDetector {
	if d := newRealDetector(); d != nil {
		return d
	}
	return StubDetector{}
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

