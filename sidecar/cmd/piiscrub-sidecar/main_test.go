package main

import (
	"context"
	"errors"
	"testing"

	"github.com/u007/dataprotect/recognizers"
	"github.com/u007/dataprotect/sidecar/internal/ner"
)

type fakeBackend struct {
	name     string
	closes   int
	closeErr error
}

func (f *fakeBackend) Name() string { return f.name }

func (f *fakeBackend) Recognize(context.Context, string) ([]recognizers.Entity, error) {
	return nil, nil
}

func (f *fakeBackend) Close() error {
	f.closes++
	return f.closeErr
}

type plainBackend struct{}

func (plainBackend) Name() string { return "plain" }

func (plainBackend) Recognize(context.Context, string) ([]recognizers.Entity, error) {
	return nil, nil
}

func TestCloseBackendClosesBackend(t *testing.T) {
	b := &fakeBackend{name: "fake"}
	closeBackend(b)
	if b.closes != 1 {
		t.Fatalf("closes = %d, want 1", b.closes)
	}
}

func TestCloseBackendIgnoresBackendWithoutClose(t *testing.T) {
	closeBackend(plainBackend{})
}

func TestCloseBackendDoesNotPanicOnCloseError(t *testing.T) {
	b := &fakeBackend{name: "fake", closeErr: errors.New("close failed")}
	closeBackend(b)
	if b.closes != 1 {
		t.Fatalf("closes = %d, want 1", b.closes)
	}
}

func TestNewBackendSelectsProse(t *testing.T) {
	b, err := newBackend("prose", ner.ONNXConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if b.Name() != "prose" {
		t.Fatalf("name = %q, want prose", b.Name())
	}
}

func TestNewBackendRejectsUnknownBackend(t *testing.T) {
	if _, err := newBackend("bogus", ner.ONNXConfig{}); err == nil {
		t.Fatal("expected an error for an unknown backend")
	}
}

func TestNewBackendReportsONNXConstructorError(t *testing.T) {
	// Without -tags onnx this is the stub's tag hint; with -tags onnx it is
	// the artifact-path validation. Either way a failed constructor must
	// surface as an error, not a nil backend.
	if _, err := newBackend("onnx", ner.ONNXConfig{}); err == nil {
		t.Fatal("expected an error for an empty onnx config")
	}
}
