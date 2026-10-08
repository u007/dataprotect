# piiscrub — Go PII Detection & Anonymization Library

## Spec v1

**Location:** `~/www/dataprotect`
**Language:** Go 1.24+
**License:** MIT
**Target RAM:** 100-200 MB (vs Presidio's 4 GB)

---

## Rationale & Non-Goals

**Why build a Go version when Presidio exists?**
Presidio is Python-only, requires 4 GB RAM with spaCy models loaded, and runs as a sidecar service. For Go projects that need inline PII scrubbing (before LLM calls, in data pipelines, in log sanitization), a native Go library is 100x lighter, compiles into any binary, and has zero operational overhead.

**Non-goals (v1):**
- No multi-language support (English only)
- No cloud API — everything runs in-process
- No formal privacy guarantees (k-anonymity, differential privacy) — that's ARX's domain
- No GUI — library + optional REST API only

---

## MVP Scope (v1)

Core interfaces + regex recognizers + checksum recognizers + 4 operators.

### In scope:
- Core interfaces: `Recognizer`, `Operator`, `Analyzer`, `Anonymizer`
- **Regex recognizers** — email, phone, credit card, SSN, IP, URL, IBAN, bitcoin
- **Checksum recognizers** — Luhn (credit cards), IBAN mod-97
- **Operators** — replace, mask, hash, redact
- **Context-aware confidence boosting** — opt-in via `analyzer.WithContextBoost()`, applied
  before the confidence filter and before overlap resolution
- **Structured data** — CSV batch processing
- **Golden dataset** — 18-case hand-labelled seed set for CI, plus an
  on-demand corpus measurement over the fetched corpora (`make corpus`)

### Later (v2+):
- NLP recognizer (prose-based NER for person/org/location)
- Secret recognizers (API keys, tokens, passwords, private keys, entropy)
- Format-preserving encryption (FPE)
- Image PII detection (facial recognition)
- REST API sidecar
- Multi-language support

---

## Project Structure (MVP)

```
dataprotect/
├── go.mod
├── go.sum
├── AGENTS.md
├── CLAUDE.md
├── README.md
├── TODO.md
├── .gitignore
├── .env.example
├── piiscrub.go              # Main package entry point
├── analyzer/
│   ├── analyzer.go          # Analyzer engine, overlap resolution
│   ├── analyzer_test.go
│   ├── overlap_test.go      # card-vs-phone, IBAN collisions
│   ├── context_boost_test.go
│   └── property_test.go     # FuzzAnalyzeInvariants + invariants
├── recognizers/
│   ├── recognizers.go       # Entity, EntityType, Recognizer interface
│   ├── regex/
│   │   ├── regex.go         # all eight regex recognizers
│   │   └── regex_test.go
│   └── checksum/
│       ├── checksum.go      # Luhn + mod-97 IBAN validators
│       └── checksum_test.go
├── anonymizer/
│   ├── anonymizer.go
│   ├── anonymizer_test.go
│   └── property_test.go     # FuzzAnonymizeInvariants + splice oracle
├── operators/
│   ├── operators.go         # replace, mask, hash, redact
│   ├── operators_test.go
│   └── property_test.go     # FuzzOperatorReplace + invariants
├── structured/
│   ├── batch.go             # CSV/JSON batch processing
│   └── batch_test.go
├── internal/context/
│   └── context.go           # confidence boosting (+ context_test.go)
├── golden/
│   ├── golden.go            # scoring runner for hand-labelled cases
│   ├── golden_test.go
│   └── accuracy_test.go     # CI gate over testdata/golden
├── testdata/
│   └── golden/
│       └── seed.json        # 18 hand-labelled seed cases
└── scripts/
    └── benchmark.sh         # scores testdata/golden, prints the report
```

This tree lists files that exist. The previous version described a
per-recognizer file layout (`email.go`, `phone.go`, …) plus an
`analyzer/entity.go` and `analyzer/recognizer.go` split that were never
created — all eight regex recognizers live in `recognizers/regex/regex.go`, and
the entity types are in `recognizers/recognizers.go`.

---

## Core Interfaces

```go
package recognizers

type EntityType string

const (
    EntityEmail       EntityType = "EMAIL"
    EntityPhone       EntityType = "PHONE"
    EntityCreditCard  EntityType = "CREDIT_CARD"
    EntitySSN         EntityType = "SSN"
    EntityIP          EntityType = "IP_ADDRESS"
    EntityURL         EntityType = "URL"
    EntityIBAN        EntityType = "IBAN"
    EntityBitcoin     EntityType = "BITCOIN"
)

type Entity struct {
    Type       EntityType
    Start      int
    End        int
    Text       string
    Confidence float64
}

type Recognizer interface {
    Name() string
    Recognize(ctx context.Context, text string) ([]Entity, error)
}
```

```go
package operators

// Operator transforms a single detected entity's text.
//
// Operators work one value at a time and the caller owns splicing: a
// replacement changes the text's length, which invalidates the byte offsets of
// every entity not yet rewritten. Anonymizer therefore rewrites right-to-left so
// each span is still valid when reached, which also makes the result independent
// of map iteration order.
type Operator interface {
    Name() string
    Replace(ctx context.Context, value string) (string, error)
}
```

```go
package analyzer

type Analyzer struct {
    recognizers []recognizers.Recognizer
    minConfidence float64
}

func New(opts ...Option) *Analyzer
func (a *Analyzer) AddRecognizer(r recognizers.Recognizer)
func (a *Analyzer) Analyze(ctx context.Context, text string) ([]recognizers.Entity, error)
```

```go
package anonymizer

type Anonymizer struct {
    operators map[recognizers.EntityType]operators.Operator
    defaultOp operators.Operator
}

func New(opts ...Option) *Anonymizer
func (a *Anonymizer) SetOperator(entityType recognizers.EntityType, op operators.Operator)
func (a *Anonymizer) Anonymize(ctx context.Context, text string, entities []recognizers.Entity) (string, error)
```

`Anonymize` requires non-overlapping, in-bounds entities. An overlapping or
out-of-bounds span is rejected with an error, and on any error the returned
string is empty — never a partially spliced document. The `Analyzer` already
resolves overlaps before returning, so the normal pipeline never triggers this;
the check guards direct callers.

---

## Recognizers (MVP)

| Recognizer | Entity Type | Method | Target | Measured (18-case seed) |
|------------|-------------|--------|--------|--------------------------|
| Email | `EMAIL` | Regex (RFC 5322 simplified) | >99% P&R | P=1.0000 R=1.0000 |
| Phone | `PHONE` | Regex (US/international/E.164) | >98% P, >97% R | P=1.0000 R=1.0000 |
| Credit Card | `CREDIT_CARD` | Luhn checksum | >99% P&R | P=1.0000 R=1.0000 |
| SSN | `SSN` | Regex (XXX-XX-XXXX) | >99% P&R | P=1.0000 R=1.0000 |
| IP Address | `IP_ADDRESS` | Regex (IPv4) | >99% P&R | P=1.0000 R=1.0000 |
| URL | `URL` | Regex (HTTP/HTTPS) | >99% P&R | P=1.0000 R=1.0000 |
| IBAN | `IBAN` | Regex + mod-97 | >99% P&R | no seed cases |
| Bitcoin | `BITCOIN` | Regex (P2PKH, P2SH, Bech32) | >99% P&R | no seed cases |

**"Target" is a goal for a ~500-case dataset; "Measured" is what
`scripts/benchmark.sh` reports today over the 18-case seed set.** Every measured
cell is now 1.0, but that is a property of this small seed set and is NOT
evidence that the targets are met: the shortfall is the dataset, not the
pipeline. The Luhn and mod-97 validators are wired into the default pipeline.
The measured column covers EMAIL, PHONE, CREDIT_CARD, SSN, IP_ADDRESS and URL
only — IBAN and BITCOIN have no seed cases, so their cells read "no seed cases"
rather than a number, because an absent measurement is not a pass.

An earlier version of this table presented ">99% P&R" as though it described the
recognizers. It described an aspiration for a dataset that did not exist. Only
the measured column is evidence.

Note the aggregate is not an average of this table, and the per-type cells are
coarse: `PHONE` rests on two cases and `EMAIL` on five, so a single flipped case
moves either a long way. Aggregate exact precision/recall over the whole seed set
is P=1.0000 R=1.0000 (tp=13 fp=0 fn=0), and overlap matching — which tolerates
mis-sized spans — gives the same P=1.0000 R=1.0000. Before the parenthesised-phone
fix the exact aggregate was P=0.9231 R=0.9231 while overlap stayed at 1.0000; the
gap between the two granularities is exactly what localised the mis-sized span,
which is why both are reported.

---

## Corpus measurement (2026-10-08)

The seed set above is hand-written and small. `make corpus` scores the same
detector against the two fetched corpora, which is the measurement the ">99%"
target was always meant to describe. Both numbers below come from the **same
scorer** as the seed gate — only the dataset and the schema filter differ.

Reproduce with `make corpus` (default `LIMIT=2000` records per corpus, in file
order, recorded in each `schema.json`).

| | nemotron (2000 of 100000) | gretel (2000 of 5596) |
|---|---|---|
| **Exact** | P=0.5994 R=0.6935 F1=0.6430 (tp=1887 fp=1261 fn=834) | P=0.2821 R=0.7673 F1=0.4126 (tp=610 fp=1552 fn=185) |
| **Overlap** | P=0.7541 R=0.8725 F1=0.8090 (tp=2374 fp=774 fn=347) | P=0.2969 R=0.8075 F1=0.4342 (tp=642 fp=1520 fn=153) |
| negatives clean | 412 of 600 | 1171 of 1497 |
| out-of-schema (not scored) | IBAN=1 | BITCOIN=44, URL=817 |

Per type (exact):

| Type | nemotron P / R | gretel P / R |
|------|----------------|--------------|
| `EMAIL` | 0.9975 / 0.9975 | 0.3281 / 0.8060 |
| `PHONE` | 0.5703 / 0.9982 | 0.2775 / 0.8179 |
| `CREDIT_CARD` | 0.5714 / 0.0945 | 0.5094 / 0.4737 |
| `SSN` | 0.3495 / 1.0000 | 0.0929 / 0.8846 |
| `IP_ADDRESS` | 1.0000 / 0.6400 | 0.9545 / 0.4884 |
| `URL` | 0.3053 / 0.2846 | *no label in corpus* |
| `IBAN` | *no label in corpus* | 0.9556 / 0.9773 |
| `BITCOIN` | *no label in corpus* | *no label in corpus* |

**These numbers are far below the ">99%" target, and every large gap has been
explained rather than assumed away:**

- **SSN on gretel (P=0.0929, 449 FP)** — `regex.go` uses
  `\b\d{3}[\s.\-]?\d{2}[\s.\-]?\d{4}\b`, where every separator is
  *optional*, so any bare nine-digit run matches: `CAS No.: 123456789`,
  `Account Number: 123456789`, `Passport Number: 322996077`. This is a real
  detector defect that the 18-case seed set cannot expose (its single SSN is
  dashed). Recorded as a TODO, not fixed here.
- **CREDIT_CARD on nemotron (R=0.0945)** — all 230 false negatives fail the
  Luhn check, and exactly the 24 Luhn-valid ones are the true positives. The
  validator is working as designed; the corpus's synthetic card numbers simply
  are not card numbers. Whether that is the right benchmark target is an open
  question, not a defect.
- **gretel EMAIL (P=0.3281)** — corpus-side, not detector-side. gretel labels
  the local part as `user_name` in some records (`lowe73@email.com` carries
  only `user_name 90..96 = 'lowe73'`), markdown `[addr](mailto:addr)` renders
  each address twice while the corpus labels one, and in records that carry any
  email label 106 of 647 email-shaped occurrences were labelled `email` at all.
  This is exactly the corpus-quality risk the spec anticipated: a corpus miss
  appears as our false positive.
- **nemotron URL (P=0.3053, R=0.2846)** — all 701 detections overlap a gold
  URL (verified), but only 214 match it exactly, so the loss is **boundaries**,
  not missed detections. The exact/overlap gap is what localises it, which is
  why both granularities are reported.

**Coverage the corpora cannot give:** BITCOIN is labelled by neither corpus, so
it remains unmeasured; URL is measurable only on nemotron and IBAN only on
gretel, because each corpus labels only one of them.

A converted dataset declares its own scope in `schema.json`. `scripts/benchmark.sh`
applies it, so detections of a type a corpus never labels are reported as
out-of-schema instead of inflating false positives.

---

## Operators (MVP)

| Operator | Behavior | Example |
|----------|----------|---------|
| `replace` | Substitute with fixed value | `john@example.com` → `<EMAIL>` |
| `mask` | Partial masking | `4111111111111111` → `****-****-****-1111` |
| `hash` | Deterministic SHA-256 | `john@example.com` → `a3f5c8...` |
| `redact` | Remove entirely | `john@example.com` → `[REDACTED]` |

`mask` is rune-based: one Unicode character is one slot, so a multi-byte rune is
never split — `é` with `#*` yields `é*`, not the invalid UTF-8 a byte-wise mask
produced. `#` keeps a slot, `*` masks it, any other pattern character is a
separator emitted verbatim, and an empty pattern masks every character. Input
that is not valid UTF-8 is kept byte-faithfully on a `#` slot.

---

## Structured Data (CSV)

```go
package structured

// ProcessCSV returns the number of rows whose field count differed from the
// header. Such rows are padded with empty trailing cells rather than rejected,
// because ragged CSV is common in real exports and failing the whole file over
// one truncated row is worse than writing an empty cell — but the count lets a
// caller detect that it happened instead of silently trusting the output.
func (bp *BatchProcessor) ProcessCSV(ctx context.Context, r io.Reader, w io.Writer) (int, error)
```

---

## Dependencies (Pinned)

| Dependency | Purpose | Version |
|------------|---------|---------|
| [prose](https://github.com/jdkato/prose) | NLP NER (v2+) | v2.0.0 |
| [go-face](https://github.com/Kagami/go-face) | Facial recognition (v2+) | v0.0.0-20240518085811-770585e |
| [fpe](https://github.com/capitalone/fpe) | Format-preserving encryption (v2+) | v1.0.0 |

---

## Testing Strategy

### Golden Dataset
- ~500 hand-labeled samples — **18 exist** in `testdata/golden/seed.json`; the
  remaining ~482 do not
- CI regression testing on every commit: `golden/accuracy_test.go` gates on
  precision/recall thresholds and fails if the dataset drops below 12 cases
- Precision/recall/F1 per entity type, plus aggregate, under two matching
  policies (exact span and byte overlap)

Run `scripts/benchmark.sh` to print the current numbers. Thresholds in
`golden/accuracy_test.go` are set to the measured baseline and must be raised
deliberately as the dataset grows — they are not a target. The gate is
mutation-verified: removing the `\b` anchors from the phone recognizer (the
historical card-vs-phone defect) fails it.

Labels are hand-written from the text, never copied from detector output.
A dataset derived from the detector's own predictions cannot fail and therefore
proves nothing.

### Benchmark Datasets (v2+)
| Dataset | Source |
|---------|--------|
| OpenNER 1.0 (English) | arXiv |
| Financial PII | Mendeley |
| Kaggle PII Detection | Kaggle |
| CoNLL 2025 NER | Hugging Face |
| ai4privacy/pii-masking-43k | Hugging Face |
| PII-Bench | arXiv |
| SecretBench | arXiv |
| secrets-patterns-db | GitHub |

---

## Implementation Phases

| Phase | Scope | Status |
|-------|-------|--------|
| **1 (MVP)** | Core interfaces + regex + checksum + 4 operators + context boosting + structured + golden dataset | **SHIPPED**, except: the CI golden set is an 18-case seed (not ~500); the corpora are measured on demand instead |
| 2 | NLP recognizer (prose) | Later |
| 3 | Secret recognizers (patterns + entropy) | Later |
| 4 | FPE operator | Later |
| 5 | Image PII detection (go-face) | Later |
| 6 | REST API sidecar | Later |

---

## Success Criteria (MVP)

- [ ] All regex recognizers >99% precision/recall on golden dataset — **NOT
      MET, now measured.** The seed set still reads P=1.0000 R=1.0000 over 18
      cases, which cannot support a >99% claim. The corpora are now measurable
      and do not come close: exact aggregate P=0.5994 R=0.6935 (nemotron) and
      P=0.2821 R=0.7673 (gretel) over 2000 records each — see "Corpus
      measurement (2026-10-08)" above for the per-type numbers and the
      explanation of every large gap. BITCOIN remains unmeasured because
      neither corpus labels it. Reproduce with `make corpus`.
- [x] Luhn + IBAN checksum validation working — **MET.** Both validators are
      wired into the default pipeline. Cards are detected by Luhn alone: the
      regex card recognizer is excluded because wiring both cannot work, since
      for a Luhn-failing number the validator emits nothing while the regex
      recognizer still matches the shape. CREDIT_CARD went from P=0.7500 to
      P=1.0000 and all negative cases became clean.
- [x] 4 operators (replace, mask, hash, redact) working
- [x] Structured data CSV processing working
- [x] Full test suite passes
- [x] README with usage examples
