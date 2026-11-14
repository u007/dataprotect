---
type: Decision
title: ONNX doc corrections (3264db5) — special-token opt-in, env-gated success tests
description: 'Dated 2026-11-13 correction record: 3264db5 adds an opt-in special-token path; the ONNX success-path tests were env-gated and skipped on this tree; three multilingual checkpoint leads remain unverified. Updated 2026-11-14: the success path PASSes with onnxruntime 1.32.0 (ORT_API_VERSION 29) and distilbert/ner; Windows cross-compiles but runtime is untested.'
resource: sidecar/internal/ner; docs
tags:
  - onnx
  - ner
  - sidecar
  - correction
  - status
timestamp: 2026-11-14T13:25:05Z
---
# ONNX doc corrections (3264db5) — special-token opt-in, env-gated success tests

Date: 2026-11-13. Dated correction record for the ONNX multilingual NER docs.
Every claim below was checked against the tree on 2026-11-13, not inferred from
an earlier note.

## 1. `3264db5` — opt-in special tokens

`feat(onnx): opt-in special-token wrapping in hf tokenizer adapter` adds:

- `NewHFTokenizerWithOptions(path string, addSpecial bool)` in
  `sidecar/internal/ner/hf_tokenizer.go`. It passes `addSpecial` to
  `EncodeWithOptionsErr`, so `true` wraps each sequence with the model's
  `<s>`/`</s>` separators (XLM-R style); the tokens are marked `Special=true`
  and dropped by the decoder.
- `NewHFTokenizer(path)` now calls `NewHFTokenizerWithOptions(path, false)`.
  The default is unchanged: a 400-token chunk is exactly 400 content tokens.
- `ONNXConfig.AddSpecialTokens bool` in `sidecar/internal/ner/onnx_config.go`,
  wired through `NewONNXBackend` to `NewHFTokenizerWithOptions`
  (`sidecar/internal/ner/onnx_backend.go:84`).
- `TestHFTokenizerSpecialTokens` in `hf_tokenizer_test.go`, which checks that
  the default emits no special tokens and `addSpecial=true` emits some.

The flag is **not** exposed as a CLI flag on `sidecar/cmd/piiscrub-sidecar` yet:
`main.go` passes only `ModelDir`, `TokenizerPath` and `ONNXRuntimeLib`.

The real checkpoint's special-token requirement is still **unverified in
Phase 1**. The XLM-R configs declaring `bos_token_id 0`, `eos_token_id 2` and
`max_position_embeddings 514` remain evidence, not proof. This change makes the
requirement testable; it does not settle it.

## 2. Correction — success-path tests are env-gated and skipped here

`sidecar/internal/ner/onnxrt/session_test.go` contains
`TestInitEnvironmentSuccess` and `TestSessionRunSuccess`. They are gated on
`NER_ONNX_RUNTIME_LIB` and `NER_ONNX_MODEL` and skip when either is unset. On
this tree both were unset:

```
NER_ONNX_RUNTIME_LIB unset; skipping success-path InitEnvironment test
NER_ONNX_RUNTIME_LIB or NER_ONNX_MODEL unset; skipping success-path Session.Run test
```

The test-file header states that the success path needs a real
`libonnxruntime`, which the test machine does not have. Earlier docs claimed
the tests "were verified against a `distilbert/ner` export plus an onnxruntime
shared library"; that was false and has been corrected.

`make onnx-check` is green on this tree (verified 2026-11-13, exit 0). It runs
`cd sidecar && go build -tags onnx ./...`, so it is a build gate only and is
vacuous for the success path: it does not call `InitEnvironment`, create a
session or run inference.

**Superseded 2026-11-14:** the success path has since been run with a real
runtime and model and both tests PASS. See §6.

## 3. Unverified multilingual checkpoint leads

A websearch surfaced these model cards as leads:

- `henda/bert-multilingual-ner`
- `orgcatorg/bert-base-multilingual-cased-ner`
- `ugurcanozalp/multilingual-ner`

They are **unverified leads, not vetted models**. Neither their licence nor
their training data has been checked, so they are not adopted and do not meet
the design's Apache-2.0/MIT six-language bar. No licence claim is made for
them here.

## 4. Still unverified

- The real checkpoint's special-token requirement (Phase 1).
- Windows **runtime** behaviour: cross-compilation is verified (see §6), but
  the `CREATE_NEW_PROCESS_GROUP` / `CTRL_BREAK_EVENT` path has no runtime test.
  Cross-compilation is not runtime validation.

The ONNX success path (environment init, session creation and inference) was
listed here on 2026-11-13 and is **no longer unverified**: it PASSed on
2026-11-14 (see §6).

## 5. Where this is recorded

- [superpowers/specs/2026-10-28-multilingual-ner-design.md](/superpowers/specs/2026-10-28-multilingual-ner-design.md)
  — status line corrected with both the opt-in flag and the env-gated test
  facts, then updated with the 2026-11-14 verification.
- [decisions/2026-11-12-session-findings.md](/decisions/2026-11-12-session-findings.md)
  — section 2 corrected and a 2026-11-13 correction note appended, then a
  2026-11-14 verification update.
- `docs/log.md` — this entry's dated record.

## 6. Verification (2026-11-14)

The 2026-11-13 note recorded the success path as unverified because
`NER_ONNX_RUNTIME_LIB` and `NER_ONNX_MODEL` were unset. That is now superseded.

- **ONNX success path — verified.** `TestInitEnvironmentSuccess` and
  `TestSessionRunSuccess` both **PASS** with onnxruntime 1.32.0
  (`ORT_API_VERSION` 29) and the `distilbert/ner` model. This is a real
  `InitEnvironment` + `NewSession` + `Run`, not the build gate.
- **Windows cross-compilation — verified.** `GOOS=windows go vet` and
  `GOOS=windows go test -c` both pass. **Runtime is still untested:** the
  `CREATE_NEW_PROCESS_GROUP` / `CTRL_BREAK_EVENT` path has not run on Windows.
  Do not read the cross-compilation result as runtime validation.
- **Image PII — `RealDetector` now works.** `libdlib-dev` is installed and the
  model downloader gained `mmod_human_face_detector.dat`
  (`sidecar/internal/facedetect/models.go`). go-face's `NewRecognizer` needs
  that file; without it the constructor failed and `NewDetector` fell back to
  `StubDetector`.

Still unverified after this run: the real checkpoint's special-token
requirement (Phase 1).
