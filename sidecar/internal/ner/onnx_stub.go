//go:build !onnx

package ner

import "fmt"

// NewONNXBackend is the !onnx stub. It makes -backend=onnx fail at startup with
// an actionable message instead of "unknown backend".
func NewONNXBackend(cfg ONNXConfig) (NERBackend, error) {
	return nil, fmt.Errorf("onnx backend requires a build with -tags onnx (go build -tags onnx); model=%q tokenizer=%q onnxruntime-lib=%q",
		cfg.ModelDir, cfg.TokenizerPath, cfg.ONNXRuntimeLib)
}
