//go:build onnx

package onnxrt

import (
	"context"
	"os"
	"strings"
	"testing"
)

// The success path needs a real libonnxruntime, which the test machine does not
// have. These tests cover the failure path: a failed initialisation must not be
// cached, so a later call with another path reports that path's own error.
//
// Success-path coverage (InitEnvironment + NewSession + Run) is gated on the
// NER_ONNX_RUNTIME_LIB and NER_ONNX_MODEL environment variables, which name
// the shared library and the model directory (model.onnx). When either is
// unset the tests skip, so `go test -tags onnx` stays green on a bare host.
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

func TestInitEnvironmentSuccess(t *testing.T) {
	lib := os.Getenv("NER_ONNX_RUNTIME_LIB")
	if lib == "" {
		t.Skip("NER_ONNX_RUNTIME_LIB unset; skipping success-path InitEnvironment test")
	}
	if err := InitEnvironment(lib); err != nil {
		t.Fatalf("InitEnvironment(%q) = %v, want nil", lib, err)
	}
}

func TestSessionRunSuccess(t *testing.T) {
	lib := os.Getenv("NER_ONNX_RUNTIME_LIB")
	modelDir := os.Getenv("NER_ONNX_MODEL")
	if lib == "" || modelDir == "" {
		t.Skip("NER_ONNX_RUNTIME_LIB or NER_ONNX_MODEL unset; skipping success-path Session.Run test")
	}
	if err := InitEnvironment(lib); err != nil {
		t.Fatalf("InitEnvironment(%q) = %v", lib, err)
	}

	modelPath := modelDir + "/model.onnx"
	sess, err := NewSession(modelPath)
	if err != nil {
		t.Fatalf("NewSession(%q) = %v", modelPath, err)
	}
	defer func() { _ = sess.Close() }()

	if sess.LabelCount() <= 0 {
		t.Fatalf("LabelCount = %d, want > 0", sess.LabelCount())
	}

	feeds := map[string][]int64{}
	for _, name := range sess.InputNames() {
		feeds[name] = []int64{1}
	}
	rows, err := sess.Run(context.Background(), feeds)
	if err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	if len(rows) != 1 {
		t.Fatalf("Run returned %d rows, want 1", len(rows))
	}
	if len(rows[0]) != sess.LabelCount() {
		t.Fatalf("Run returned %d labels, want %d", len(rows[0]), sess.LabelCount())
	}
}
