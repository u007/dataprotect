---
type: Decision
title: ONNX doc corrections (3264db5) — special-token opt-in, env-gated success tests
description: 'Dated 2026-11-13 correction record: 3264db5 adds an opt-in special-token path; the ONNX success-path tests are env-gated and skipped on this tree; three multilingual checkpoint leads remain unverified.'
resource: sidecar/internal/ner; docs
tags:
  - onnx
  - ner
  - sidecar
  - correction
  - status
timestamp: 2026-11-13T02:11:31Z
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
- The ONNX success path: environment init, session creation and inference.
- Windows runtime behaviour: only `GOOS=windows go build` is exercised; the
  `CREATE_NEW_PROCESS_GROUP` / `CTRL_BREAK_EVENT` path has no runtime test.

## 5. Where this is recorded

- [superpowers/specs/2026-10-28-multilingual-ner-design.md](/superpowers/specs/2026-10-28-multilingual-ner-design.md)
  — status line corrected with both the opt-in flag and the env-gated test
  facts.
- [decisions/2026-11-12-session-findings.md](/decisions/2026-11-12-session-findings.md)
  — section 2 corrected and a 2026-11-13 correction note appended.
- `docs/log.md` — this entry's dated record; the 2026-11-02 log line ("the
  `<s>`/`</s>` expectation is an inference, not a verified result") is
  superseded only in that an opt-in flag now exists; it is still not a verified
  result.