---
type: Decision
title: piiscrub v1 Spec — Scope, Architecture & Non-Goals
description: 'Architecture decision record for piiscrub v1: why a dependency-free Go PII library instead of Python Presidio, the explicit non-goals, MVP scope versus v2+ phases, core interfaces (Recognizer, Operator, Analyzer, Anonymizer), the actual on-disk package layout, per-recognizer accuracy targets alongside measured results from the 81-case golden seed AND from a 2000-record sample of both fetched corpora (which fall well short of the >99% target, with every large gap explained), operator semantics, the CSV batch contract, pinned dependencies, and the testing/benchmark strategy.'
tags:
  - pii
  - go
  - architecture
  - spec
  - recognizers
  - anonymization
  - privacy
  - v1
timestamp: 2026-10-07T04:56:44Z
---

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
- **National ID recognizers** — Singapore NRIC/FIN (check letter, prefixes S/T/F/G),
  Malaysia MyKad (hyphenated form, real date of birth; no checksum exists),
  Thailand citizen ID (mod-11 check digit). Malaysia passports (one letter A/H/K
  and eight digits, matched only next to a passport keyword). Singapore passports
  (E or K, seven digits, one letter, same keyword gate; shape not confirmed by PRADO).
  Thailand passports and the M prefix are not covered yet; see TODO.md.
  Thailand passports stay uncovered by decision (2026-10-15): the only sourced shape
  is the expired 2012 booklet's, which overlaps Japan's. National IDs need a word
  boundary before them, so an ID glued to a letter is not redacted (for example
  `ID3100600445635` or `xS1234567D`). The exception is an SG NRIC glued directly to
  its label (`NRICS1234567D`), which is found. This is documented scope (decided
  2026-10-15): a looser boundary would also redact order and reference numbers.
- **Operators** — replace, mask, hash, redact
- **Context-aware confidence boosting** — opt-in via `analyzer.WithContextBoost()`, applied
  before the confidence filter and before overlap resolution. Measured: no change on the
  seed set; on nemotron it lowers exact P and R slightly, because a boosted IP or card inside a
  URL displaces the URL (see TODO.md). Keep it off for URL-bearing text until that is decided.
- **Structured data** — CSV batch processing
- **Golden dataset** — 81-case hand-labelled seed set for CI, plus an
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
│       └── seed.json        # 50 hand-labelled seed cases
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

`Anonymize` requires non-overlapping, in-bounds entities whose start and end fall
on UTF-8 rune boundaries. An overlapping, out-of-bounds, or mid-rune span is
rejected with an error, and on any error the returned string is empty — never a
partially spliced document. The `Analyzer` already
resolves overlaps before returning, so the normal pipeline never triggers this;
the check guards direct callers.

An entity whose type has neither its own operator (`SetOperator`) nor a default
(`WithDefaultOperator`) is also an error, checked before any splice. It is never
returned unredacted. This is a breaking change for partial operator maps: a
caller that wants redact-all for unlisted types passes `WithDefaultOperator`.

---

## Recognizers (MVP)

| Recognizer | Entity Type | Method | Target | Measured (81-case seed) |
|------------|-------------|--------|--------|--------------------------|
| Email | `EMAIL` | Regex (RFC 5322 simplified) | >99% P&R | P=1.0000 R=1.0000 |
| Phone | `PHONE` | Regex (US/international/E.164) | >98% P, >97% R | P=1.0000 R=1.0000 |
| Credit Card | `CREDIT_CARD` | Luhn checksum | >99% P&R | P=1.0000 R=1.0000 |
| SSN | `SSN` | Regex (XXX-XX-XXXX) | >99% P&R | P=1.0000 R=1.0000 |
| IP Address | `IP_ADDRESS` | Regex (IPv4) | >99% P&R | P=0.7500 R=1.0000 (version string read as IP) |
| URL | `URL` | Regex (HTTP/HTTPS) | >99% P&R | P=1.0000 R=1.0000 |
| IBAN | `IBAN` | Regex + mod-97 | >99% P&R | P=1.0000 R=1.0000 |
| Bitcoin | `BITCOIN` | Regex (P2PKH, P2SH, Bech32) + Base58Check / bech32 checksum | >99% P&R | P=1.0000 R=1.0000 (seed cases only; neither corpus labels it) |
| SG NRIC/FIN | `SG_NRIC` | Regex + check letter (S, T, F, G) | >99% P&R | P=1.0000 R=1.0000 (M prefix unsupported) |
| Malaysia MyKad | `MY_IC` | Regex (hyphenated) + date check | >99% P&R | P=1.0000 R=1.0000 (no checksum exists) |
| Thailand citizen ID | `TH_ID` | Regex + mod-11 check digit | >99% P&R | P=1.0000 R=1.0000 (Scrubber types Luhn-valid Thai IDs as TH_ID, decision d; raw analyzers keep CREDIT_CARD) |
| Malaysia passport | `MY_PASSPORT` | Regex (A/H/K + 8 digits) next to a passport keyword | >99% P&R | P=1.0000 R=1.0000 (keyword-backed; shape from Wikipedia and Purview, not PRADO) |
| Singapore passport | `SG_PASSPORT` | Regex (E or K + 7 digits + letter) next to a passport keyword | >99% P&R | P=1.0000 R=1.0000 (keyword-backed; shape from Purview and the 2006 ICA BioPass description; K prefix and digit count not confirmed by PRADO) |

**"Target" is a goal for a ~500-case dataset; "Measured" is what
`scripts/benchmark.sh` reports today over the 81-case seed set.** The measured
cells are 1.0 except IP_ADDRESS, whose batch-2 defect is in TODO.md. None of
these cells is evidence that the targets are met: the shortfall is the dataset
and that one defect. The Luhn and mod-97 validators are wired into the default
pipeline.
The measured column covers all twelve types; the five national-ID and passport rows rest on synthetic cases only. BITCOIN is shape-then-checksum: a
shape match is reported only when its Base58Check (`1`/`3`) or bech32/bech32m
(`bc1`) checksum verifies. A shape match that fails its checksum is not an address
and is not reported. Taproot (`bc1p`, bech32m) is
matched by the same pattern and verified by the bech32m checksum.

An earlier version of this table presented ">99% P&R" as though it described the
recognizers. It described an aspiration for a dataset that did not exist. Only
the measured column is evidence.

Note the aggregate is not an average of this table, and the per-type cells are
coarse: `PHONE`, `SSN` and `EMAIL` rest on four cases each, so a single flipped case
moves any of them a long way. Aggregate exact precision/recall over the whole seed set
is P=0.9811 R=1.0000 (tp=52 fp=1 fn=0), and overlap matching — which tolerates
mis-sized spans — gives the same. Before the parenthesised-phone
fix the exact aggregate was P=0.9231 R=0.9231 while overlap stayed at 1.0000; the
gap between the two granularities is exactly what localised the mis-sized span,
which is why both are reported.

---

## Corpus measurement (2026-10-09)

The seed set above is hand-written and small. `make corpus` scores the same
detector against the two fetched corpora, which is the measurement the ">99%"
target was always meant to describe. Both numbers below come from the **same
scorer** as the seed gate — only the dataset and the schema filter differ.

Reproduce with `make corpus` (default `LIMIT=2000` records per corpus, in file
order, recorded in each `schema.json`).

| | nemotron (2000 of 100000) | gretel (2000 of 5596) |
|---|---|---|
| **Exact** | P=0.8340 R=0.8699 F1=0.8516 (tp=2367 fp=471 fn=354) | P=0.3557 R=0.7673 F1=0.4861 (tp=610 fp=1105 fn=185) |
| **Overlap** | P=0.8365 R=0.8725 F1=0.8541 (tp=2374 fp=464 fn=347) | P=0.3743 R=0.8075 F1=0.5116 (tp=642 fp=1073 fn=153) |
| negatives clean | 474 of 600 | 1296 of 1497 |
| out-of-schema (not scored) | IBAN=1 | BITCOIN=21, URL=817, MY_PASSPORT=1 |

Per type (exact):

| Type | nemotron P / R | gretel P / R |
|------|----------------|--------------|
| `EMAIL` | 0.9975 / 0.9975 | 0.3281 / 0.8060 |
| `PHONE` | 0.5703 / 0.9982 | 0.2772 / 0.8179 |
| `CREDIT_CARD` | 0.5714 / 0.0945 | 0.5094 / 0.4737 |
| `SSN` | 0.8780 / 1.0000 | 0.9787 / 0.8846 |
| `IP_ADDRESS` | 1.0000 / 0.6400 | 0.9545 / 0.4884 |
| `URL` | 0.9900 / 0.9229 | *no label in corpus* |
| `IBAN` | *no label in corpus* | 0.9556 / 0.9773 |
| `BITCOIN` | *no label in corpus* | *no label in corpus* |

**Unlabelled spans, reported separately.** A detection that lands on a span the
corpus labels with a name outside our type map is not counted as wrong in the
headline. It is set aside into a bucket, and the report shows the precision with
it removed, labelled as an upper bound. The headline stays as it was, so the
bucket can't quietly move a number. Measured on the 2000-record samples:

- nemotron, exact: headline P=0.8340; 453 detections set aside (PHONE 408, SSN 25,
  CREDIT_CARD 18, EMAIL 2); upper bound P=0.9925.
- gretel, exact: headline P=0.3557; 18 detections set aside (CREDIT_CARD on `bban`
  7, EMAIL on `name` 6 and `user_name` 4, PHONE 1); upper bound P=0.3595.

An upper bound is not accuracy. The nemotron PHONE bucket is mostly ID-field
values (`biometric_identifier`, `customer_id`, `account_number`, and others). Those
are most likely detector false positives from the 10-digit phone branch, and the
bucket can't separate them from real errors, so the PHONE bound should not be
quoted as meaningful. See TODO.md.

**These numbers are far below the ">99%" target, and every large gap has been
explained rather than assumed away:**

- **SSN (gretel P=0.0929 → 0.9787; nemotron P=0.3495 → 0.8780)** — every
  separator was *optional*, so any bare nine-digit run matched: `CAS No.:
  123456789`, `Account Number: 123456789`, `Passport Number: 322996077`. Fixed:
  both separators are now required in prose, and a bare nine-digit value is
  accepted only when it is the whole input. Recall is unchanged (gretel 0.8846,
  nemotron 1.0000). Residual, hand-checked: all 25 nemotron false positives sit at spans the
  source labels with unmapped ID types (23 `tax_id`), and gretel's 1 is
  labelled by nothing. Whether unmapped-label spans should count as false
  positives is a scope decision — see TODO.md.
- **PHONE on nemotron (P=0.5703)** — 408 of its 419 false positives sit on spans
  the source labels as an ID-type field: `biometric_identifier` 117,
  `customer_id` 83, `account_number` 79, `medical_record_number` 76,
  `health_plan_beneficiary_number` 34. These are most likely detector false
  positives: the phone pattern's bare 10-digit branch matches ID numbers. This
  is not yet confirmed by hand. An earlier draft called it scope; that was
  wrong. See TODO.md.
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
- **nemotron URL (P=0.3053 → 0.9900, R=0.2846 → 0.9229)** — all 701 detections
  overlapped a gold URL, but only 214 matched it exactly. Most of the boundary
  disagreements were a trailing `.` (406) or `,` (64), which the pattern kept.
  The URL recognizer now trims that punctuation (see TODO.md). Overlap numbers
  were already matched and are unchanged.

**Coverage the corpora cannot give:** BITCOIN is labelled by neither corpus, so
it remains unmeasured; URL is measurable only on nemotron and IBAN only on
gretel, because each corpus labels only one of them.

A converted dataset declares its own scope in `schema.json`. `scripts/benchmark.sh`
applies it, so detections of a type a corpus never labels are reported as
out-of-schema instead of inflating false positives.

---

## Presidio test cases (2026-10-15)

Presidio (microsoft/presidio, MIT) ships a test suite with per-type PII cases. Its in-scope modules are converted by `scripts/presidio_extract.py` and scored with `make presidio`. Pin: `2523c7b74a469270c5c78bb253f140eafca21e31`. Converted data lives in `testdata/external/presidio/` (gitignored). The CI gate still runs only on `testdata/golden/`. The numbers below are a measurement, not a gate, and no recognizer changed for them.

**Scope.** The ten modules whose `entities` fixture maps to a type this library implements: EMAIL_ADDRESS, IP_ADDRESS, URL, IBAN_CODE, CRYPTO, CREDIT_CARD, PHONE_NUMBER, US_SSN, TH_TNIN, SG_NRIC_FIN. CRYPTO is bitcoin only: its pattern is `bc1` and `1`/`3` base58 addresses (`presidio_analyzer/predefined_recognizers/generic/crypto_recognizer.py`). Out of scope and not converted, of the 140 test modules: 76 country recognizer modules for countries this library does not cover, 24 other recognizer modules (including the NLP and model-based entities such as PERSON and LOCATION), and 30 infrastructure modules (engine, registry, configuration, chunkers). Also out of scope: presidio-research's evaluation framework and generator data (the generator's identities are CC BY-SA 3.0), the context-word sentence file (no spans, so no gold), and the e2e tests, which are API smoke tests with no per-type case tables.

**Method.** Only `parametrize` lists are parsed, with `ast` (never imported or run). Presidio's character offsets are converted to UTF-8 bytes, and each value is taken from the text. Rows that cannot be scored are set aside with a reason in `skipped.json`: count-only rows (Presidio asserts how many matches, not where), leniency rows (a non-default recognizer setting), and empty-text negatives (`golden.Load` rejects empty text). Each module is scored as its own dataset, so `in_schema` is that module's type.

| Module | Cases | Negatives | Exact P / R (tp, fp, fn) | Overlap P / R |
|--------|-------|-----------|--------------------------|---------------|
| email | 8 | 2 | 0.8571 / 0.8571 (6, 1, 1) | 1.0000 / 1.0000 |
| ip | 112 | 29 | 0.4082 / 0.2273 (20, 29, 68) | 1.0000 / 0.5568 |
| url | 16 | 3 | 0.7500 / 0.6429 (9, 3, 5) | 0.9167 / 0.7857 |
| iban | 330 | 173 | 1.0000 / 0.9937 (157, 0, 1) | 1.0000 / 0.9937 |
| crypto (BITCOIN) | 9 | 3 | 1.0000 / 1.0000 (7, 0, 0) | 1.0000 / 1.0000 |
| credit_card | 19 | 5 | 0.8750 / 0.8750 (14, 2, 2) | 0.9375 / 0.9375 (15, 1, 1) |
| phone | 19 | 0 | 0.7826 / 0.6429 (18, 5, 10) | 1.0000 / 0.8214 |
| us_ssn | 28 | 12 | 0.5769 / 0.8824 (15, 11, 2) | 0.5769 / 0.8824 |
| th_tnin (TH_ID) | 39 | 26 | 0.7647 / 1.0000 (13, 4, 0) | 0.7647 / 1.0000 |
| sg_fin (SG_NRIC) | 10 | 1 | 1.0000 / 0.5000 (5, 0, 5) | 1.0000 / 0.5000 |

Totals: 590 cases from 601 rows. 11 rows set aside (3 IP count-only, 6 phone leniency, 2 empty-text). The overlap column counts a span that touches a gold span as a match, so it is an upper bound.

**Card and IBAN fail-open, fixed 2026-10-15.** A card followed by a space and digits (`4111111111111111 123`), two cards separated by a space, and an IBAN followed by an uppercase word or digit group were not redacted. The cause was candidate grouping: each run of digits joined by space, hyphen or dot was checked as one number, and a run that failed the check was dropped. The fix keeps the whole-run check. When that fails, every card-layout window of digit groups that passes Luhn is found, the search is bounded to 19 digits per start, and overlapping windows are merged so every card digit is covered (`checksum.cardSpans`). A run inside a valid IBAN is not searched. An IBAN keeps the whole candidate, or an unspaced first block, or the candidate without its final block, and only if the kept prefix has the printed shape (`checksum.canonicalIBANShape`). Guards: a single group must be bounded by spaces, and a run inside a valid IBAN is not searched. Seed batch 9 (8 positives, 2 negatives) pins the cases, and the gate is 91 cases. Exact bounds were 61/62 and 1.0 and are now 60/62 and 60/61 (see the review response below).

Measured before and after, on the same inputs:

| Set | Before | After | Note |
|-----|--------|-------|------|
| seed | 81 cases, exact P 0.9811 (tp 52, fp 1), R 1.0000 | 91 cases, exact P 0.9677 (tp 60, fp 2), R 0.9836 (tp 60, fn 1); overlap R 1.0000 | gate: `minCases` 91; exact bounds 60/62 and 60/61, re-measured (see review response) |
| nemotron, 2000 cases, exact | P 0.8340 (fp 471), R 0.8699 | P 0.8329 (fp 475), R 0.8699 | CREDIT_CARD P 0.5714 (fp 18) to 0.5217 (fp 22), R 0.0945. The four new FPs are card windows on account-number spans (unlabelled gold) |
| gretel, 2000 cases, exact | P 0.3557 (fp 1105), R 0.7673 | P 0.3557 (fp 1105), R 0.7673 | CREDIT_CARD P 0.5094 (fp 26), R 0.4737; IBAN P 0.9556 (fp 2), R 0.9773 |
| Presidio credit_card | tp 13, fp 1, fn 3 (exact R 0.8125) | tp 14, fp 2, fn 2 exact (R 0.8750); overlap 15, 1, 1 (R 0.9375) | the merged cluster covers two adjacent gold cards |
| Presidio iban | tp 155, fp 0, fn 3 | tp 157, fp 0, fn 1 | precision 1.0000 kept |

Real-text recall did not move: the corpora have no Luhn-valid CREDIT_CARD gold that was missed (0 of the misses pass Luhn on their own), so the fix gains nothing on the corpora and costs nothing there. The differential over all sets (seed, 4000 corpus texts, 590 Presidio cases) loses no detection, and no case has overlapping detections. Anonymize runs without error over the seed, the corpora and every Presidio module.

**Residuals, not fixed.** These stay as TODO items: a dash- or dot-joined CVV (`4111111111111111-123`) is not redacted; a spaced IBAN followed by two or more uppercase blocks (`GB82 WEST 1234 5698 7654 32 ALL FEES`) is not redacted; and seven card windows form inside the IBAN bodies in the Presidio IBAN module (out of schema, not scored, and none in the corpora).

**Other findings.** Every mismatch class is recorded in TODO.md under "Presidio test cases: open findings".

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
- ~500 hand-labeled samples — **81 exist** in `testdata/golden/seed.json`; the
  remaining ~450 do not
- CI regression testing on every commit: `golden/accuracy_test.go` gates on
  precision/recall thresholds and fails if the dataset drops below 81 cases (ratcheted with each batch)
- Precision/recall/F1 per entity type, plus aggregate, under two matching
  policies (exact span and byte overlap)

Run `scripts/benchmark.sh` to print the current numbers. Thresholds in
`golden/accuracy_test.go` are set to the measured baseline and must be raised
deliberately as the dataset grows — they are not a target. The gate is
mutation-verified, re-run at 50 cases (stripping branch 3's `\b` raises exact fp from
1 to 4 and the gate fails): removing the `\b` anchors from the phone recognizer (the
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
| **1 (MVP)** | Core interfaces + regex + checksum + 4 operators + context boosting + structured + golden dataset | **SHIPPED**, except: the CI golden set is an 81-case seed (not ~500); the corpora are measured on demand instead |
| 2 | NLP recognizer (prose) | Later |
| 3 | Secret recognizers (patterns + entropy) | Later |
| 4 | FPE operator | Later |
| 5 | Image PII detection (go-face) | Later |
| 6 | REST API sidecar | Later |

---

## Success Criteria (MVP)

- [ ] All regex recognizers >99% precision/recall on golden dataset — **NOT
      MET, now measured.** The seed set reads P=0.9811 R=1.0000 over 81
      cases, which cannot support a >99% claim. The corpora are now measurable
      and do not come close: exact aggregate P=0.8340 R=0.8699 (nemotron) and
      P=0.3557 R=0.7673 (gretel) over 2000 records each — see "Corpus
      measurement (2026-10-09)" above for the per-type numbers and the
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
