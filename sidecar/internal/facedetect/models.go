// Package facedetect provides face detection model management with
// "download on first use" bundling strategy.
//
// Model files are NOT bundled with the binary. On the first call that
// needs face detection, the required models are downloaded to an OS cache
// directory and reused on subsequent calls. This keeps the binary small
// and avoids shipping large model files that may never be used.
//
// The models are required by github.com/Kagami/go-face, which binds to
// dlib (a C++ library). dlib itself is a system dependency and must be
// installed separately (libdlib-dev on Debian/Ubuntu). This package only
// handles downloading and caching the model data files; the actual face
// recognition binding is provided by a separate backend once dlib is
// available.
package facedetect

import "time"

// Model describes a single downloadable face model.
type Model struct {
	// Name is a human-readable identifier for the model.
	Name string
	// URL is the download source. Must serve the raw .bz2 compressed file.
	URL string
	// Path is the relative path (within the model cache dir) where the
	// decompressed .dat file is stored after download.
	Path string
	// ExpectedSize is the approximate uncompressed size in bytes, used
	// only for logging and sanity checks. Zero means unknown.
	ExpectedSize int64
}

// Models are the three pre-trained models required by go-face.
//
// Sources:
//
//	shape_predictor_5_face_landmarks.dat — dlib 5-point facial landmark
//	  predictor (http://dlib.net/files/shape_predictor_5_face_landmarks.dat.bz2)
//	mmod_human_face_detector.dat — dlib CNN face detector
//	  (http://dlib.net/files/mmod_human_face_detector.dat.bz2)
//	dlib_face_recognition_resnet_model_v1.dat — dlib ResNet face
//	  recognition model producing 128D face descriptors
//	  (http://dlib.net/files/dlib_face_recognition_resnet_model_v1.dat.bz2)
//
// All files are distributed as .bz2 compressed archives and are
// decompressed transparently during download.
var Models = []Model{
	{
		Name:         "shape_predictor_5_face_landmarks",
		URL:          "http://dlib.net/files/shape_predictor_5_face_landmarks.dat.bz2",
		Path:         "shape_predictor_5_face_landmarks.dat",
		ExpectedSize: 23_000_000, // approx 22 MB
	},
	{
		Name:         "mmod_human_face_detector",
		URL:          "http://dlib.net/files/mmod_human_face_detector.dat.bz2",
		Path:         "mmod_human_face_detector.dat",
		ExpectedSize: 700_000, // approx 713 KB
	},
	{
		Name:         "dlib_face_recognition_resnet_model_v1",
		URL:          "http://dlib.net/files/dlib_face_recognition_resnet_model_v1.dat.bz2",
		Path:         "dlib_face_recognition_resnet_model_v1.dat",
		ExpectedSize: 100_000_000, // approx 97 MB
	},
}

// DownloadTimeout is the maximum time allowed for a single model download.
const DownloadTimeout = 10 * time.Minute

// MaxRetries is the number of download attempts before giving up.
const MaxRetries = 3
