//go:build !onnx

package ner

import "strings"
import "testing"

func TestNewONNXBackendStubReturnsTagError(t *testing.T) {
	b, err := NewONNXBackend(ONNXConfig{ModelDir: "m", TokenizerPath: "t", ONNXRuntimeLib: "l"})
	if err == nil {
		t.Fatal("expected error")
	}
	if b != nil {
		t.Fatal("expected nil backend")
	}
	if !strings.Contains(err.Error(), "-tags onnx") {
		t.Fatalf("error missing tag hint: %v", err)
	}
}

func TestValidateONNXConfigRejectsEmpty(t *testing.T) {
	cases := []ONNXConfig{
		{},
		{ModelDir: "m"},
		{ModelDir: "m", TokenizerPath: "t"},
	}
	for _, c := range cases {
		if err := ValidateONNXConfig(c); err == nil {
			t.Fatalf("expected error for %+v", c)
		}
	}
	if err := ValidateONNXConfig(ONNXConfig{ModelDir: "m", TokenizerPath: "t", ONNXRuntimeLib: "l"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
