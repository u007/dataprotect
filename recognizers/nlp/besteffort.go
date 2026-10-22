package nlp

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/u007/dataprotect/recognizers"
)

// bestEffortRecognizer wraps a Recognizer and swallows errors with
// rate-limited slog logging.
type bestEffortRecognizer struct {
	inner       recognizers.Recognizer
	logger      *slog.Logger
	logInterval time.Duration
	lastLog     *atomicTime
}

type atomicTime struct {
	v atomic.Int64
}

func NewBestEffort(inner recognizers.Recognizer) recognizers.Recognizer {
	return NewBestEffortWithLogger(inner, slog.Default())
}

func NewBestEffortWithLogger(inner recognizers.Recognizer, logger *slog.Logger) recognizers.Recognizer {
	if logger == nil {
		logger = slog.Default()
	}
	return &bestEffortRecognizer{
		inner:       inner,
		logger:      logger,
		logInterval: 10 * time.Second,
		lastLog:     &atomicTime{},
	}
}

func BestEffort(inner recognizers.Recognizer) recognizers.Recognizer {
	return NewBestEffort(inner)
}

func WrapBestEffort(inner recognizers.Recognizer) recognizers.Recognizer {
	return NewBestEffort(inner)
}

func (b *bestEffortRecognizer) Name() string { return b.inner.Name() }

func (b *bestEffortRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	ents, err := b.inner.Recognize(ctx, text)
	if err == nil {
		return ents, nil
	}
	now := time.Now().UnixNano()
	last := b.lastLog.v.Load()
	if last == 0 || now-last >= b.logInterval.Nanoseconds() {
		if b.lastLog.v.CompareAndSwap(last, now) {
			b.logger.Warn("nlp recognizer failed; dropping error (best-effort)", "err", err)
		}
	}
	return nil, nil
}
