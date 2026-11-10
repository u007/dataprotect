# Third-party licences

This table documents upstream licences for optional components. The piiscrub
core itself is MIT; see [LICENSE](LICENSE). You inherit a component's terms only
when you choose to use it.

| Component | SPDX | Source | Trigger | Obligations |
|-----------|------|--------|---------|-------------|
| piiscrub core | MIT | [`github.com/u007/dataprotect`](https://github.com/u007/dataprotect) | any import | keep copyright notice |
| `onnxruntime_go` | Apache-2.0 *(unverified — module download blocked)* | [`github.com/yalue/onnxruntime_go`](https://github.com/yalue/onnxruntime_go) v1.35.0 | `NER_BACKEND=onnx` sidecar build | keep licence and copyright notice if redistributed |
| `go-face` | CC0-1.0 | [`github.com/Kagami/go-face`](https://github.com/Kagami/go-face) @ `v0.0.0-20210630145111-0c14797b4d0e` | `NewDetector()` when `libdlib-dev` present | none (public domain dedication) |
| dlib library | Boost-1.0 | [`github.com/davisking/dlib`](https://github.com/davisking/dlib) | `libdlib-dev` installed, compile via cgo | keep copyright notice |
| `shape_predictor_5_face_landmarks.dat` | verify (dlib.net model) | [`dlib.net/files/shape_predictor_5_face_landmarks.dat.bz2`](http://dlib.net/files/shape_predictor_5_face_landmarks.dat.bz2) | `RealDetector.DetectFaces()` (download on first use) | verify per dlib.net terms before download |
| `dlib_face_recognition_resnet_model_v1.dat` | verify (dlib.net model) | [`dlib.net/files/dlib_face_recognition_resnet_model_v1.dat.bz2`](http://dlib.net/files/dlib_face_recognition_resnet_model_v1.dat.bz2) | `RealDetector.DetectFaces()` (download on first use) | verify per dlib.net terms before download |
| Benchmark datasets | per [DATASET_LICENSES.md](DATASET_LICENSES.md) | various | `make corpus` / `make corpus-nlp` | per dataset terms |

## Model weights

dlib model `.dat` files are downloaded on first use from dlib.net and are not
bundled. Their licences may differ from the dlib library itself (Boost-1.0).
Some dlib models trained on the iBUG 300-W dataset carry non-commercial
restrictions. Verify each model file's licence at dlib.net before downloading.

## Not legal advice

This table documents upstream licences as recorded at the time of writing.
Licences can change; some corpus terms may restrict commercial use of anything
trained or evaluated on them. For a commercial launch, have counsel confirm.
