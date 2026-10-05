---
type: Concept
title: piiscrub — Usage & Quick Start
description: 'Entry point for using the piiscrub Go library: feature list (8 regex and 2 checksum recognizers, 4 operators), installation, two quick-start paths (manual Analyzer plus Anonymizer wiring, and the high-level Scrubber), and pointers to the architecture spec and development conventions.'
tags:
  - pii
  - go
  - quickstart
  - usage
  - anonymization
  - csv
timestamp: 2026-10-05T07:02:53Z
---
# piiscrub — Go PII Detection & Anonymization Library

A lightweight, dependency-free Go library for detecting and anonymizing PII in text and structured data. Designed for inline scrubbing before LLM calls, data pipelines, and log sanitization.

## Features (MVP)

- **8 regex recognizers** — email, phone, credit card, SSN, IP, URL, IBAN, bitcoin
- **2 checksum validators** — Luhn (credit cards), mod-97 (IBAN). **Not yet wired
  into the default pipeline**: `NewScrubber` registers only the regex
  recognizers, so card numbers are matched by shape alone with no checksum. See
  TODO.md.
- **4 operators** — replace, mask, hash, redact
- **Structured data** — CSV batch processing
- **Context-aware** — opt-in confidence boosting from surrounding words (`analyzer.WithContextBoost()`)
- **Measured, not claimed** — a hand-labelled seed dataset and a scoring runner
  gate accuracy in CI. Current baseline over 18 cases: exact-span
  P=0.8571 R=0.9231. Run `scripts/benchmark.sh`. The seed set is far from the
  ~500 cases the accuracy targets assume, so treat the numbers as a floor.
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

See [spec-v1](../decisions/spec-v1.md) for full architecture and [development-conventions](development-conventions.md) for development conventions.

## License

MIT
