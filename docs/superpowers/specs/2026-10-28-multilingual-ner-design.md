# Multilingual NER backend — design

Date: 2026-10-28
Status: approved design; Phase 0 partial (2026-10-31): golden.Case.Language; sidecar/internal/ner/decode.go (Token/Tokenizer/Session seams, Decode with BIO/BIOES span assembly, guards, label mapping, dedup, overlap resolution); and the !onnx stub (onnx_stub.go, build tag !onnx) plus untagged ONNXConfig (onnx_config.go), wired into main.go via -model/-tokenizer/-onnxruntime-lib flags (stub returns an error naming -tags onnx) landed with tests; plain build and sidecar tests pass. Tagged backend files (onnx_backend.go, onnxrt/session.go, hf_tokenizer.go, testdata/tokenizer_parity.json), onnx module deps, Makefile targets, and chunk-boundary handling still pending (see TODO.md).
Track: TODO.md "Multilingual NLP model"

Scope: the optional sidecar (`sidecar/`, a separate Go module), a new committed multilingual golden set, and Makefile targets. The root module, the `/v1/ner` protocol, the default sidecar build and the English prose backend are unchanged.

## 1. Problem

The sidecar embeds `github.com/jdkato/prose/v2`, which is English-only (a 17 MB model). `PERSON`, `ORGANIZATION` and `LOCATION` became in-scope on 2026-10-20, and the NLP path contributes nothing to the gretel multilingual splits (French, German, Italian, Spanish, Swedish, Dutch). Measured false negatives on the 2000-case samples: nemotron 3275 / 1988 / 1018 and gretel 2577 / 1489 / 2374 for PERSON / ORGANIZATION / LOCATION. A multilingual NER model is the remaining lever for those types.

## 2. Goals and non-goals

Goals:

- G1. A multilingual (de, nl, fr, it, es, sv) `PERSON`/`ORGANIZATION`/`LOCATION` backend in the sidecar, opt-in.
- G2. The default sidecar build stays CGO-free and prose-only.
- G3. The root module is untouched: no require block, no new imports.
- G4. Reproducible acceptance: a committed multilingual golden gate plus a per-language corpus measurement.
- G5. Misconfiguration fails at startup, not per request, and never degrades silently.

Non-goals: `MISC`, dates, and every type outside the three NER types; language auto-detection; replacing or changing the prose backend; runtime downloads; cloud APIs; GPU execution; changing the `/v1/ner` request or response shape; batching; root-module changes.

## 3. Decisions

- D1. Runtime contract: a `onnx` build tag. A plain build contains only the prose backend and no CGO or link step. `go build -tags onnx` adds the ONNX backend. `-backend=onnx` on a plain build returns an error that names the tag.
- D2. Reference model: multilingual XLM-R base fine-tuned for token classification, int8 quantised (approximately 280 MB). Licence must be Apache-2.0 or MIT. The concrete checkpoint and its licence are recorded in Phase 1; the code is model-agnostic and reads its labels from `config.json`.
- D3. Artifact supply: explicit paths only. No download at runtime and no implicit cache. Flags: `-model` (a directory containing `model.onnx` and `config.json`), `-tokenizer` (a `tokenizer.json`), `-onnxruntime-lib` (the `libonnxruntime` shared library).
- D4. Engine: `github.com/yalue/onnxruntime_go` for the session and `github.com/daulet/tokenizers` (cgo, the exact Hugging Face Rust core) for tokenisation, behind a tokenizer seam. Both modules are already in this machine's module cache. A committed parity fixture guards the adapter's offsets. No pure-Go tokenizer is planned; any future one must pass the same fixture before use.
- D5. Confidence: every ONNX entity is emitted at the fixed NLP score of 0.80. Overlap between ONNX entities is resolved inside the backend, so the analyzer never compares two NLP entities.
- D6. Evidence: a committed multilingual golden set scored in record mode, plus a per-language gretel corpus measurement. Accuracy bounds are introduced only after a real measured run, never before.

## 4. Architecture

Files and build tags:

| File | Build | Purpose |
| --- | --- | --- | --- |
| `sidecar/internal/ner/backend.go` | all | unchanged `NERBackend` interface |
| `sidecar/internal/ner/prose.go` | all | unchanged English backend |
| `sidecar/internal/ner/decode.go` | all | pure Go: span assembly, label mapping, guards, intra-backend overlap resolution |
| `sidecar/internal/ner/decode_test.go` | all | table tests for the decoder |
| `sidecar/internal/ner/onnx_backend.go` | `onnx` | implements `NERBackend` over `Tokenizer` and `Session` |
| `sidecar/internal/ner/onnx_stub.go` | `!onnx` | same constructor, returns an error naming `-tags onnx` |
| `sidecar/internal/ner/onnxrt/session.go` | `onnx` | thin `Session` over `onnxruntime_go` |
| `sidecar/internal/ner/hf_tokenizer.go` | `onnx` | adapts `daulet/tokenizers` to `Tokenizer` |
| `sidecar/internal/ner/testdata/tokenizer_parity.json` | all | reference token strings and byte offsets |
| `sidecar/cmd/piiscrub-sidecar/main.go` | all | flags and backend selection, no build tags |
| `testdata/golden/ner_multilingual/` | n/a | committed multilingual cases |
| `ner_accuracy_test.go` | `integration` | adds `TestNERMultilingual` |

Two seams keep the risky logic testable without a runtime:

```go
type Token struct {
	ID        int64
	Start     int  // byte offset in the original text
	End       int  // byte offset, exclusive
	Special   bool // tokenizer special mask, never inferred from the index
	WordStart bool // true when the token begins a word
}

type Tokenizer interface {
	Tokenize(text string) ([]Token, error)
}

type Session interface {
	InputNames() []string
	Run(ctx context.Context, feeds map[string][]int64) ([][]float32, error)
}
```

`Tokenizer` returns byte offsets, so `decode.go` never handles rune offsets. `Session.Run` returns logits as `[sequence][labels]`.

`main.go` stays tag-free: the `!onnx` stub makes `-backend=onnx` fail with an actionable message instead of "unknown backend", and `go build ./sidecar/...`, `make test-sidecar` and the root suite keep working with no CGO.

## 5. Model contract

- `-model` must be a directory containing `model.onnx` and `config.json`.
- `config.json` is the only source of the label set. `id2label` is parsed into an ordered slice; its length must equal the ONNX output channel count, or startup fails.
- Required inputs are `input_ids` and `attention_mask`. `token_type_ids` is fed only when the session declares it. Nothing is assumed about the input set: the names come from `InputNames()`.
- The tag prefix (`B-`, `I-`, `E-`, `S-`) drives BIO decoding; when a tag is mapped to a type the prefix is stripped first. Type aliases: `PER` and `PERSON` map to PERSON; `ORG` and `ORGANIZATION` map to ORGANIZATION; `LOC`, `GPE` and `LOCATION` map to LOCATION. Every other label, including `MISC`, is dropped.
- The export recipe is documented, not executed by this project: `optimum-cli export onnx --model <checkpoint> --task token-classification <dir>`, followed by the int8 quantisation step. The model's opset and quantisation mode are read and logged at startup.
- The `onnxruntime_go` module version is pinned. The matching `libonnxruntime` version is documented; a header/library mismatch is a crash, not an error, so both are recorded in the docs and checked by the operator.
- The checkpoint licence is verified in Phase 1 and recorded in `SOURCES.md` and `DATASET_LICENSES.md`. A licence outside Apache-2.0/MIT rejects the checkpoint.

## 6. Decoding

Startup:

1. Validate all three paths and read `config.json`.
2. Assert `len(id2label)` equals the logit channel count and that `input_ids` and `attention_mask` are declared.
3. Call `InitializeEnvironment`, set the shared-library path, create an `AdvancedSession`, run one warmup inference.
4. Log the SHA-256 of `model.onnx` and `tokenizer.json`, the opset, the quantisation mode and the RSS baseline.
5. Any failure is fatal at startup.

Per request:

1. Tokenize the text into `[]Token` with byte offsets.
2. Feed `input_ids` and `attention_mask`, plus `token_type_ids` if declared, with the text truncated to the model maximum.
3. Per token, take the argmax label and its softmax probability.
4. Decode `BIO`/`BIOES` on token-span adjacency, not subword adjacency. A label on a continuation piece never opens a span: it either continues an open span or is dropped. A label on a special token is dropped.
5. Treat the sentence-piece marker and a leading WordPiece continuation marker as word-internal when deciding `WordStart`.
6. The tokenizer adapter converts its offsets to UTF-8 byte offsets in the original text (accounting for normaliser changes) and clamps them to `[0, len(text))`; the decoder only ever sees byte offsets.
7. Reject spans that are whitespace-only, longer than 16 tokens or 256 bytes, or whose first token starts mid-word.
8. Map the label to a type; drop unmapped labels.

Long text: chunk at 400 tokens with a 64-token stride, deduplicate identical `(start, end, type)` spans across chunks, and hard-cap a document at 8000 tokens with a logged truncation. A request that cannot be processed returns an error; partial entities are never returned.

## 7. Confidence and overlap

Every ONNX entity is emitted at 0.80, the existing NLP score. That is strictly above the 0.70 default floor and strictly below the 0.85 lowest built-in, so the "NLP fills gaps, never overrides typed" invariant holds exactly.

Overlap between ONNX entities is resolved in the backend: for a group of overlapping spans keep the longest span, breaking ties by the higher softmax probability. Identical `(start, end, type)` spans are deduplicated. The analyzer's overlap rules (`CREDIT_CARD` beats `PHONE`, otherwise higher confidence, exact ties by earlier span) are untouched.

## 8. Build and tooling

- `make build-sidecar-onnx`: builds the sidecar with `-tags onnx`.
- `make test-sidecar-onnx`: runs `go test -tags onnx ./...` inside `sidecar/`; tests that need the runtime library are skipped when it is absent.
- `make onnx-check`: `cd sidecar && go build -tags onnx ./...`. This is the CI compile gate. It needs the Go modules and a C toolchain (cgo), but no ONNX Runtime installation: the binding compiles against vendored headers and loads the library at runtime. The gate has not been run yet, so its green state is unverified until Phase 0.
- `make test-ner-onnx`: builds the tagged sidecar with the three paths from environment variables, starts it, runs `TestNERMultilingual`, and stops it.
- `make corpus-nlp` gains `NER_BACKEND` (default `prose`) and passes `-backend=$(NER_BACKEND)`; existing invocations are unchanged.
- Module hygiene: `go mod tidy` runs inside `sidecar/`; `sidecar/go.sum` and `go.work.sum` are committed; the ONNX modules are version-pinned. The root `go.mod` gains nothing.
- CI: existing jobs are untouched. A separate job runs `make onnx-check`; it needs a module cache on first run and a C toolchain in the image.

## 9. Evaluation

The multilingual gate is a new directory, `testdata/golden/ner_multilingual/`, reusing the existing `golden.Case` schema. The English gate at `testdata/golden/ner/` is read by directory, so mixing the sets would silently change its denominators; the two stay separate.

Content: approximately 12 cases per language for de, nl, fr, it, es and sv, mixing prose, structured documents and negatives. Gold spans are authored or curated from human labels. They are never generated from the model's own output.

A case needs a language carrier: `golden.Case` has no language field today, and `Load` rejects unknown JSON fields. Phase 0 therefore adds `Language string` (`json:"language,omitempty"`) to `golden.Case` in the root `golden` package, validated against the six codes when non-empty; an empty value keeps every existing dataset unchanged. Deriving the language from the case id is rejected as an implicit convention. The gate groups its per-language metrics by this field.

`TestNERMultilingual` sits behind the `integration` build tag only. The `onnx` tag belongs to the sidecar build, not the test, because the test talks to the running sidecar over HTTP.

Record mode:

- With `-update`, the test runs the sidecar, writes `metrics.json` and passes.
- Without `-update` and without `metrics.json`, the test skips with the instruction to run with `-update` on a machine that has the artifact.
- Without `-update` and with `metrics.json`, the test asserts per-language and aggregate typed and span precision and recall against bounded constants.

`metrics.json` records: typed and span tp/fp/fn/precision/recall per language (keyed by `Case.Language`), the aggregate, the model and tokenizer SHA-256, the opset, and the quantisation mode.

Bounds policy: bounds are introduced only in a commit that also contains the measured `metrics.json`. Each bound is the measured value minus 0.02, rounded down to two decimals, and the measured numbers are quoted in the test comment and in `SPEC.md`. No bound may be written before the measurement exists.

Corpus measurement: `make corpus-nlp NER_BACKEND=onnx NER_LIMIT=400`, recording per-language gretel NER precision and recall in `SPEC.md`, alongside the current prose numbers.

## 10. Failure handling and operations

- Startup is the only place configuration can fail: missing paths, unreadable files, a label-count mismatch, a missing input, a failed environment initialisation, a failed session creation or a failed warmup. Each is fatal with an error naming the flag and the docs target.
- A per-request failure is logged with its cause and returns an error to the caller; the server maps it to a 5xx response. No partial entity list is returned.
- ONNX Runtime sessions are safe for concurrent `Run` calls. The tokenizer is guarded by a mutex because its safety is not part of its contract.
- Intra-op threads default to 1 and `OMP_NUM_THREADS=1` is documented; the operator can raise the thread count.
- Memory: the int8 XLM-R base is approximately 280 MB on disk; the runtime arena and session add overhead. Document a ~1 GB peak and log the RSS baseline at startup.
- Inference is synchronous. A client timeout cancels the HTTP response but not the in-flight ONNX call; this is documented rather than hidden.
- Air-gapped installs work: all three artifacts are local paths.

## 11. Test plan

| Test | Tag | Needs artifact | Proves |
| --- | --- | --- | --- | --- |
| `decode_test.go` | none | no | span assembly, label mapping, guards, overlap, chunk boundaries |
| `tokenizer_parity_test.go` | `onnx` | yes (`tokenizer.json`) | token strings and byte offsets match the reference tokenizer |
| `go build -tags onnx ./...` | `onnx` | no | the tagged code compiles |
| `TestNERMultilingual` | `integration` | yes | multilingual typed and span accuracy |
| `make corpus-nlp NER_BACKEND=onnx` | n/a | yes | real-corpus per-language NER recall |
| `make test-ner` | `integration` | no | the prose English gate is unchanged |

Decoder cases: plain ASCII; French accented characters; Swedish å, ä, ö; Dutch digraphs; combining marks; special tokens; `B-`/`I-`/`BIOES`; a continuation piece at a span start; whitespace-only spans; spans over the token or byte limit; chunk-boundary duplicates; nested and overlapping entities; an unknown label; a logits/token-count mismatch.

The parity fixture is generated once from the pinned `tokenizer.json` with the reference Hugging Face tokenizer and committed as data, together with that tokenizer's SHA-256. The test loads the same `tokenizer.json`, tokenizes each fixture sentence through the adapter, and compares token strings and byte offsets. Because `daulet/tokenizers` wraps the same Rust core, this guards the adapter's offset conversion rather than the model's tokenisation rules.

## 12. Rollout and acceptance

Phase 0 (no runtime artifact or ONNX Runtime install needed; the two Go modules must resolve and a C toolchain must be present): decoder and mapping with tests; the `!onnx` stub; the `onnx` session wrapper and tokenizer adapter; the compile gate; Makefile targets. Acceptance: the default sidecar build and `go test ./sidecar/...` are unchanged; `go build -tags onnx ./...` succeeds; the decoder tests pass; the root suite is unchanged.

Phase 1 (needs ONNX Runtime and the artifact): tokenizer parity fixture; startup validation and warmup; a real-machine run writes `metrics.json`; the per-language corpus table is produced. Acceptance: parity passes, the run completes, and the metrics are committed.

Phase 2: bounds are derived from the measured metrics; docs are updated. Acceptance: the multilingual gate passes with the bounds; the English gate and its bounds are unchanged; `SPEC.md`, `SOURCES.md`, `DATASET_LICENSES.md`, `TODO.md` and `AGENTS.md` describe the backend; `make docs-sync` regenerates `docs/decisions/spec-v1.md`.

No phase may invent an accuracy number. A phase that cannot be run on the implementing machine is recorded as unverified and left for the next phase. At the time of writing, the compile gate has not been run: it is a Phase 0 deliverable, and this document does not claim it passes.

## 13. Risks

| Risk | Mitigation |
| --- | --- |
| `onnxruntime_go` headers and the installed `libonnxruntime` differ | pin the module version, document the matching runtime version, fail at startup |
| the tokenizer adapter's offsets diverge from Hugging Face | `daulet/tokenizers` is the exact Rust core; the committed parity fixture still guards the adapter's byte offsets |
| character-to-byte offset conversion is wrong for non-ASCII | dedicated tests per language, `ConvertOffset`, clamping, and a guard against mid-word starts |
| int8 quantisation degrades accuracy | measure typed and span metrics, and keep the quantisation mode in `metrics.json` |
| the checkpoint licence is unacceptable | verify before adoption; the model is a flag, not a compile-time choice |
| memory or latency exceeds the sidecar budget | log RSS and measure latency; the backend is opt-in |
| the gate overfits to hand-written cases | pair it with the corpus measurement and never derive gold from model output |
| the track cannot be validated in the current environment | phase separation: Phase 0 needs no runtime or artifact, and Phases 1 and 2 are explicitly unverified until run |

## 14. Documentation obligations

- `SPEC.md`: a section describing the backend, its flags, the confidence policy and the measured corpus table once available.
- `AGENTS.md`: the sidecar now has two backends and the build-tag split.
- `SOURCES.md`: the checkpoint, its licence and the pin.
- `DATASET_LICENSES.md`: the model licence.
- `TODO.md`: the multilingual NLP model item moves from open to its measured status.
- `docs/decisions/spec-v1.md` and the rest of the OKF bundle: regenerated only through `make docs-sync` and the context agent; this spec does not edit them directly.
