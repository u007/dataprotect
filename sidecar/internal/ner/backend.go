package ner

import (
	"context"

	"github.com/u007/dataprotect/recognizers"
)

// NERBackend is the interface implemented by NER backends.
type NERBackend interface {
	Name() string
	Recognize(ctx context.Context, text string) ([]recognizers.Entity, error)
}
