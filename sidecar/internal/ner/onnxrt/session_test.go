//go:build onnx

package onnxrt

import (
	"strings"
	"testing"
)

// The success path needs a real libonnxruntime, which the test machine does not
// have. These tests cover the failure path: a failed initialisation must not be
// cached, so a later call with another path reports that path's own error.
func TestInitEnvironmentFailureIsNotCached(t *testing.T) {
	const first = "/nonexistent/first/libonnxruntime.so"
	const second = "/nonexistent/second/libonnxruntime.so"

	err := InitEnvironment(first)
	if err == nil || !strings.Contains(err.Error(), first) {
		t.Fatalf("InitEnvironment(%q) = %v, want an error naming the path", first, err)
	}

	err = InitEnvironment(second)
	if err == nil {
		t.Fatalf("InitEnvironment(%q) succeeded, want an error", second)
	}
	if strings.Contains(err.Error(), "already initialised") {
		t.Fatalf("InitEnvironment(%q) reported a stale state after a failure: %v", second, err)
	}
	if !strings.Contains(err.Error(), second) {
		t.Fatalf("InitEnvironment(%q) = %v, want the error to name the path it tried", second, err)
	}
}
