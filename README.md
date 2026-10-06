# piiscrub — Go PII Detection & Anonymization Library

A lightweight, dependency-free Go library for detecting and anonymizing PII in text and structured data. Designed for inline scrubbing before LLM calls, data pipelines, and log sanitization.

## Features (MVP)

- **8 regex recognizers** — email, phone, SSN, IP, URL, IBAN, bitcoin, plus a
  shape-only credit-card recognizer
- **2 checksum validators, wired into the default pipeline** — Luhn (credit
  cards), mod-97 (IBAN). Cards are detected by Luhn rather than by digit shape,
  so a long number that fails the checksum is not redacted as payment data.
- **4 operators** — replace, mask, hash, redact
- **Structured data** — CSV batch processing
- **Context-aware** — opt-in confidence boosting from surrounding words (`analyzer.WithContextBoost()`)
- **Measured, not claimed** — a hand-labelled seed dataset and a scoring runner
  gate accuracy in CI. Current baseline over 18 cases: exact-span
  P=0.9231 R=0.9231, overlap P=1.0000 R=1.0000. Run `scripts/benchmark.sh`.
  The seed set is far from the ~500 cases the accuracy targets assume, so treat
  the numbers as a floor.
- **Zero external dependencies** — stdlib only
- **~100-200 MB RAM** — vs Presidio's 4 GB

## Installation

```bash
go get dataprotect
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    "dataprotect/analyzer"
    "dataprotect/anonymizer"
    "dataprotect/operators"
    "dataprotect/recognizers"
    "dataprotect/recognizers/regex"
)

func main() {
    ctx := context.Background()

    // Build analyzer
    a := analyzer.New(analyzer.WithMinConfidence(0.7))
    a.AddRecognizer(regex.NewEmailRecognizer())
    a.AddRecognizer(regex.NewPhoneRecognizer())
    a.AddRecognizer(regex.NewCreditCardRecognizer())

    // Build anonymizer
    an := anonymizer.New()
    an.SetOperator(recognizers.EntityEmail, operators.NewReplaceOperator("<EMAIL>"))
    an.SetOperator(recognizers.EntityPhone, operators.NewMaskOperator("***-***-####"))
    an.SetOperator(recognizers.EntityCreditCard, operators.NewMaskOperator("****-****-****-####"))

    // Analyze + anonymize
    text := "Contact John at john@example.com or 555-123-4567. Card: 4111111111111111."
    entities, _ := a.Analyze(ctx, text)
    result, _ := an.Anonymize(ctx, text, entities)
    fmt.Println(result)
    // Output: "Contact John at <EMAIL> or ***-***-4567. Card: ****-****-****-1111."
}
```

Or use the high-level `Scrubber`, which wires the default recognizers and operators:

```go
package main

import (
	"context"
	"fmt"

	"dataprotect"
)

func main() {
	s := piiscrub.NewScrubber()

	out, _ := s.Anonymize(context.Background(), "SSN 123-45-6789")
	fmt.Println(out)
	// Output: "SSN ***-**-6789"

	entities, _ := s.Analyze(context.Background(), "mail a@b.com")
	fmt.Println(entities[0].Type)
	// Output: EMAIL
}
```

## Documentation

See [SPEC.md](SPEC.md) for full architecture and [AGENTS.md](AGENTS.md) for development conventions.

## License

MIT — see [LICENSE](LICENSE).

The benchmark corpora this project is measured against are **not** covered by
this licence and are not redistributed here. Each carries its own terms; see
[DATASET_LICENSES.md](DATASET_LICENSES.md). Notably, ai4privacy is free only for
individuals, non-profits, for-profits with 3 or fewer staff, or evaluation.
