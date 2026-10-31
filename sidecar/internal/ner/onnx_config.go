package ner

import "fmt"

// ONNXConfig carries the artifact paths for the ONNX backend. It is defined in
// an untagged file so that both the !onnx stub and the onnx implementation
// share one signature, and so main.go can compile in a plain build.
type ONNXConfig struct {
	ModelDir       string
	TokenizerPath  string
	ONNXRuntimeLib string
}

// ValidateONNXConfig reports an error when any required artifact path is empty.
// The tagged backend calls this before touching the filesystem; no default or
// bundled path is ever substituted.
func ValidateONNXConfig(cfg ONNXConfig) error {
	if cfg.ModelDir == "" {
		return fmt.Errorf("onnx backend: -model is required")
	}
	if cfg.TokenizerPath == "" {
		return fmt.Errorf("onnx backend: -tokenizer is required")
	}
	if cfg.ONNXRuntimeLib == "" {
		return fmt.Errorf("onnx backend: -onnxruntime-lib is required")
	}
	return nil
}
