# AGENTS.md — piiscrub Project

## Project Overview

piiscrub is a Go library for PII (Personally Identifiable Information) detection and anonymization. It provides regex-based and checksum-based recognizers for common PII types (emails, phones, credit cards, SSNs, IPs, URLs, IBANs, bitcoin addresses) and operators for anonymizing detected entities (replace, mask, hash, redact).

## Commands

- `go build ./...` — build all packages
- `go test ./...` — run all tests
- `go vet ./...` — run static analysis
- `go test -v ./recognizers/...` — run recognizer tests
- `go test -v ./operators/...` — run operator tests

## Architecture

- `analyzer/` — orchestrates recognizers, returns detected entities
- `recognizers/regex/` — regex-based PII detection (email, phone, credit card, SSN, IP, URL, IBAN, bitcoin)
- `recognizers/checksum/` — checksum validation (Luhn for credit cards, mod-97 for IBAN, Base58Check and bech32/bech32m for bitcoin addresses; the regex BITCOIN recognizer calls the bitcoin validator)
- `recognizers/secret/` — API keys, tokens, passwords and PEM headers, each matched by shape and kept unless its value is uniform (one character repeated, or a unit of up to four repeated). A label-based match reports the value's span only: the key name (`api_key`, `password`) is context and never part of the span, and a JWT or JWE value (an `eyJ` header) is reported whole. A dotted tail is kept, except that one trailing dotted segment followed by whitespace is prose and is trimmed, and a password drops one sentence-final `.`, or one trailing `)`, `]` or `}` with no matching opener in the value, before whitespace or the end of text. The short labels `pass` and `pwd` need a non-letter before them, so `bypass=` and `compass:` are not passwords; `password`, `passwd` and `secret` have no such boundary.
- `anonymizer/` — orchestrates operators, transforms detected entities
- `operators/` — anonymization operators (replace, mask, hash, redact)
- `structured/` — batch processing for CSV
- `internal/context/` — context-aware confidence boosting, applied by `Analyzer` when `WithContextBoost()` is set

## Confidence & Threshold Semantics

- `WithMinConfidence(t)` KEEPS an entity when `confidence >= t`. Inclusive.
- Confidence is quantised to a 1e-9 grid by `roundConfidence` in `analyzer.go`,
  the single place it is quantised. All three inputs go through it: scores from
  `quantiseEntities` (every recognizer, including third-party ones), the boosted
  result via `applyContextBoost`, and the threshold via `WithMinConfidence`.
- **Testing float precision requires runtime variables, not literals.** Go
  evaluates untyped CONSTANT arithmetic at arbitrary precision, so `0.7 + 0.2`
  written literally in a struct field folds to exactly 0.9 and no IEEE 754
  rounding occurs. Such a test is silently vacuous — it passes with or without
  the fix. Route the values through `float64` variables. See
  `belowThresholdSum` in `analyzer/context_boost_test.go`.
- Do NOT quantise to a coarse grid. Accumulating 0.1 boosts and comparing without
  quantisation makes `0.7 + 0.2` evaluate as `0.8999999999999999`, which fails a
  `>= 0.9` test and silently drops an entity carrying exactly enough evidence.
- Context boosting is opt-in (`WithContextBoost()`) because it changes
  overlap-resolution outcomes. It is applied BEFORE the confidence filter and
  BEFORE overlap resolution, otherwise it can neither rescue a borderline entity
  nor influence which of two colliding recognizers wins.
- A boost never applies to a span that overlaps a URL which survives
  `minConfidence` (`overlapsSurvivingURL` in `analyzer.go`). Otherwise a boosted IP
  or card inside a URL can outrank the URL and leave its path and query visible.
  A URL below the threshold is dropped anyway, so it does not guard. Covered by
  `TestContextBoostDoesNotLiftSpanInsideURL` and
  `TestContextBoostIgnoresURLDroppedByThreshold`.
- `removeOverlapping` breaks ties by higher confidence; on an exact tie the
  earlier span wins. That is a policy choice, not an implementation accident.

## Code Conventions

- Go 1.24+ idioms only
- All exported types/functions must have doc comments
- Tests live alongside source files (`*_test.go`)
- No external dependencies in MVP (stdlib only) — **this applies to the root
  module**. `tools/corpusbench` is a separate Go module with its own `go.mod`
  that holds the Parquet reader (parquet-go); it imports the root module through
  `replace github.com/u007/dataprotect => ../..`, never the other way round. The root module's
  `go.mod` must never gain a `require` block.
- Deterministic output — same input always produces same output
- Conversion and other corpus tools fail fast: a record that cannot be read is
  an error naming corpus/file/row, never a silent skip

## Corpus measurement

- `make corpus` converts both corpora and scores them; `make corpus-convert`
  converts only. Both are on-demand — the corpora are gitignored, so CI cannot
  see them and the committed gate stays the seed set in `testdata/golden/`.
- `make test` runs the library suite; `make test-tools` runs the tool module's.
- A converted dataset carries `schema.json` declaring its in-schema types.
  `scripts/benchmark.sh` applies it automatically when present, so a detection
  of a type the corpus never labels is reported as out-of-schema instead of
  counted as a false positive.

## Secrets & Credentials

- Never commit real PII or credentials to test fixtures
- Use synthetic/generated data only
- `.env.example` documents required env vars (none in MVP)

## File Structure

See SPEC.md for full architecture. MVP scope: core interfaces + regex recognizers + checksum recognizers + 4 operators + structured CSV processing.
