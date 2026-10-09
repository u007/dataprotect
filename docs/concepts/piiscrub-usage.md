---
type: Concept
title: piiscrub — Usage & Quick Start
description: 'Entry point for using the piiscrub Go library: feature list (8 regex, 2 checksum and 4 national-ID and passport recognizers, 4 operators), installation, two quick-start paths (manual Analyzer plus Anonymizer wiring, and the high-level Scrubber), measured accuracy over the 68-case seed and its per-entity-type coverage gaps, the fetchable external corpora and their licences, the on-demand corpus measurement (`make corpus`) with its real numbers, and pointers to the architecture spec and development conventions.'
tags:
  - pii
  - go
  - quickstart
  - usage
  - anonymization
  - csv
timestamp: 2026-10-07T04:56:44Z
---
# piiscrub — Go PII Detection & Anonymization Library

A lightweight, dependency-free Go library for detecting and anonymizing PII in text and structured data. Designed for inline scrubbing before LLM calls, data pipelines, and log sanitization.

## Features (MVP)

- **8 regex recognizers** — email, phone, SSN, IP, URL, IBAN, bitcoin, plus a
  shape-only credit-card recognizer
- **4 national ID and passport recognizers** — Singapore NRIC/FIN, Malaysia MyKad and
  Thailand citizen ID, each validated by its own rule. Malaysia passports need a passport keyword; Singapore and Thailand passports are not covered yet
- **2 checksum validators, wired into the default pipeline** — Luhn (credit
  cards), mod-97 (IBAN). Cards are detected by Luhn rather than by digit shape,
  so a long number that fails the checksum is not redacted as payment data.
- **4 operators** — replace, mask, hash, redact
- **Structured data** — CSV batch processing
- **Context-aware** — opt-in confidence boosting from surrounding words (`analyzer.WithContextBoost()`)
- **Measured, not claimed** — a hand-labelled seed dataset, an on-demand corpus
  measurement over the fetched corpora, and a scoring runner
  gate accuracy in CI. Current baseline over 68 cases: exact-span
  P=0.9778 R=1.0000, overlap P=0.9778 R=1.0000. Run `scripts/benchmark.sh`.
  The seed set is far from the ~500 cases the accuracy targets assume, so treat
  the numbers as a floor rather than a result.
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

Accuracy here is measured, not asserted. `testdata/golden/seed.json` holds 68
hand-labelled cases (43 positive, 25 negative), and `golden/accuracy_test.go`
gates on the exact measured baseline, so a regression fails the build instead of
drifting silently. Labels are written from the text, never copied from detector
output — a dataset derived from the detector's own predictions cannot fail and
proves nothing. Run `scripts/benchmark.sh` for the full report.

| Match policy | Precision | Recall | F1 | tp/fp/fn |
|---|---|---|---|---|
| exact span | 0.9778 | 1.0000 | 0.9888 | 44/1/0 |
| byte overlap | 0.9778 | 1.0000 | 0.9888 | 44/1/0 |

Coverage by entity type, counting the cases that carry each type:

| Entity type | Cases |
|---|---|
| EMAIL | 4 |
| CREDIT_CARD | 5 |
| PHONE | 4 |
| SSN | 4 |
| IP_ADDRESS | 3 |
| URL | 4 |
| IBAN | 5 |
| BITCOIN | 3 (shape-only, no checksum) |
| SG_NRIC | 4 |
| MY_IC | 2 |
| TH_ID | 2 |
| MY_PASSPORT | 3 |

The numbers are not perfect. Batch 2 exposed two detector defects. The URL one,
a sentence-final period kept, is fixed. The other, a four-part version string
read as an IP, is still open; that case stays labelled as written and is in
TODO.md. The seed set
is 68 cases, far from the ~500 the accuracy targets assume, so read every
number above as a floor rather than a result. This baseline was
reached by fixing the phone recognizer's dropped leading `(` on parenthesised
area codes — until that fix it was the single false positive and the single
false negative.

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

Both clean corpora are fetchable **and now scored**: `make corpus` converts them
and prints a report (see SPEC.md § "Corpus measurement (2026-10-09)" for the
numbers and what explains each gap). The conversion is schema-aware — each
converted dataset carries a `schema.json` naming the entity types it labels, and
the scorer excludes detections of any other type from the false-positive count
instead of counting them. Without that, a corpus labelling only PERSON/ORG/LOC
would score a correct EMAIL detection as a false positive.

| | nemotron (2000 of 100000) | gretel (2000 of 5596) |
|---|---|---|
| exact | P=0.8340 R=0.8699 | P=0.3557 R=0.7673 |
| byte overlap | P=0.8365 R=0.8725 | P=0.3743 R=0.8075 |

Those are real numbers from a 2000-record sample, not a target: the seed set
above is too small to test the detector at scale, while 2000 real records
find defects the seed set cannot reach — the SSN pattern matched any bare
nine-digit run (`Account Number: 123456789`, now fixed), and gretel labels the local part
of an address as `user_name` rather than the full email. Every large gap is
explained in SPEC.md rather than waved away.

Derived output lives in the same gitignored directory as the corpora and is
never committed, so no corpus-derived bytes reach this repository.

## Documentation

See [spec-v1](../decisions/spec-v1.md) for full architecture and [development-conventions](development-conventions.md) for development conventions.

## License

MIT — see [licence](../../LICENSE).

The benchmark corpora this project is measured against are **not** covered by
this licence and are not redistributed here. Each carries its own terms; see
[dataset licences](../../DATASET_LICENSES.md). Notably, ai4privacy is free only for
individuals, non-profits, for-profits with 3 or fewer staff, or evaluation.
