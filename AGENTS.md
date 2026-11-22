# AGENTS.md — piiscrub Project

## Project Overview

piiscrub is a Go library for PII (Personally Identifiable Information) detection and anonymization. It provides regex-based and checksum-based recognizers for common PII types (emails, phones, credit cards, SSNs, IPs, URLs, IBANs, bitcoin addresses) and operators for anonymizing detected entities (replace, mask, hash, redact).

## Commands

- `go build ./...` — build all packages
- `go test ./...` — run all tests
- `go vet ./...` — run static analysis
- `go test -v ./recognizers/...` — run recognizer tests
- `go test -v ./operators/...` — run operator tests
- `make fetch-libtokenizers` — download the pinned `libtokenizers.a` that the `onnx` tag links (picks the asset by host: linux x86_64/arm64, darwin arm64/x86_64, with only linux/x86_64 tested; re-verifies the archive SHA-256 on every run)
- `make onnx-check` — compile gate for `-tags onnx` (no model or ONNX Runtime needed)
- `.github/workflows/onnx-check.yml` — CI job that runs `make fetch-libtokenizers` then `make onnx-check` on pushes to `main` and on pull requests
- `make test-sidecar-onnx` — sidecar tests with `-tags onnx`
- `make install` — download Go module dependencies for all three workspaces (root, sidecar, tools/corpusbench); populates the module cache so offline builds/tests work. Go-modules only — the ONNX static library (libtokenizers.a) is NOT included.
- `make install-all` — `make install` plus `fetch-libtokenizers` (downloads the pinned libtokenizers.a archive and verifies its SHA-256).

## Architecture

- `analyzer/` — orchestrates recognizers, returns detected entities
- `recognizers/regex/` — regex-based PII detection (email, phone, credit card, SSN, IP, URL, IBAN, bitcoin)
- `recognizers/checksum/` — checksum validation (Luhn for credit cards, mod-97 for IBAN, Base58Check and bech32/bech32m for bitcoin addresses; the regex BITCOIN recognizer calls the bitcoin validator). `Mod97Valid` is the exported ISO 7064 MOD 97-10 check with no rearrangement, shared by IBAN and LEI. `keccak.go` is the original Keccak-256 (padding 0x01, not SHA3-256), which `ValidEthereumAddress` (EIP-55) needs; `crypto/sha3` has no legacy Keccak.
- `recognizers/finance/` — keyword-gated banking identifiers with no checksum or no universal shape: `SWIFT_BIC`, `US_ROUTING` (ABA checksum), `CARD_CVV`, `BANK_ACCOUNT`, `PIN`. Each reports the value only, never the label, and scores 0.85. Shapes and the corpus evidence are in SOURCES.md. Also the checksum identifiers `LEI` (ISO 7064 MOD 97-10, no keyword, 0.90), `ISIN` (keyword plus Luhn, 0.85) and `ETHEREUM` (EIP-55 checksum, 0.95, the value is the address).
- `recognizers/finance/` also holds `MONEY` (0.85): a currency symbol (`$`, `US$`, `S$`, `RM`, `€`, `£`, `¥`) or an ISO 4217 code (16 codes, uppercase, whole word) next to an amount, or an amount then a code. A bare number is never MONEY. The amount takes thousands commas, a dot decimal or a one-or-two-digit decimal comma, and an optional scale word (k, m, bn, million, billion, thousand, juta). A symbol is not matched after a letter, digit, `$` or `_` (so a bcrypt `$10` is not money), and a fragment of a longer number (`1` in `1,2500`) is dropped. Sources and the unfetched symbols are in SOURCES.md.
- `recognizers/legal/` — `CASE_NUMBER` (ON by default). A shape rule at 0.90 for US federal numbers (`23-CV-0001-JFH`, `3:00CR0001-01`) and Singapore court forms (`CA/CA 1/2021`, `DC / OC 1 / 2022`, `SCT/10001/2022`); the HC/OS and SIC/S prefixes are accepted only behind a label. A label-gated generic form at 0.85 (case no, suit no, guaman no and the other labels in the file) takes one token holding a digit, which covers the Malaysian judgment forms. Neutral citations (`[2021] SGHC 123`) are never matched. The sources are cited in the file header and in SOURCES.md.
- `recognizers/terms/` — `terms.New(entityType, terms...)`, the caller-supplied term recognizer (`CUSTOM_TERM` by default; `PERSON` and `ORGANIZATION` are also accepted). Case-insensitive, whole-term only, whitespace runs match any whitespace run, longest term wins, compiled once in `New`, scores 1.00. It is NOT registered in the default pipeline; a caller adds it with `piiscrub.WithRecognizer`.
- `recognizers/identity/` — keyword-gated personal and tax identifiers: `TAX_ID`, `PASSPORT` (generic), `DRIVER_LICENSE`, `DATE_OF_BIRTH` (reports the date only). Same rules as finance, scores 0.85. A NNN-NN-NNNN tax id is matched by SSN at 0.99 and SSN wins the overlap.
- `recognizers/national/` also holds the keyword-gated country numbers `IN_PAN`, `SG_UEN`, `MY_SSM` and `MY_TIN` (keyword gate, value only, 0.85), next to the checksum-gated national IDs and `IN_GSTIN`.
- `internal/keyword/` — the shared label gate (`LabelGate`, the 0.85 score and the closed filler list) used by both keyword-gated packages.
- `recognizers/secret/` — API keys, tokens, passwords and PEM headers, each matched by shape and kept unless its value is uniform (one character repeated, or a unit of up to four repeated). A label-based match reports the value's span only: the key name (`api_key`, `password`) is context and never part of the span, and a JWT or JWE value (an `eyJ` header) is reported whole. A dotted tail is kept, except that one trailing dotted segment followed by whitespace is prose and is trimmed, and a password drops one sentence-final `.`, or one trailing `)`, `]` or `}` with no matching opener in the value, before whitespace or the end of text. The short labels `pass` and `pwd` need a non-letter before them, so `bypass=` and `compass:` are not passwords; `password`, `passwd` and `secret` have no such boundary. A provider token with no label (`bareShape`: AWS `AKIA`/`ASIA`, GitHub, Slack `xoxb-`/`xoxp-`, Stripe live, Google `AIza`, npm, SendGrid, a bare JWT) is matched by its fixed prefix and shape and reported whole, with the uniform check run on its body; it is dropped when it overlaps a labelled span, so a token in an assignment is reported once. The formats and their sources are in SOURCES.md.
- `anonymizer/` — orchestrates operators, transforms detected entities
- `operators/` — anonymization operators (replace, mask, hash, redact)
- `piiscrub.WithoutEntityTypes(types...)` / `analyzer.DisableEntityTypes(types...)` turn types off at construction time. The analyzer drops a disabled type right after the recognizers run, before context boost, the judge, the confidence filter and overlap resolution, so a disabled type never suppresses another type's span.
- `Scrubber.Redact` / `Anonymizer.Redact` replace every entity with a typed numbered marker `[[redacted<type>N]]` (e.g. `[[redactedperson1]]`, `[[redactedsocialnumber2]]`), numbered per type in text order; default `Anonymize` now renders `RedactOperator` as such a marker instead of `[REDACTED]`
- `structured/` — batch processing for CSV
- `internal/context/` — context-aware confidence boosting, applied by `Analyzer` when `WithContextBoost()` is set. PERSON and ORGANIZATION carry legal role words (plaintiff, defendant, counsel, and the Malay plaintif, perayu, pemohon and others), so a party name next to one is boosted. Opt-in only; default output does not use them.
- `recognizers/nlp/` — HTTP client recognizer delegating NER to the sidecar (root stays stdlib-only). `StartManaged` is the opt-in managed mode: it spawns the sidecar binary on a free loopback port, waits for `/healthz`, and `ManagedRecognizer.Close` stops it. On Unix the child runs in its own process group and shutdown sends SIGTERM to the group, then Kill after `managedStopTimeout`; on Windows the child starts with `CREATE_NEW_PROCESS_GROUP` and `Close` sends `CTRL_BREAK_EVENT`, falling back to an immediate kill with no drain when there is no shared console (untested at runtime). The `ctx` passed to `StartManaged`/`ManagedFromEnv` also bounds the child's lifetime, so a parent that never calls `Close` does not leak it. Startup retries on a fresh port when the child exits with a bind failure on stderr (`address already in use`, or the Windows wording), up to `managedStartAttempts` (3); any other early exit fails fast. The last 4 KiB of child stderr is appended to startup errors and still goes to this process's stderr. Once `Close` has begun, `Recognize` returns `errManagedClosed` instead of a raw dial error, and an exited child yields "sidecar process exited" with the stderr tail. Terminal states bypass `WithBestEffort`: a closed or exited sidecar always returns the error, never `nil, nil`; only transient transport errors are logged and dropped under best-effort. The env opt-in is `PII_SCRUB_NLP_BINARY` (the binary path), read by `nlp.ManagedFromEnv(ctx)`; it errors when `PII_SCRUB_NLP_URL` is also set. `nlp.FromEnv()` reads only the URL and returns `(*NLPRecognizer, error)`, erroring when the binary variable is set. The default `New` path is unchanged.
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
- `analyzer.WithLLMJudge(judge)` and `piiscrub.WithLLMJudge(judge)` register a
  weak-candidate judge on the Analyzer. Weak candidates (e.g. bare 12-digit
  MyKad with no identity-card keyword, scored 0.50) are judged before the
  confidence filter; accepted candidates are promoted to 0.85, rejected ones are
  dropped. With no judge configured (the default), weak candidates are dropped
  and output is byte-identical to running without the pass. The judge runs on a
  separate local OpenAI-compatible HTTP endpoint (loopback-only) and is
  construction-time only — calling it concurrently with Analyze is a data race.
- A boost never applies to a span that overlaps a URL which survives
  `minConfidence` (`overlapsSurvivingURL` in `analyzer.go`). Otherwise a boosted IP
  or card inside a URL can outrank the URL and leave its path and query visible.
  A URL below the threshold is dropped anyway, so it does not guard. Covered by
  `TestContextBoostDoesNotLiftSpanInsideURL` and
  `TestContextBoostIgnoresURLDroppedByThreshold`.
- `removeOverlapping` consults `winsOverlap` before confidence. Two type rules
  come first, at every score: a `CUSTOM_TERM` span wins every overlap (it is
  caller-asserted at 1.00, and it beats even CREDIT_CARD and the keyword-gated
  types), and `MONEY` loses every overlap to any other type, whatever its score.
  Then three type-pair rules apply at every score: CREDIT_CARD beats PHONE; each keyword-gated type
  (`SWIFT_BIC`, `US_ROUTING`, `CARD_CVV`, `BANK_ACCOUNT`, `PIN`, `TAX_ID`, `PASSPORT`,
  `DRIVER_LICENSE`, `DATE_OF_BIRTH`, `ISIN`, `IN_PAN`, `SG_UEN`, `MY_SSM`, `MY_TIN`,
  `CASE_NUMBER`; `isKeywordGated`) beats PHONE,
  because the keyword names the value and a PHONE match over it is a sub-run of
  that value; and CREDIT_CARD beats each keyword-gated type, so a keyword
  cannot take digits that belong to a card. A TAX_ID yields to SSN, PH_TIN,
  IN_GSTIN and MY_TIN, a PASSPORT yields to MY_PASSPORT and SG_PASSPORT, an IN_PAN
  yields to IN_GSTIN, and a MY_IC yields to MY_SSM, at every score (`yields`),
  because confidence alone does not keep them (PH_TIN scores 0.80).
  Every other pair breaks by higher
  confidence, and on an exact tie the earlier span wins. Both are policy choices, not implementation accidents. The card rule
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

## Known Blockers (t8 — Image PII / multilingual NLP / remaining benchmarks)

Blocked on external dependencies, not downloads. Network downloads verified working (dlib.net HTTP 200, 5.7MB model fetched).

- **go-face / image PII**: `libdlib-dev` is a system dependency (Debian/Ubuntu: `libdlib-dev libjpeg-dev libblas-dev liblapack-dev`). `sidecar/internal/facedetect/RealDetector` (backed by `github.com/Kagami/go-face`, already in `sidecar/go.mod`) is preferred by `NewDetector()` when dlib and all three model weights (shape_predictor_5_face_landmarks.dat, mmod_human_face_detector.dat, dlib_face_recognition_resnet_model_v1.dat) resolve; `StubDetector` is the fallback returning `ErrDlibUnavailable` otherwise. The RealDetector compiles only with cgo (`//go:build cgo`); without cgo the package builds a stub-only path. Models are downloaded on first use from dlib.net (bz2) and the go-face-testdata repo; this needs network access and a writable cache directory (`~/.cache/dataprotect/face-models/`). RealDetector path verified 2026-11-14: detected 1 face in a test image. SPEC.md § "go-face model bundling strategy".
- **Multilingual NLP model (ONNX checkpoint)**: DECISIONS.md item 3 formal decision (d) defer — no Apache-2.0/MIT checkpoint with six languages exists. `tjruesch/xlm-roberta-base-ner-hrl-onnx` (MIT, PER/ORG/LOC, 1059MB FP32 / 265MB INT8) is derived from Davlan's AFL-3.0 base model; adopting it is a licensing decision for the owner, not a download blocker. ONNX backend built and tested (`make onnx-check` green, `make test-sidecar-onnx` passes). Blocked on a licence-compliant checkpoint + `libonnxruntime`.
- **Remaining benchmarks**: DATASET_LICENSES.md "final set" is only `nemotron-pii` (CC-BY-4.0) and `gretel-pii-finance` (Apache-2.0 + NOTICE), both already fetched and measured. OpenNER (CC-BY 4.0 compilation but constituent restrictions), PIIBench (constituent licences govern), ai4privacy (≤3 staff / non-profit / evaluation), CoNLL-2003 (restricted Reuters RCV1), Kaggle PII (per-competition terms), Financial PII (Mendeley, unverified), SecretBench (MIT but gated with data protection agreement) all have licence restrictions or unresolved terms.
