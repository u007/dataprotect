//go:build onnx

// Package onnxrt is a thin Session over github.com/yalue/onnxruntime_go.
//
// The binding compiles against vendored headers and loads libonnxruntime at
// runtime, so a plain build neither runs cgo nor links ONNX Runtime. Every
// method here is a translation of one binding call; the decoding, chunking and
// label logic stay in package ner, where they are testable without a model.
package onnxrt

import (
	"context"
	"fmt"
	"log"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

var (
	envMu   sync.Mutex
	envDone bool
	envLib  string
)

// InitEnvironment initialises the process-wide ONNX Runtime from the shared
// library at libPath. There is no default path and no lookup on PATH: the
// operator names the library explicitly. A failed initialisation is not cached,
// so a later call with a corrected path can succeed. Once initialised, a call
// with the same path is a no-op, and a call with a different path is an error,
// because the environment is per-process and silently handing back the first
// library would make the flag a lie.
func InitEnvironment(libPath string) error {
	envMu.Lock()
	defer envMu.Unlock()

	if envDone {
		if envLib != libPath {
			return fmt.Errorf("onnxruntime already initialised from %q, cannot also load %q", envLib, libPath)
		}
		return nil
	}
	ort.SetSharedLibraryPath(libPath)
	if err := ort.InitializeEnvironment(); err != nil {
		return fmt.Errorf("onnxruntime %s: %w", libPath, err)
	}
	envDone = true
	envLib = libPath
	return nil
}

// destroyValues releases every non-nil tensor. A failed Run may leave some
// output slots unset, so nil entries are expected.
func destroyValues(vs []ort.Value) {
	for _, v := range vs {
		if v == nil {
			continue
		}
		if err := v.Destroy(); err != nil {
			log.Printf("onnxrt: destroying tensor %v: %v", v.GetShape(), err)
		}
	}
}

// Session wraps a dynamic ONNX session. It queries the model for its input and
// output names, so the backend never guesses them, and it records the model's
// output label dimension so startup can compare it with config.json's id2label.
type Session struct {
	sess        *ort.DynamicAdvancedSession
	inputNames  []string
	outputNames []string
	labelCount  int
}

// NewSession opens modelPath and inspects it. InitEnvironment must have
// succeeded first.
func NewSession(modelPath string) (*Session, error) {
	inputs, outputs, err := ort.GetInputOutputInfo(modelPath)
	if err != nil {
		return nil, fmt.Errorf("model %s: %w", modelPath, err)
	}
	if len(inputs) == 0 {
		return nil, fmt.Errorf("model %s declares no inputs", modelPath)
	}
	if len(outputs) != 1 {
		return nil, fmt.Errorf("model %s declares %d outputs, want 1", modelPath, len(outputs))
	}
	inputNames := make([]string, len(inputs))
	for i, in := range inputs {
		inputNames[i] = in.Name
	}
	outputNames := []string{outputs[0].Name}

	sess, err := ort.NewDynamicAdvancedSession(modelPath, inputNames, outputNames, nil)
	if err != nil {
		return nil, fmt.Errorf("model %s: %w", modelPath, err)
	}
	return &Session{
		sess:        sess,
		inputNames:  inputNames,
		outputNames: outputNames,
		labelCount:  labelDimension(outputs[0].Dimensions),
	}, nil
}

// labelDimension returns the last dimension of a logits tensor, or 0 when the
// model was exported without fixed dimensions and the count is unknown.
func labelDimension(shape ort.Shape) int {
	if len(shape) == 0 {
		return 0
	}
	if last := shape[len(shape)-1]; last > 0 {
		return int(last)
	}
	return 0
}

// InputNames returns the model's input names, in the model's order.
func (s *Session) InputNames() []string { return s.inputNames }

// LabelCount returns the model's output label dimension, or 0 when unknown.
func (s *Session) LabelCount() int { return s.labelCount }

// Close releases the session.
func (s *Session) Close() error { return s.sess.Destroy() }

// Run executes one inference. Each feed is a single-segment int64 input of
// shape [1, sequence]; the returned rows are [sequence][labels].
//
// The binding has no cancellation, so the context is checked before the call and
// again after it. A cancelled request therefore stops the loop that calls Run,
// and an inference already inside the runtime finishes rather than leaking.
func (s *Session) Run(ctx context.Context, feeds map[string][]int64) ([][]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	seq := -1
	inputs := make([]ort.Value, 0, len(s.inputNames))
	defer func() { destroyValues(inputs) }()
	for _, name := range s.inputNames {
		ids, ok := feeds[name]
		if !ok {
			return nil, fmt.Errorf("onnx: no feed for input %q", name)
		}
		if len(ids) == 0 {
			return nil, fmt.Errorf("onnx: input %q is empty", name)
		}
		if seq < 0 {
			seq = len(ids)
		} else if len(ids) != seq {
			return nil, fmt.Errorf("onnx: input %q has %d values, want %d like the first input", name, len(ids), seq)
		}
		t, err := ort.NewTensor(ort.NewShape(1, int64(len(ids))), ids)
		if err != nil {
			return nil, fmt.Errorf("onnx: input %q: %w", name, err)
		}
		inputs = append(inputs, t)
	}

	outputs := make([]ort.Value, len(s.outputNames))
	defer func() { destroyValues(outputs) }()
	if err := s.sess.Run(inputs, outputs); err != nil {
		return nil, fmt.Errorf("onnx: run: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	t, ok := outputs[0].(*ort.Tensor[float32])
	if !ok {
		return nil, fmt.Errorf("onnx: output is %T, want a float32 tensor", outputs[0])
	}
	// Copy: the tensor owns its buffer and is destroyed on return. The copy is
	// the one per-chunk allocation of the logits and cannot be avoided here.
	data := append([]float32(nil), t.GetData()...)
	shape := t.GetShape()

	var rows, labels int
	switch len(shape) {
	case 2:
		rows, labels = int(shape[0]), int(shape[1])
	case 3:
		if shape[0] != 1 {
			return nil, fmt.Errorf("onnx: output batch dimension is %d, want 1", shape[0])
		}
		rows, labels = int(shape[1]), int(shape[2])
	default:
		return nil, fmt.Errorf("onnx: output has %d dimensions, want 2 ([seq,labels]) or 3 ([1,seq,labels])", len(shape))
	}
	if rows != seq {
		return nil, fmt.Errorf("onnx: output has %d rows, want %d like the input", rows, seq)
	}
	if labels <= 0 || rows*labels != len(data) {
		return nil, fmt.Errorf("onnx: output shape %v does not match %d values", shape, len(data))
	}

	out := make([][]float32, rows)
	for i := range out {
		out[i] = data[i*labels : (i+1)*labels]
	}
	return out, nil
}
