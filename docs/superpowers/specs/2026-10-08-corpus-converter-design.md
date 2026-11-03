---
type: Decision
title: "Corpus converter and corpus scoring mode — design"
description: "Design for tools/corpusbench Parquet→golden conversion and in-schema corpus scoring; implemented"
resource: tools/corpusbench
tags: [corpus, benchmark, design]
timestamp: 2026-11-03T11:51:49Z
status: active
---

# Corpus Converter + Corpus Scoring Mode — Design

- **Date:** 2026-10-08
- **Status:** APPROVED 2026-10-08; IMPLEMENTED (tools/corpusbench, `make corpus`, `make corpus-lang`). Measured results live in docs/decisions/spec-v1.md.
- **Decisions taken at approval** (both reversible):
  - `fax_number → PHONE` stands as a corpus-label mapping. If it depresses
    precision the *map* is revisited (§5.5), never the recognizer.
  - `LIMIT ?= 2000` is the default sample for both corpora, recorded in
    `schema.json` (`source_rows`/`limit`/`cases`) so no report can be mistaken
    for a full-corpus number (§5.5).
- **Closes:** TODO.md "Corpus converter and scoring mode — NOT STARTED" and the
  "grow toward ~500 cases" half of the golden-dataset entry.
- **Trigger:** SPEC.md promises ">99% precision/recall" and "~500 hand-labeled
  samples for CI", but the committed seed set is 18 cases
  (`golden/accuracy_test.go`, `minCases = 12`), so those numbers cannot be
  measured today.

## 1. Problem

Both corpora are already fetched into `testdata/external/` by
`scripts/fetch-dataset.sh` at pinned revisions, and both are licence-clean
(synthetic CC-BY-4.0 / Apache-2.0, see `DATASET_LICENSES.md`). Nothing scores
them: they are Parquet in corpus-specific schemas, and `golden.Load`
(`golden/golden.go:342`) reads `*.json` case files only.

The naive converter is wrong, and TODO.md already records why: `scorePair`
(`golden/golden.go:196-212`) counts a prediction as FP unless it matches a gold
label of the **same type**, so a corpus labelling only PERSON/ORG/LOC would
score a correct EMAIL detection as a false positive. This design resolves that
trap rather than reproducing it.

## 2. Goals

1. A converter from each corpus's schema to `golden.Case`, deterministic for a
   given input and tool version.
2. A scoring mode that reads the corpus instead of `testdata/golden/` and does
   **not** inflate FP.
3. Both corpora behind one interface, each carrying its own label schema.
4. Every assumption the converter rests on measured, not assumed.
5. **No corpus-derived bytes committed.** `.gitignore` already covers
   `/testdata/external/`; all output lands there.

## 3. Non-goals

- Not a change to the committed CI gate. `golden/accuracy_test.go` keeps its
  seed-set thresholds and `minCases` floor.
- Not a commitment of corpus data.
- Not a claim of coverage the corpora cannot give. **Verified label presence:**

  | Our type | nemotron | gretel |
  |---|---|---|
  | EMAIL | yes | yes |
  | PHONE | yes (`phone_number`, `fax_number`) | yes (`phone_number`) |
  | CREDIT_CARD | yes (`credit_debit_card`) | yes (`credit_card_number`) |
  | SSN | yes | yes |
  | IP_ADDRESS | yes (`ipv4`, `ipv6`) | yes (`ipv4`, `ipv6`) |
  | URL | **yes** (`url`) | **no label exists** |
  | IBAN | **no label exists** | **yes** (`iban`) |
  | BITCOIN | no | no |

  URL precision/recall is measurable only on nemotron; IBAN only on gretel;
  **BITCOIN stays unmeasured by both.** SPEC.md must say so rather than imply
  coverage.
- Not a recognizer fix. Any defect the measurement exposes becomes a TODO entry,
  not an opportunistic change inside this work.

## 4. Decisions

### D1 — Isolated tool module (approved, fork A)

`tools/corpusbench/` carries its **own** `go.mod`.

**Why:** `AGENTS.md` §Code Conventions fixes the library at *"No external
dependencies in MVP (stdlib only)"*. parquet-go v0.32.0 pulls ~10 transitive
modules (measured by `go list -m all`: brotli, uuid, klauspost/compress,
bitpack, jsonlite, lz4, go-geom, x/sys, protobuf). Isolating the tool keeps that
rule true for every library consumer.

**Proven before committing to this design:** a nested module with
`require dataprotect v0.0.0` plus `replace dataprotect => ../..` builds against
the local `golden` package, and the root `go build ./...` / `go list ./...` do
not see `tools/` (both verified). The root module path has no domain prefix, so
`replace` is the only way the tool can import `golden` at all.

### D2 — Reuse `golden.Case` plus a `schema.json` sidecar (approved, fork A)

Converted output is ordinary `golden.Case` JSON plus a sidecar naming the
corpus's in-schema types. Scoring stays inside `golden`, so the corpus
measurement and the CI gate share one scoring implementation — two scorers would
be two places for the semantics to drift.

**Constraint found while designing:** `golden.Load` reads *every* `*.json` in
the directory (`golden.go:352`), so a sidecar named `schema.json` beside the case
files is decoded as `[]Case` and fails with `json: cannot unmarshal object into
Go value of type []golden.Case` (reproduced, not theorised). Resolved in §5.3.

### D3 — Offsets, not substrings, for converted labels

`Case.resolve()` matches each label by **substring** and **rejects a value that
occurs more than once** in the text (`golden.go:82`). Measured over the first
3000 records of each corpus, counting only in-schema labels:

| Corpus | Positive records | Records repeating a value |
|---|---|---|
| nemotron test split | 2137 | **298 (13.9%)** |
| gretel combined_test | 772 | **115 (14.9%)** |

Real documents repeat identifiers (`4929 3867 4532 1018` ×3, `949-852-4287` ×2).
Substring-only labels would discard ~14% of positives, biased toward exactly the
documents that are most realistic.

**Consequence:** converted labels carry explicit byte offsets. The seed path is
untouched — substring labels keep working, offsets are additive.

### D4 — Fail fast on malformed records; never skip

The converter **aborts** on a record it cannot convert, with an error naming the
corpus, source file, row index, and span index. There is no skip-and-continue
path and no partial output: a silently dropped record would understate every
metric while looking like a clean run.

**Note on the repository's error convention:** the codebase reports failures
with `fmt.Errorf` carrying context fields; it has no logging framework (verified
— no `log.` use outside tests), and `AGENTS.md` keeps the library stdlib-only.
"Detailed error context" therefore means structured fields in a returned error,
not a new logging dependency.

Two states are **not** malformed and are never dropped:

- a record with no in-schema label → a legitimate negative case;
- **overlapping gold labels**, which gretel has (16 pairs across 1411 positive
  records, nemotron 0/5000). Overlap is valid annotation nesting, and `scorePair`
  handles overlapping gold correctly. Counted in `schema.json`, not discarded.

### D5 — Tool interface: positional parameters, no mode flags

`corpusbench convert <corpus> <data-root> <limit>`. No `dryRun`, no `verbose`,
no boolean mode toggles. The limit is a **Makefile variable** (`LIMIT ?= 2000`),
not a flag.

**Sampling is explicit and recorded, never implicit.** nemotron's test split is
100000 rows; scoring all of it is slow and unnecessary for a first measurement.
`golden.Load`'s own contract rejects partial loads as understated metrics, so
the sample bound is written into `schema.json` (`source_rows`, `limit`,
`cases`) and is therefore never silent.

### D6 — Pinning and deterministic order

- Dataset revisions are already pinned by `scripts/fetch-dataset.sh` and recorded
  in each `PROVENANCE.txt` (nemotron `b70ffaf5ff39…`, gretel `7b844d167385…`);
  both are copied into `schema.json`.
- parquet-go is pinned to an exact version, and the tool module pins its `go`
  directive — a floating version could change parsing behaviour under a
  published number.
- `AGENTS.md` requires deterministic output. Case IDs derive from the source row
  index; case files are written as a sorted, fixed-width sequence; case order
  within a file follows first in-schema offset; the `in_schema` and
  `unmapped_labels` arrays are sorted. `golden.Load` already sorts filenames.
- Pagination does not apply: the tool is a local batch job over a fixed set of
  pinned files, and `golden.Load` already documents why a fixed dataset must be
  loaded in full.

## 5. Design

### 5.1 Files

```
tools/corpusbench/go.mod           module dataprotect/tools/corpusbench;
                                   replace dataprotect => ../..; pinned parquet-go
tools/corpusbench/main.go          entry point: convert <corpus> <data-root> <limit>
tools/corpusbench/convert/         corpus adapters → []golden.Case + Schema
tools/corpusbench/convert/nemotron.go
tools/corpusbench/convert/gretel.go
tools/corpusbench/convert/offset.go    character→byte translation
tools/corpusbench/convert/literal.go   Python-repr span parser (nemotron only)
tools/corpusbench/convert/labels.go    per-corpus label → EntityType maps
tools/corpusbench/convert/*_test.go    tests, written first (§7)

golden/golden.go                   Load skips the reserved name; LoadSchema added
golden/golden_test.go              coverage for both
golden/schema.go                   Schema type + WithInSchema option (if split out)
scripts/benchmark.sh               pass schema.json to Score when present
Makefile                           targets: corpus-convert, corpus, corpus-clean
.env.example                       document DATAPROTECT_DATA_DIR
```

No library package imports the tool. No root `go.mod` change.

### 5.2 Conversion pipeline

Per record: text + spans → validate → translate offsets to bytes → filter
through the label map → emit a `Case`.

**Character→byte offset translation is the core of the converter.** Corpus
offsets are **Python character indices**, not byte indices. Measured:

- gretel: of 6712 spans probed, **3614 (54%)** differ between byte-slice and
  char-slice, and **132** byte-slices land on an invalid UTF-8 boundary;
- worked example: span `{851,867}` char-slices to `"31 dicembre 2023"` but
  byte-slices to `"o al 31 dicembre"` — same span, wrong content;
- 102/500 gretel records are non-ASCII; 279/2102 spans start at a shifted byte
  index.

nemotron's English text is ASCII, so the defect would be invisible there — which
is exactly why it must be measured rather than reasoned about. Both corpora go
through the same translation.

**Offsets are authoritative; the span `text` field is advisory.** On nemotron the
`text` field disagreed with the offsets in ~4% of spans (case differences such as
`Black` vs `black`, and non-string values such as `text: 57`). Recorded label
text is always the translated slice, never `span.text`.

**Fail-fast validation (D4).** Each of these aborts conversion with corpus, file,
row, and span context: offsets out of text bounds; a zero-length span; a span
boundary that is not a rune boundary; text that is not valid UTF-8; empty case
text; an unparseable span literal; a mapped label with no `recognizers`
counterparty.

**nemotron `spans` is a Python repr, not JSON.** Samples contain
`{'start': 184, 'end': 201, 'text': "bachelor's degree", 'label': 'education_level'}`
— single-quoted keys, both quote styles for values, apostrophes inside strings.
`json.Unmarshal` fails on this; a small literal parser (list/dict/int/str, both
quote styles, escapes) is required. gretel's `pii_spans` is valid JSON.

### 5.3 `schema.json` sidecar

Written beside the case files; `golden.Load` skips it by reserved name (documented
and tested — a case file of that name would otherwise be silently ignored), and
`golden.LoadSchema(dir)` reads it with `DisallowUnknownFields`, matching `Load`.
`benchmark.sh` passes it to `Score` when present; seed-set behaviour is
byte-identical when absent.

Fields: `corpus`, `source` (file path), `source_sha256`, `source_revision`,
`source_rows`, `limit`, `cases`, `offset_unit` (bytes), `in_schema` (sorted
types), `unmapped_labels` (sorted corpus labels), `overlapping_pairs`,
`negatives`, `generated_by`.

### 5.4 Scoring: the global-schema filter

`Score` gains an option — `golden.WithInSchema` — rather than a parallel API:

- predictions whose type is not in the schema are **excluded from TP/FP/FN** and
  counted in a new `Report.OutOfSchema`, broken down by type;
- gold comes from the corpus and already contains only mapped types, so no gold
  filtering is needed;
- this is literally the TODO prescription: *"report out-of-schema detections
  separately, not as FP."*

Per-case schema was rejected: it would let the same detected type be credited in
one record and excluded in another, making precision non-comparable across the
dataset.

Because each corpus labels its in-schema types exhaustively when present, a
missing label genuinely is a false positive, so negatives stay meaningful:
gretel yields 4185 negatives of 5596 rows, nemotron 863 of 3000 sampled.

### 5.5 Label maps (reviewed, not inferred)

| Corpus | Corpus label | Our type |
|---|---|---|
| nemotron | `email` | EMAIL |
| nemotron | `phone_number`, `fax_number` | PHONE |
| nemotron | `credit_debit_card` | CREDIT_CARD |
| nemotron | `ssn` | SSN |
| nemotron | `ipv4`, `ipv6` | IP_ADDRESS |
| nemotron | `url` | URL |
| gretel | `email` | EMAIL |
| gretel | `phone_number` | PHONE |
| gretel | `credit_card_number` | CREDIT_CARD |
| gretel | `ssn` | SSN |
| gretel | `ipv4`, `ipv6` | IP_ADDRESS |
| gretel | `iban` | IBAN |

**Explicitly unmapped, deliberately:** `credit_card_security_code`,
`account_pin` (not card numbers), `bban` (not an IBAN), `swift_bic`,
`swift_bic_code`, `password`, `api_key`, `customer_id`, `employee_id`,
`account_number`, `bank_routing_number`, and every name/address/date type. The
complete unmapped set is recorded in `schema.json`, so the filter's scope is
auditable rather than implicit.

`fax_number → PHONE` is a judgement call recorded here: fax numbers are phone
numbers by our taxonomy. If it depresses precision, the map — not the recognizer
— is what gets revisited.

### 5.6 Interface with the masking / overlap contracts

The previous commit made masking rune-based and made `Anonymize` reject
overlapping spans (SPEC.md §Operators). Converted labels must not silently
violate either:

- **Rune boundaries.** Offsets are produced by rune-aware translation and are
  validated to land on rune boundaries, so the rune-based `mask` operator never
  sees a mid-rune span and byte-faithful `#` handling still applies. A
  non-boundary offset aborts conversion (D4).
- **Overlap.** The corpus scoring path calls `Analyze` only — it never splices
  text — so overlapping gold labels are safe for scoring. `Anonymize`'s
  overlap rejection (SPEC.md:182) is not in this path and does not gate
  conversion. Recorded as `overlapping_pairs` for visibility.

### 5.7 Tooling

- `make corpus-convert` — build the tool, convert both corpora at `LIMIT ?= 2000`.
- `make corpus` — `corpus-convert` then `scripts/benchmark.sh` over each output
  directory.
- `make corpus-clean` — delete the generated output (data stays gitignored).
- `make help` gains the three entries.
- `scripts/benchmark.sh` is otherwise unchanged; it applies `schema.json` only
  when the file is present.

## 6. Documentation (after implementation)

- `README.md` and its bundle copy `docs/concepts/piiscrub-usage.md` are kept in
  step — re-sync both, re-applying the four link rewrites documented in TODO.md
  or the bundle links dangle again.
- `SPEC.md` — replace the ">99%" column with the measured number once available;
  state that URL is nemotron-only, IBAN gretel-only, BITCOIN unmeasured.
- `AGENTS.md` — note the tool module and its isolated dependency.
- `TODO.md` — close the converter entry; add whatever the measurement exposes.
- `DATASET_LICENSES.md` — derived output lands in the gitignored
  `testdata/external/` and is never committed.
- `.env.example` — document `DATAPROTECT_DATA_DIR`, which the converter shares
  with `scripts/fetch-dataset.sh` (`testdata/external` default, so still not
  required).

## 7. Testing — written before implementation

No existing test is modified or removed. New tests in dependency order:

1. **The FP trap itself, first.** A corpus-shaped fixture whose gold labels only
   PHONE, a detector emitting EMAIL and PHONE: asserts the EMAIL lands in
   `OutOfSchema` and is **not** counted FP, and that a PHONE with no gold **is**
   FP. This is the failure TODO.md describes; it must fail against today's
   `scorePair` and pass against the filter.
2. **Character→byte translation** on a non-ASCII fixture, asserted byte-for-byte,
   including the `{851,867}` shape that differs by method.
3. **Fail-fast:** out-of-bounds span, zero-length span, mid-rune boundary, empty
   text, unparseable literal — each aborts with the row identified.
4. **`schema.json`:** `Load` skips it; `LoadSchema` rejects unknown fields; cases
   still load when the sidecar is present.
5. **Overlapping gold** scores without error and increments
   `overlapping_pairs`.
6. **Duplicate in-schema values** in one record survive (the D3 case).
7. **Python-repr parser:** apostrophes, both quote styles, escapes, non-string
   `text`.
8. **Determinism:** converting twice yields byte-identical output.

**Fixtures are generated at test time by parquet-go's writer** over hand-written
synthetic records, so no corpus bytes enter the repository — consistent with
`AGENTS.md` ("Never commit real PII or credentials to test fixtures; use
synthetic/generated data only") and `DATASET_LICENSES.md`.

**Fuzz:** `FuzzCharToByte` never panics, always lands on a rune boundary, and
round-trips on valid UTF-8.

**Mutation-verified:** reverting the offset translation, the `schema.json` skip,
the fail-fast guard, or the schema filter must each fail its intended test.

**Gate:** the CI gate is unchanged; corpus scoring runs on demand.

## 8. Risks

1. **Offsets validated only against these corpora** — mitigated by fuzzing and
   byte-level fixture assertions.
2. **`schema.json` reserved-name collision** with a hand-authored case file —
   documented, tested, and `LoadSchema` fails loudly on a case array.
3. **A corpus's own label quality is unverified.** A corpus-side miss appears as
   our false positive. Detectable: hand spot-check the first measurement's FPs
   before recording any threshold.
4. **parquet-go `LIST` column handling** (`pii_types`) — the converter reads
   `pii_spans` only, so a LIST type mismatch must not break schema inspection.
5. **Sampling bias** at `LIMIT` below the full row count — visible via the
   recorded `source_rows`/`limit`, never silent.

## 9. Deferred (recorded in TODO.md on implementation)

- BITCOIN coverage (neither corpus labels it) — needs a third corpus or a
  hand-authored seed extension.
- Per-language scoring of gretel's separate language files. (done 2026-11-02: `make corpus-lang CORPUS_LANG=<lang>`, `convert.GretelLang`)
- Corpus-scoring thresholds as a CI gate (blocked: corpus data cannot be
  committed, so CI cannot see it).
- An end-to-end anonymize check over corpus cases (would require resolving gold
  overlaps first).
