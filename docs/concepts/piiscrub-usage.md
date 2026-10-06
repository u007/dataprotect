---
type: Concept
title: piiscrub — Usage & Quick Start
description: 'Entry point for using the piiscrub Go library: feature list (8 regex and 2 checksum recognizers, 4 operators), installation, two quick-start paths (manual Analyzer plus Anonymizer wiring, and the high-level Scrubber), measured accuracy over the 18-case seed and its per-entity-type coverage gaps, the fetchable external corpora and their licences, and pointers to the architecture spec and development conventions.'
tags:
  - pii
  - go
  - quickstart
  - usage
  - anonymization
  - csv
timestamp: 2026-10-06T11:54:19Z
---
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
    "dataprotect/recognizers/checksum"
    "dataprotect/recognizers/regex"
)

func main() {
    ctx := context.Background()

    // Build analyzer. Cards are detected by the Luhn validator, NOT by the
    // regex card recognizer: the regex matches any 13-19 digit run on shape
    // alone, so a number failing the checksum would still be reported and
    // masked as payment data. Wiring both cannot fix it, because on an invalid
    // number only the regex entity exists and nothing overlaps it away.
    a := analyzer.New(analyzer.WithMinConfidence(0.7))
    a.AddRecognizer(regex.NewEmailRecognizer())
    a.AddRecognizer(regex.NewPhoneRecognizer())
    a.AddRecognizer(checksum.NewLuhnValidator())

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

## Testing & dataset coverage

Accuracy here is measured, not asserted. `testdata/golden/seed.json` holds 18
hand-labelled cases (12 positive, 6 negative), and `golden/accuracy_test.go`
gates on the exact measured baseline, so a regression fails the build instead of
drifting silently. Labels are written from the text, never copied from detector
output — a dataset derived from the detector's own predictions cannot fail and
proves nothing. Run `scripts/benchmark.sh` for the full report.

| Match policy | Precision | Recall | F1 | tp/fp/fn |
|---|---|---|---|---|
| exact span | 0.9231 | 0.9231 | 0.9231 | 12/1/1 |
| byte overlap | 1.0000 | 1.0000 | 1.0000 | 13/0/0 |

Coverage by entity type — 6 of the 8 recognizers have any coverage at all, and
five of those rest on a single case:

| Entity type | Cases |
|---|---|
| EMAIL | 4 |
| CREDIT_CARD | 3 |
| PHONE | 2 |
| SSN | 1 |
| IP_ADDRESS | 1 |
| URL | 1 |
| IBAN | **0 — untested** |
| BITCOIN | **0 — untested** |

The one non-perfect number is a known defect: the phone recognizer drops a
leading `(` on parenthesised area codes, which is both the only false positive
and the only false negative. IBAN and bitcoin are entirely unmeasured, and 18
cases is far from the ~500 the accuracy targets assume, so read every number
above as a floor rather than a result.

### External corpora

`scripts/fetch-dataset.sh` fetches larger benchmarks for local scoring (`list`,
`info <name>`, `fetch <name>`, `fetch-all`). Nothing it downloads is committed —
every corpus lands in a gitignored directory, which is what keeps dataset terms
from reaching this source code. Per-corpus licences and obligations are recorded
in [dataset licences](../../DATASET_LICENSES.md); read them before use, as several
restrict commercial use.

| Corpus | Licence | Commercial use |
|---|---|---|
| `nemotron-pii` | CC-BY-4.0 | yes |
| `gretel-pii-finance` | Apache-2.0 | yes |
| `conll2003` | other (Reuters RCV1) | restricted |
| `pii-masking-43k` | custom dual-tier | restricted (≤3 staff) |
| `piibench` | apache *declared* | audit first |

These are fetchable but **not yet scored** — the per-corpus schema converter and
corpus scoring mode are still outstanding. A naive conversion would overstate
false positives, because the scorer counts any detection that does not match a
label of the same entity type as a false positive, and most corpora label fewer
types than piiscrub detects.

## Documentation

See [spec-v1](../decisions/spec-v1.md) for full architecture and [development-conventions](development-conventions.md) for development conventions.

## License

MIT — see [licence](../../LICENSE).

The benchmark corpora this project is measured against are **not** covered by
this licence and are not redistributed here. Each carries its own terms; see
[dataset licences](../../DATASET_LICENSES.md). Notably, ai4privacy is free only for
individuals, non-profits, for-profits with 3 or fewer staff, or evaluation.
