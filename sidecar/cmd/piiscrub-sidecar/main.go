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
	flag.Parse()

	var b ner.NERBackend
	switch *backendName {
	case "prose":
		b = ner.NewProseBackend()
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
