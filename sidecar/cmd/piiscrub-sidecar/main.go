package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/u007/dataprotect"
	"github.com/u007/dataprotect/sidecar/internal/ner"
	"github.com/u007/dataprotect/sidecar/internal/server"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8787", "listen address")
	backendName := flag.String("backend", "prose", "ner backend")
	modelDir := flag.String("model", "", "directory containing model.onnx and config.json (onnx backend)")
	tokenizerPath := flag.String("tokenizer", "", "path to tokenizer.json (onnx backend)")
	onnxRuntimeLib := flag.String("onnxruntime-lib", "", "path to libonnxruntime (onnx backend)")
	flag.Parse()

	var b ner.NERBackend
	switch *backendName {
	case "prose":
		b = ner.NewProseBackend()
	case "onnx":
		ob, err := ner.NewONNXBackend(ner.ONNXConfig{
			ModelDir:       *modelDir,
			TokenizerPath:  *tokenizerPath,
			ONNXRuntimeLib: *onnxRuntimeLib,
		})
		if err != nil {
			log.Fatalf("onnx backend: %v", err)
		}
		b = ob
	default:
		log.Fatalf("unknown backend: %s", *backendName)
	}

	scrubber := piiscrub.NewScrubber()

	srv := server.New(*addr, b, scrubber)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := srv.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
