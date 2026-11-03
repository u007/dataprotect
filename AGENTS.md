# AGENTS.md — piiscrub Project

## Project Overview

piiscrub is a Go library for PII (Personally Identifiable Information) detection and anonymization. It provides regex-based and checksum-based recognizers for common PII types (emails, phones, credit cards, SSNs, IPs, URLs, IBANs, bitcoin addresses) and operators for anonymizing detected entities (replace, mask, hash, redact).

## Commands

- `go build ./...` — build all packages
- `go test ./...` — run all tests
- `go vet ./...` — run static analysis
- `go test -v ./recognizers/...` — run recognizer tests
- `go test -v ./operators/...` — run operator tests
- `make fetch-libtokenizers` — download the pinned `libtokenizers.a` that the `onnx` tag links (linux/x86_64 only; re-verifies the archive SHA-256 on every run)
- `make onnx-check` — compile gate for `-tags onnx` (no model or ONNX Runtime needed)
- `.github/workflows/onnx-check.yml` — CI job that runs `make fetch-libtokenizers` then `make onnx-check` on pushes to `main` and on pull requests
- `make test-sidecar-onnx` — sidecar tests with `-tags onnx`

## Architecture

- `analyzer/` — orchestrates recognizers, returns detected entities
- `recognizers/regex/` — regex-based PII detection (email, phone, credit card, SSN, IP, URL, IBAN, bitcoin)
- `recognizers/checksum/` — checksum validation (Luhn for credit cards, mod-97 for IBAN, Base58Check and bech32/bech32m for bitcoin addresses; the regex BITCOIN recognizer calls the bitcoin validator)
- `recognizers/secret/` — API keys, tokens, passwords and PEM headers, each matched by shape and kept unless its value is uniform (one character repeated, or a unit of up to four repeated). A label-based match reports the value's span only: the key name (`api_key`, `password`) is context and never part of the span, and a JWT or JWE value (an `eyJ` header) is reported whole. A dotted tail is kept, except that one trailing dotted segment followed by whitespace is prose and is trimmed, and a password drops one sentence-final `.`, or one trailing `)`, `]` or `}` with no matching opener in the value, before whitespace or the end of text. The short labels `pass` and `pwd` need a non-letter before them, so `bypass=` and `compass:` are not passwords; `password`, `passwd` and `secret` have no such boundary. A provider token with no label (`bareShape`: AWS `AKIA`/`ASIA`, GitHub, Slack `xoxb-`/`xoxp-`, Stripe live, Google `AIza`, npm, SendGrid, a bare JWT) is matched by its fixed prefix and shape and reported whole, with the uniform check run on its body; it is dropped when it overlaps a labelled span, so a token in an assignment is reported once. The formats and their sources are in SOURCES.md.
- `anonymizer/` — orchestrates operators, transforms detected entities
- `operators/` — anonymization operators (replace, mask, hash, redact)
- `structured/` — batch processing for CSV
- `internal/context/` — context-aware confidence boosting, applied by `Analyzer` when `WithContextBoost()` is set
- `recognizers/nlp/` — HTTP client recognizer delegating NER to the sidecar (root stays stdlib-only)
- `sidecar/` — separate Go module: standalone HTTP service embedding prose NER (`cmd/piiscrub-sidecar`). The prose backend decodes prose's TOKEN labels, not its entity output, because prose's coalescer mislabels company names and surnames. On SIGTERM the HTTP server drains, then the backend's `Close` releases the ONNX session and tokenizer when the backend implements `io.Closer`.
- The ONNX backend is opt-in at build time: `-tags onnx` adds `onnx_backend.go`, `hf_tokenizer.go` and `onnxrt/`, while a plain build keeps only prose plus the `!onnx` stub that names the tag. `github.com/daulet/tokenizers` links a static `libtokenizers.a` through cgo, so the `onnx` tag needs that library at LINK time — the Go module cache alone is not enough. `make fetch-libtokenizers` downloads the pinned v1.27.0 linux/x86_64 archive, verifies its SHA-256 on every run and unpacks it atomically into the gitignored `.cache/tokenizers`; any other host fails up front. `github.com/yalue/onnxruntime_go` needs no library at build time and loads `libonnxruntime` at runtime from `-onnxruntime-lib`. `NER_ONNX_MODEL` (for `make corpus-nlp NER_BACKEND=onnx`) is a DIRECTORY holding `model.onnx` and `config.json`. Requests are tokenised once and chunked at 400 tokens with a 64-token overlap; a document over 8000 tokens fails the request with an error and is never truncated. Identical spans are deduplicated across chunks, remaining overlaps are resolved pairwise longest-first, and every ONNX entity scores 0.80. The token adapters' parity fixture is `sidecar/internal/ner/testdata/tokenizer_parity.json`.

## Confidence & Threshold Semantics

- `WithMinConfidence(t)` KEEPS an entity when `confidence >= t`. Inclusive.
- The floor is configurable at both layers: `analyzer.WithMinConfidence(t)` at
  `Analyzer` construction, and `piiscrub.WithMinConfidence(t)` as a
  `ScrubberOption` (which overrides the `newScrubber` default of 0.70). Both
  route through `Analyzer.SetMinConfidence`, which is construction-time only;
  calling it concurrently with `Analyze` is a data race.
- The NLP score is 0.80: strictly above the 0.70 default floor and strictly
  below the lowest built-in (0.85, the keyword-gated national rules), so NLP
  fills gaps and never displaces a typed detection. `WithMinConfidence(0.85)`
  on a `Scrubber` drops every NLP entity; that is the conservative mode.
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
- `removeOverlapping` consults `winsOverlap` before confidence. The one
  type-pair rule is that CREDIT_CARD beats PHONE on overlap at every score; every
  other pair breaks by higher confidence, and on an exact tie the earlier span
  wins. Both are policy choices, not implementation accidents. The card rule
  exists because a card window needs 13+ digits behind a valid network prefix, so
  a colliding phone match is a sub-run of digits the card already covers — the
  phone winning would leak the card's leading group (see SPEC.md, "Corpus
  measurement").

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
- `make test-race` runs the library suite under the race detector. It is a
  correctness gate, not a speed gate: the bounded-time tests scale their
  wall-clock budget 8x when the detector is on (`boundedBudget`, `race_test.go`),
  because instrumentation multiplies the same work by roughly 5x to 11x. The
  inputs are left at full size so a superlinear regression still cannot fit.
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
