# Plan: NLP sidecar for prose-based NER

Status: PLAN (revised after advisor review; not implemented). Scope: v2 item "NLP recognizer (prose-based NER for person/org/location)" plus, optionally, the "REST API sidecar" item (decide at Phase C).

## 1. Goals

- Add PERSON, ORGANIZATION, LOCATION detection to piiscrub without breaking the
  root module's stdlib-only constraint.
- Keep the root library usable offline (regex/checksum/national/secret only) when
  no sidecar is running, and keep `NewScrubber()` deterministic (no hidden
  network calls).
- Provide a standalone HTTP service that embeds the full scrubber plus NLP, so
  callers can either use the Go library with an optional NLP client or call the
  service directly.

## 2. Non-goals (this plan)

- In-process model loading in the root module (forbidden by stdlib-only rule).
- Training/fine-tuning models; we consume a pre-trained backend.
- Multi-language NER (English only for MVP).
- FPE, image PII, multi-language support (separate items).
- `/v1/analyze` and `/v1/anonymize` are speculative scope; cut unless requested
  (Phase C decision).

## 2.1 Why a sidecar (and not an in-process recognizer)

Three alternatives were considered; the sidecar wins on the project's
constraints:

1. **Pure-Go NLP in root (prose imported directly).** Rejected: the root module
   is stdlib-only, and prose (plus any future ONNX runtime) would violate that
   and bloat every consumer binary.
2. **Keyword/context rules instead of NER.** Already used for credit cards
   (see `docs/superpowers/specs/2026-10-20-credit-card-keyword-gate-design.md`),
   but person/org/location have no reliable lexical trigger; a model is needed.
3. **In-process via build tags / optional module.** Rejected: build tags make
   the API surface conditional and break `go test ./...` reproducibility; a
   separate module keeps the root deterministic and lets the sidecar ship its own
   dependency closure (prose, future ONNX).

The sidecar cost (process, IPC, deployment, failure modes) is accepted because
it is the only option that keeps the root stdlib-only while adding real NER.

## 3. Architecture

```
┌──────────────────────────────┐        HTTP/JSON (localhost)        ┌──────────────────────────────┐
│  root module (stdlib only)   │  ───────────────────────────────►  │  sidecar module              │
│  recognizers/nlp (client)    │  POST /v1/ner  {text}             │  cmd/piiscrub-sidecar        │
│  Scrubber.Analyze (regex +   │  ◄── {entities:[...]}             │  piiscrub (plain scrubber,   │
│  checksum + national +       │                                    │  NLP disabled) + prose NER   │
│  secret + optional nlp)      │                                    │  (future) ONNX backend       │
└──────────────────────────────┘                                    └──────────────────────────────┘
```

- Root module never imports the sidecar module. Sidecar imports root + prose.
- The sidecar's scrubber is the plain one (no NLP client), so there is no
  recursion by construction. No "disable" flag needed.
- NLP client is opt-in via an explicit constructor/option, never via env inside
  `NewScrubber()`.

## 4. Entity types (new)

Add to `recognizers/recognizers.go` (Presidio names, matching corpus schema):
- `EntityPerson EntityType = "PERSON"`
- `EntityOrganization EntityType = "ORGANIZATION"`
- `EntityLocation EntityType = "LOCATION"`

Plus:
- `AllEntityTypes` entries.
- `EntityTypeFromString` cases.
- Operators (redact) in `piiscrub.go`.
- `anonymizer.placeholderCollision` already iterates `AllEntityTypes`, so it
  picks these up automatically once added.
- Update README type count (14 → 17) and any docs that enumerate types.

## 5. Deterministic `NewScrubber()`

`NewScrubber()` (piiscrub.go) must stay env-free and deterministic so golden
tests are reproducible. Add an explicit constructor:

- `func NewScrubberWithRecognizers(extra ...recognizers.Recognizer) *Scrubber`
  (or a functional option `WithRecognizer`). The NLP client is passed in by the
  caller, not auto-discovered.
- Env discovery (`PII_SCRUB_NLP_URL`) happens in `main`/CLI or a helper the
  caller invokes, never in `NewScrubber()`.

## 6. Sidecar module layout

New directory `sidecar/` with its own `go.mod` (mirroring tools/corpusbench):

```
sidecar/
  go.mod          # module github.com/u007/dataprotect/sidecar
                  # replace github.com/u007/dataprotect => ../..
  cmd/piiscrub-sidecar/main.go
  internal/ner/backend.go        # NERBackend interface
  internal/ner/prose.go          # prose-based impl
  internal/server/server.go      # HTTP handlers
  internal/server/server_test.go
```

Root additions:
```
recognizers/nlp/
  nlp.go          # NLPRecognizer (client), implements recognizers.Recognizer
  nlp_test.go     # httptest-based, no network, run with -race
```

Module wiring:
- Add `go.work` at repo root (or CI steps) covering root + sidecar + corpusbench.
- Pin versions per the project's pinning rule.

## 7. HTTP API (v1)

- `GET /healthz` → 200 `{"status":"ok","backend":"prose"}`.
- `POST /v1/ner` body `{"text":"..."}` →
  `{"entities":[{"type":"PERSON","start":..,"end":..,"text":"..","confidence":0.80}]}`.

Security/lifecycle (mandatory):
- Bind `127.0.0.1` by default (flag `-addr`, default `127.0.0.1:8787`); fail
  loudly if the port is in use.
- `http.MaxBytesReader` (~1 MiB) on request bodies.
- `ReadHeaderTimeout` and `WriteTimeout` set.
- SIGTERM → `http.Server.Shutdown`.
- `/v1/anonymize` (if ever added) returns PII-adjacent text: localhost-only or
  auth-gated; document as such.

## 8. NLP client (root)

`recognizers/nlp.NLPRecognizer`:
- `New(url string, opts...)`; empty url → disabled (Recognize returns nil, nil).
- `Name()` → "nlp".
- `Recognize(ctx, text)` returns the error on any failure (fail-fast default).
  `Analyzer.Analyze` aborts on a recognizer error, so a configured-but-broken
  sidecar surfaces to the caller instead of silently degrading.
- Best-effort mode is opt-in via `WithNLPBestEffort()`: it wraps the client in
  an adapter that logs via `slog` (rate-limited) and drops the error, so the
  rest of the pipeline runs. This is the only place errors may be swallowed.
- Respect context cancellation.
- No retries.
- Shared `*http.Client` (safe to reuse); `atomic.Bool` for health state.
- Expose `Health() bool` and `LastError() error`.
- Results sorted by `Start` offset; batch endpoints (if added) paginate.

Analyzer behavior check: `Analyzer.Analyze` aborts on a recognizer error
(confirmed in analyzer.go), so fail-fast is the default and the best-effort
adapter is the only way to degrade.

Confidence: 0.80. Overlap resolution: regex, checksum and secret types (0.95+)
and the keyword-gated national ID/passport rules (0.85) all outrank NLP, so NLP
strictly fills gaps (names in prose) rather than overriding typed entities.
0.80 is also above the default 0.70 floor. The original 0.85 tied the national
rules; the root `TestNLPConfidenceWindow` and the sidecar
`TestDefaultConfidenceWindow` guard the [0.70, 0.85) window. The floor is
public: `NewScrubber(WithMinConfidence(t))` overrides 0.70, and a threshold
above 0.80 drops every NLP entity (the conservative mode). Document this
precedence.

## 9. Backend interface (sidecar)

A `NERBackend` interface exposes `Name()` and `Recognize(ctx, text)`. Prose impl
wraps `prose.NewDocument` and filters Person/Org/GPE/Location at confidence 0.85.
Future ONNX impl swaps in via `-backend=onnx -model=path`.

Prose risk: v2.0.0 is stale/unmaintained and NER quality is modest. Verify
`go mod download` resolves it before Phase B. Treat Phase B as a spike; keep
the backend interface so a swap is cheap.

## 10. Golden dataset & thresholds

- Do NOT add PERSON/ORG/LOCATION cases to `testdata/golden/seed.json`: their
  results depend on a running sidecar and prose version, which would make the
  offline gate nondeterministic.
- Put NER cases in a separate file gated by an `integration` build tag, run
  only when the sidecar is up. **Done:** `testdata/golden/ner/`, three files
  (a development set, a holdout and a final set), scored by
  `ner_accuracy_test.go`.
- Leave `minCases = 549` and existing thresholds untouched. **Done:** the
  seed gate is unchanged, and the NER gate has its own bounds and its own
  minimum case count.

## 11. Testing plan

- Root: `recognizers/nlp/nlp_test.go` with httptest server; disabled (empty url),
  success, timeout, 500, malformed JSON, context cancel. Run with `-race`.
- Sidecar: server_test with prose backend on sample sentences; healthz.
- Integration: build tag `integration` spins up the binary end-to-end.
- `go vet`/`gofmt` clean; full `go test ./...` stays green.

## 12. Phases

| Phase | Scope | Status |
|-------|-------|--------|
| A | Entity types + operators + recognizers/nlp client + tests | DONE |
| B | Sidecar module + prose backend + HTTP server + /healthz + /v1/ner (spike on prose quality) | DONE |
| C | Optional: /v1/analyze + /v1/anonymize (decide if needed) | DEFERRED |
| D | Integration NER cases (build tag) + threshold documentation | DONE |

### Phase D outcome (2026-10-20)

The NER cases live in `testdata/golden/ner/` (92 cases) and the gate is
`ner_accuracy_test.go`, run by `make test-ner`, which starts a sidecar, scores
the detector and stops it again.

Two metrics, because the NER types share one operator (redact):

- **typed** — the golden package's own, type-sensitive metric. P=0.8395
  R=0.8718 over 124 cases.
- **span** — type-agnostic, computed in the gate. P=0.9012 R=0.9359. This is
  the number that protects the caller's data: a span covered exactly but
  typed differently is redacted just the same, so the typed metric
  understates what the caller actually gets.

The dataset has FOUR files. The fourth, `structured.json` (32 cases), was
added after the corpora showed the decoder reporting JSON keys, HTTP headers
and spreadsheet field labels as names. The first three sets were all prose
and had a blind spot there.

The generalisation figure is the FIRST evaluation of the final set, on text
the decoder had never seen: typed P=0.5333 R=0.6154, span P=0.8667 R=1.0000.
That number is recorded in the gate, not the flattering re-run.

prose's own entity output is NOT used. Its coalescer keeps the first token's
label for a span, so "Fernwood Logistics Inc" came back as a GPE and "Samuel
Okoye" as an ORGANIZATION. The sidecar decodes prose's token labels instead
(`sidecar/internal/ner/prose.go`).

The gate was mutation-verified: breaking the typing made the typed gate fail
while the span gate still passed, and breaking the coverage made both fail.
Neither gate can hide behind the other.
| E | ONNX backend (optional, future) | Later |
| F | Managed process mode (optional, future) | Later |

## 13. Decisions needed before coding

1. Sidecar port default (8787) and URL env var (`PII_SCRUB_NLP_URL`, full URL).
2. Whether NLP recognizer is registered by default when env set, or only via
   explicit constructor. Recommendation: explicit constructor only (deterministic
   NewScrubber); env discovery in caller.
3. Confidence 0.85 and no context boost for NLP.
4. Whether to implement Phase C (REST sidecar) or keep NLP-only. Recommendation:
   defer C until a caller asks.

Resolved (2026-10-20): port default 8787; env var PII_SCRUB_NLP_URL (base URL or full /v1/ner endpoint); explicit constructor only (NewScrubber stays env-free); confidence 0.85; Phase C deferred.

## 14. Risks

- prose accuracy on names in noisy text may be low; integration gate will expose.
  Mitigation: start with redact (safe) and measure.
- Sidecar process management in production (restarts, health) is out of scope
  for MVP; document as operational concern.
- CGO: prose is pure Go; ONNX backend would add CGO. Keep default backend
  prose-only so the sidecar binary builds without CGO.
- Module versioning: sidecar depends on root via replace; CI must build both.
  The repository has no CI workflow yet, so the rule is carried by the
  Makefile: `make build` (root), `make build-sidecar`, `make test-sidecar` and
  `make vet-sidecar` are the four commands a CI job must run, in that order.
  `go.work` is gitignored, so CI resolves each module on its own and the
  `replace` directives carry the link; the workspace file is a local
  convenience only.