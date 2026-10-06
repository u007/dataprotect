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
- **Golden dataset** — ~500 hand-labeled samples for CI

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
│   └── anonymizer_test.go
├── operators/
│   ├── operators.go         # replace, mask, hash, redact
│   └── operators_test.go
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

---

## Recognizers (MVP)

| Recognizer | Entity Type | Method | Target | Measured (18-case seed) |
|------------|-------------|--------|--------|--------------------------|
| Email | `EMAIL` | Regex (RFC 5322 simplified) | >99% P&R | P=1.0000 R=1.0000 |
| Phone | `PHONE` | Regex (US/international/E.164) | >98% P, >97% R | P=0.5000 R=0.5000 |
| Credit Card | `CREDIT_CARD` | Luhn checksum | >99% P&R | P=1.0000 R=1.0000 |
| SSN | `SSN` | Regex (XXX-XX-XXXX) | >99% P&R | P=1.0000 R=1.0000 |
| IP Address | `IP_ADDRESS` | Regex (IPv4) | >99% P&R | P=1.0000 R=1.0000 |
| URL | `URL` | Regex (HTTP/HTTPS) | >99% P&R | P=1.0000 R=1.0000 |
| IBAN | `IBAN` | Regex + mod-97 | >99% P&R | no seed cases |
| Bitcoin | `BITCOIN` | Regex (P2PKH, P2SH, Bech32) | >99% P&R | no seed cases |

**"Target" is a goal for a ~500-case dataset; "Measured" is what
`scripts/benchmark.sh` reports today over the 18-case seed set.** No target is
met at the stated level, and the shortfall is now the dataset rather than the
pipeline: the one remaining defect is the phone recognizer mis-sizing
parenthesised numbers, recorded in TODO.md. The Luhn and mod-97 validators are
wired into the default pipeline. The measured column covers EMAIL, PHONE,
CREDIT_CARD, SSN, IP_ADDRESS and URL only — IBAN and BITCOIN have no seed
cases, so their cells read "no seed cases" rather than a number, because an
absent measurement is not a pass.

An earlier version of this table presented ">99% P&R" as though it described the
recognizers. It described an aspiration for a dataset that did not exist. Only
the measured column is evidence.

Note the aggregate is not an average of this table: `PHONE` scores 0.5 on two
cases, so its figure carries very little weight, while `EMAIL` scores 1.0 across
five. Aggregate exact precision/recall over the whole seed set is
P=0.9231 R=0.9231 (tp=12 fp=1 fn=1); overlap matching, which tolerates
mis-sized spans, gives P=1.0000 R=1.0000. The gap between exact and overlap
recall is the parenthesised-phone case, and reporting both is what makes that
visible.

---

## Operators (MVP)

| Operator | Behavior | Example |
|----------|----------|---------|
| `replace` | Substitute with fixed value | `john@example.com` → `<EMAIL>` |
| `mask` | Partial masking | `4111111111111111` → `****-****-****-1111` |
| `hash` | Deterministic SHA-256 | `john@example.com` → `a3f5c8...` |
| `redact` | Remove entirely | `john@example.com` → `[REDACTED]` |

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
| **1 (MVP)** | Core interfaces + regex + checksum + 4 operators + context boosting + structured + golden dataset | **SHIPPED**, except: golden dataset is an 18-case seed set (not ~500) |
| 2 | NLP recognizer (prose) | Later |
| 3 | Secret recognizers (patterns + entropy) | Later |
| 4 | FPE operator | Later |
| 5 | Image PII detection (go-face) | Later |
| 6 | REST API sidecar | Later |

---

## Success Criteria (MVP)

- [ ] All regex recognizers >99% precision/recall on golden dataset — **NOT MET.**
      Measured over 18 seed cases: exact P=0.9231 R=0.9231, per-type
      PHONE=0.5000/0.5000 and CREDIT_CARD=1.0000/1.0000. The dataset is far too
      small to support a >99% claim, and the phone recognizer still mis-sizes
      parenthesised numbers. Everything else measures 1.0.
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
