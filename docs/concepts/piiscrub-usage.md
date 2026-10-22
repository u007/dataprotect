---
type: Concept
title: piiscrub — Usage & Quick Start
description: 'Entry point for using the piiscrub Go library: feature list (8 regex, 2 checksum, 5 national-ID and passport recognizers, 1 secret recognizer, 4 operators), installation, two quick-start paths (manual Analyzer plus Anonymizer wiring, and the high-level Scrubber), measured accuracy over the 549-case seed and its per-entity-type coverage gaps, the fetchable external corpora and their licences, the on-demand corpus measurement (`make corpus`) with its real numbers, and pointers to the architecture spec and development conventions.'
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
- **5 national ID and passport recognizers** — Singapore NRIC/FIN, Malaysia MyKad and
  Thailand citizen ID, each validated by its own rule. Malaysia and Singapore passports need a passport keyword; Thailand passports are not covered yet
- **2 checksum validators, wired into the default pipeline** — Luhn (credit
  cards), mod-97 (IBAN). Cards are detected by Luhn at 0.99, plus a
  Luhn-failing window at 0.85 when a label-style keyword ("card number",
  "credit card", "Visa:") names it and the prefix and layout are plausible, so
  a long number that fails the checksum is not redacted as payment data on
  shape alone.
- **1 secret recognizer** — API keys, bearer/JWT tokens, PEM private keys, password assignments and provider tokens with no label (AWS, GitHub, Slack, Stripe, Google, npm, SendGrid, bare JWT), with a length floor per shape and a uniform-value check (every character the same, or a short unit repeated), so placeholders such as `00000000000000000000` are not reported
- **4 operators** — replace, mask, hash, redact
- **Structured data** — CSV batch processing
- **Context-aware** — opt-in confidence boosting from surrounding words (`analyzer.WithContextBoost()`)
- **Measured, not claimed** — a hand-labelled seed dataset, an on-demand corpus
  measurement over the fetched corpora, and a scoring runner
  gate accuracy in CI. Current baseline over 549 cases: exact-span
  P=1.0000 R=0.9974, overlap P=1.0000 R=0.9974. Run `scripts/benchmark.sh`.
  The seed set is 549 cases, past the ~500 the accuracy targets assume, but it is
  hand-written, so treat the numbers as a floor rather than a result.
- **Zero external dependencies** — stdlib only
- **~100-200 MB RAM** — vs Presidio's 4 GB
- **Optional NLP sidecar** — a separate module (`sidecar/`) adds PERSON,
  ORGANIZATION and LOCATION through prose NER over HTTP. The root module stays
  stdlib-only and the recognizer is opt-in, so a caller that never starts the
  sidecar gets the same behaviour as before.

## Installation

```bash
go get github.com/u007/dataprotect
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"

    "github.com/u007/dataprotect/analyzer"
    "github.com/u007/dataprotect/anonymizer"
    "github.com/u007/dataprotect/operators"
    "github.com/u007/dataprotect/recognizers"
    "github.com/u007/dataprotect/recognizers/checksum"
    "github.com/u007/dataprotect/recognizers/regex"
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
    an := anonymizer.New(anonymizer.WithDefaultOperator(operators.NewRedactOperator()))
    an.SetOperator(recognizers.EntityEmail, operators.NewReplaceOperator("<EMAIL>"))
    an.SetOperator(recognizers.EntityPhone, operators.NewTailMaskOperator("***-***-####"))
    an.SetOperator(recognizers.EntityCreditCard, operators.NewTailMaskOperator("****-****-****-####"))

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

	"github.com/u007/dataprotect"
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

Replace each value with a numbered placeholder and keep the map to restore it:

```go
redacted, pseudonyms, err := s.Pseudonymize(context.Background(), "mail a@b.com and a@b.com")
// redacted:   "mail [EMAIL_1] and [EMAIL_1]"
// pseudonyms: map[string]string{"[EMAIL_1]": "a@b.com"}
```

The map holds the original values in plain text. Protect it as you would the input.

## Testing & dataset coverage

`testdata/golden/seed.json` holds 549
hand-labelled cases (350 positive, 199 negative), and `golden/accuracy_test.go`
gates on the exact measured baseline, so a regression fails the build instead of
drifting silently. Labels are written from the text, never copied from detector
output — a dataset derived from the detector's own predictions cannot fail and
proves nothing. Run `scripts/benchmark.sh` for the full report.

| Match policy | Precision | Recall | F1 | tp/fp/fn |
|---|---|---|---|---|
| exact span | 1.0000 | 0.9974 | 0.9987 | 381/0/1 |
| byte overlap | 1.0000 | 0.9974 | 0.9987 | 381/0/1 |

Coverage by entity type, counting the cases that carry each type:

| Entity type | Cases |
|---|---|
| EMAIL | 51 |
| CREDIT_CARD | 49 |
| PHONE | 54 |
| SSN | 32 |
| IP_ADDRESS | 44 |
| URL | 30 |
| IBAN | 35 |
| BITCOIN | 13 (Base58Check / bech32 checksum) |
| SG_NRIC | 5 |
| MY_IC | 3 |
| TH_ID | 4 |
| MY_PASSPORT | 3 |
| SG_PASSPORT | 2 |
| SECRET | 34 (API key, token, password, PEM header, provider token with no label) |

The numbers are not perfect. Batch 2 exposed two detector defects. The URL one,
a sentence-final period kept, is fixed. The other, a four-part version string
read as an IP, is still open; that case stays labelled as written and is in
TODO.md. The seed set
is 549 cases, past the ~500 the accuracy targets assume, so read every
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
and prints a report (see SPEC.md § "Corpus measurement (2026-10-19)" for the
numbers and what explains each gap). The conversion is schema-aware — each
converted dataset carries a `schema.json` naming the entity types it labels, and
the scorer excludes detections of any other type from the false-positive count
instead of counting them. Without that, a corpus labelling only PERSON/ORG/LOC
would score a correct EMAIL detection as a false positive.

| | nemotron (2000 of 100000) | gretel (2000 of 5596) |
|---|---|---|
| exact | P=0.8357 R=0.2886 | P=0.3816 R=0.0957 |
| byte overlap | P=0.8455 R=0.2919 | P=0.3986 R=0.1000 |

The recall is low because PERSON, ORGANIZATION and LOCATION became in-scope gold
on 2026-10-20 and this is the regex-only pipeline, which does not implement them.
Nemotron carries 3275 PERSON, 1988 LOCATION and 1018 ORGANIZATION false
negatives; with the NLP sidecar the same samples give exact P=0.4376 R=0.4135.

Those are real numbers from a 2000-record sample, not a target: the seed set
above is too small to test the detector at scale, while 2000 real records
find defects the seed set cannot reach — the SSN pattern matched any bare
nine-digit run (`Account Number: 123456789`, now fixed), and gretel labels the local part
of an address as `user_name` rather than the full email. Every large gap is
explained in SPEC.md rather than waved away.

Derived output lives in the same gitignored directory as the corpora and is
never committed, so no corpus-derived bytes reach this repository.

## NLP sidecar

```bash
cd sidecar && go build ./cmd/piiscrub-sidecar && ./piiscrub-sidecar
```

```go
s := piiscrub.NewScrubberWithRecognizers(nlp.New("http://127.0.0.1:8787"))
```

NLP entities score 0.80: above the default 0.70 floor and strictly below every
built-in recognizer (lowest 0.85), so they fill gaps and never displace a typed
detection. Raise the public threshold to drop NLP entirely:

```go
s := piiscrub.NewScrubber(
    piiscrub.WithRecognizer(nlp.New("http://127.0.0.1:8787")),
    piiscrub.WithMinConfidence(0.85), // drops every NLP entity (0.80)
)
```

`NewScrubber()` reads no environment variable and makes no network call, so the
golden tests stay deterministic. The sidecar URL comes from the caller, or from
`PII_SCRUB_NLP_URL` read by the caller (`nlp.FromEnv()`); that variable must be
set in the deployment environment for the sidecar to be used. A configured but
broken sidecar is an error; `nlp.New(url, nlp.WithNLPBestEffort())` is the
opt-in mode that logs the failure and keeps the pipeline running. See
[spec-v1](../decisions/spec-v1.md), "NLP Sidecar".

The sidecar's accuracy is measured. `make test-ner` starts a sidecar, scores
the detector against `testdata/golden/ner/` and stops it again: typed
P=0.8395 R=0.8718, and span-level P=0.9012 R=0.9359. The span measure ignores
the type, because all three NER types redact — it is the one that says the
PII is actually removed. The first evaluation on unseen text gave typed
P=0.5333 R=0.6154 and span P=0.8667 R=1.0000; that is the generalisation
figure, and it is recorded in the gate.

## Documentation

See [spec-v1](../decisions/spec-v1.md) for full architecture and [development-conventions](development-conventions.md) for development conventions.

## License

MIT — see [licence](../../LICENSE).

The benchmark corpora this project is measured against are **not** covered by
this licence and are not redistributed here. Each carries its own terms; see
[dataset licences](../../DATASET_LICENSES.md). Notably, ai4privacy is free only for
individuals, non-profits, for-profits with 3 or fewer staff, or evaluation.
