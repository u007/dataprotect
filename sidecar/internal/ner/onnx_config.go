package ner

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
)

// ONNXConfig carries the artifact paths for the ONNX backend. It is defined in
// an untagged file so that both the !onnx stub and the onnx implementation
// share one signature, and so main.go can compile in a plain build.
type ONNXConfig struct {
	ModelDir         string
	TokenizerPath    string
	ONNXRuntimeLib   string
	AddSpecialTokens bool
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

// modelConfig is the part of a model's config.json that defines its label set.
// id2label is the only source of labels: the backend never guesses the label
// order from the model's channel count.
type modelConfig struct {
	ID2Label map[string]string `json:"id2label"`
}

// loadID2Label reads config.json and returns the labels ordered by their numeric
// id. The mapping must be contiguous from 0: a gap means the config and the
// model disagree, and a label at the wrong channel is worse than a refusal to
// start.
func loadID2Label(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("model config %s: %w", path, err)
	}
	var cfg modelConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("model config %s: %w", path, err)
	}
	if len(cfg.ID2Label) == 0 {
		return nil, fmt.Errorf("model config %s has no id2label map", path)
	}

	keys := make([]int, 0, len(cfg.ID2Label))
	for key := range cfg.ID2Label {
		id, err := strconv.Atoi(key)
		if err != nil || id < 0 || id >= len(cfg.ID2Label) {
			return nil, fmt.Errorf("model config %s: id2label key %q is not an index in [0,%d)", path, key, len(cfg.ID2Label))
		}
		keys = append(keys, id)
	}
	sort.Ints(keys)

	labels := make([]string, len(keys))
	hasOutside := false
	for i, id := range keys {
		if id != i {
			return nil, fmt.Errorf("model config %s: id2label is not contiguous from 0 (missing %d)", path, i)
		}
		label := cfg.ID2Label[strconv.Itoa(id)]
		if label == "" {
			return nil, fmt.Errorf("model config %s: id2label[%d] is empty", path, id)
		}
		if label == "O" {
			hasOutside = true
		} else if labelPrefix(label) == "" {
			// A generic label such as LABEL_1 carries no BIO tag, so decode
			// could not tell where a span begins or ends.
			return nil, fmt.Errorf("model config %s: id2label[%d] %q is neither \"O\" nor a B-/I-/E-/S- tag", path, id, label)
		}
		labels[i] = label
	}
	if !hasOutside {
		return nil, fmt.Errorf("model config %s: id2label has no \"O\" label", path)
	}
	return labels, nil
}
