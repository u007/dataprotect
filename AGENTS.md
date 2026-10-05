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
- `recognizers/checksum/` — checksum validation (Luhn for credit cards, mod-97 for IBAN)
- `anonymizer/` — orchestrates operators, transforms detected entities
- `operators/` — anonymization operators (replace, mask, hash, redact)
- `structured/` — batch processing for CSV/JSON
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
- `removeOverlapping` breaks ties by higher confidence; on an exact tie the
  earlier span wins. That is a policy choice, not an implementation accident.

## Code Conventions

- Go 1.24+ idioms only
- All exported types/functions must have doc comments
- Tests live alongside source files (`*_test.go`)
- No external dependencies in MVP (stdlib only)
- Deterministic output — same input always produces same output

## Secrets & Credentials

- Never commit real PII or credentials to test fixtures
- Use synthetic/generated data only
- `.env.example` documents required env vars (none in MVP)

## File Structure

See SPEC.md for full architecture. MVP scope: core interfaces + regex recognizers + checksum recognizers + 4 operators + structured CSV processing.
