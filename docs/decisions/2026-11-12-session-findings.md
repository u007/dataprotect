---
type: Decision
title: NLP managed mode, ONNX and LLM second pass — session findings (2026-11-12)
description: 'Dated 2026-11-12 record: managed-NLP Sonnet review items resolved/open, ONNX success-path tests and the Phase 1 licence block, LLM second-pass Phase 3 fixture, the France phone gap, the licensing status correction, and a same-session follow-up that closes item (3) and partly closes (6).'
resource: recognizers/nlp; sidecar/internal/ner/onnxrt; testdata/judge; THIRD_PARTY.md
tags:
  - nlp
  - onnx
  - llm
  - mykad
  - managed
  - review
  - status
timestamp: 2026-11-12T03:09:48Z
---
# NLP managed mode, ONNX and LLM second pass — session findings (2026-11-12)

Dated record of what the 2026-11-12 session settled. It is append-only: the
topical docs keep their history and this note records resolutions, remaining
gaps and one stale-status correction. Code claims below were re-checked against
the tree on 2026-11-12, not inferred from the review text.

## 1. NLP managed mode — Sonnet review follow-ups

The Sonnet review of the managed sidecar listed seven items. Three are now
resolved in code, three remain open, and one comment still overclaims.

### Resolved

- **(1) Shared `LastError` inference removed.** `ManagedRecognizer.Recognize`
  now returns its own result. It checks `stateErr()` before and after the
  delegate call and returns the delegate's entities/error; it no longer infers
  failure from a shared `LastError()` that a concurrent call could overwrite
  (`recognizers/nlp/managed.go:162-173`). Landed in `ac7c6fd` (feat(nlp):
  terminal states bypass best-effort; wire NLP binary opt-in into tooling;
  Windows build gate).
- **(2) Terminal states bypass best-effort.** Once `Close` has begun or the
  child has exited, `Recognize` returns `errManagedClosed` or "sidecar process
  exited" even under `WithBestEffort`; only transient transport errors are
  logged and dropped. Documented in
  [concepts/development-conventions.md](/concepts/development-conventions.md)
  and [concepts/piiscrub-usage.md](/concepts/piiscrub-usage.md).
- **(7) Port-retry test flake removed.** `managed_test.go` no longer asserts
  that two startup attempts use different ports. `TestManagedRecognizeUsesChildPort`
  asserts the recognizer talks to the child's chosen port
  (`recognizers/nlp/managed_test.go:261-276`); `TestStartManagedRetriesWhenPortIsTaken`
  keeps the behavioural retry assertion without a cross-attempt distinct-port
  check (`:332`).

### Still open

- **(3) `ManagedFromEnv` takes no arguments or options**, so a caller cannot
  pass a binary-adjacent option (timeouts or logger) through the env opt-in.
- **(4) No Windows stop test or CI job.** Only `GOOS=windows go build` is
  exercised; the `CREATE_NEW_PROCESS_GROUP` / `CTRL_BREAK_EVENT` path has no
  runtime test.
- **(5) No `Close`/ctx-cancel race test.** There is no test that races
  `Recognize` against `Close` or a cancelled `StartManaged` ctx. (`WithBestEffort`
  on the managed path *is* covered, at `managed_test.go:604` and `:633`.)

### Comment overclaim

- **(6) MyKad filler comment overclaims for `ic no no`.** The comment in
  `recognizers/national/national.go:164-166` says the two filler words must
  differ so `"no no"` does not match. That is true for the bare `mykad no no
  <digits>` form, but the keywords `ic no`, `i/c no`, `kp no` and `k/p no`
  already contain `no`, so `ic no no <12 digits>` still matches as keyword plus
  one filler. Options are the plain `{0,2}` filler quantifier or a Go-side
  compare; the comment must not claim more than the rule enforces.

### Residuals (deliberately not fixed)

- **Windows no-console kill leaves grandchildren.** When no shared console
  exists, `Close` falls back to an immediate kill with no drain; child
  processes started by the sidecar survive. Untested at runtime on Windows.
- **`ic no no <12 digits>` ambiguity is deferred to the LLM second pass**
  (decision 2026-11-06), not handled with more regex special-casing.

## 2. ONNX multilingual NER — success-path tests

`sidecar/internal/ner/onnxrt/session_test.go` now has success-path coverage:

- `TestInitEnvironmentSuccess`
- `TestSessionRunSuccess`

They are gated on `NER_ONNX_RUNTIME_LIB` and `NER_ONNX_MODEL` and were verified
against a `distilbert/ner` export plus an onnxruntime shared library. Commit
`cdca70d` (feat(onnx): add success-path session tests (gated on env)). The
failure-path `TestInitEnvironmentFailureIsNotCached` remains.

**Phase 1 is still blocked on the checkpoint licence.** The design requires a
six-language (de, nl, fr, it, es, sv) checkpoint under Apache-2.0 or MIT, and
the survey re-confirmed that none exists: `Davlan/xlm-roberta-base-ner-hrl` is
AFL-3.0, `Babelscape/wikineural-multilingual-ner` is CC BY-NC-SA 4.0, and
`flair/ner-multi` covers only four languages and names no licence. The status
line in
[superpowers/specs/2026-10-28-multilingual-ner-design.md](/superpowers/specs/2026-10-28-multilingual-ner-design.md)
is updated accordingly.

## 3. LLM second pass — Phase 3 pending, fixture exists but is unwired

Phases 1 (weak-candidate marker) and 2 (judge client) shipped in `daa62e1`;
the model is Qwen3-0.6B, Apache-2.0 (see
[decisions/llm-second-pass-model.md](/decisions/llm-second-pass-model.md)).

Phase 3 has a 10-case synthetic fixture at `testdata/judge/mykad.json`:

- 5 bare-MyKad positives, including one keyed sanity case;
- 5 look-alike negatives.

It is **not wired**: there is no test harness, no `make` target, and no
measurements. Phase 3 therefore remains pending regardless of the fixture.

## 4. Phone — France bracket form stays a deliberate gap

France's bare `[+33] …` bracket form is still not matched. This is deliberate,
not an oversight: the rule is that no regex shape is invented without a
labelled example or a provider page behind it.
[decisions/spec-v1.md](/decisions/spec-v1.md) already records the gap in its
non-English phone section; this note only reaffirms it.

## 5. Licensing — stale status corrected

[concepts/licensing-and-provenance.md](/concepts/licensing-and-provenance.md)
said `THIRD_PARTY.md` was uncommitted at the time of writing. It is now
committed in `3bf03f2` (docs(licensing): add optional-components disclosure and
THIRD_PARTY inventory), together with the README and `.env.example` changes.
That paragraph has been refreshed.

## 6. Repo-root TODO.md — stale unchecked entries

`TODO.md` (repo root, outside this bundle) still has unchecked items that the
session superseded:

- the optional LLM second-pass item says "no code yet" although Phases 1 and 2
  shipped (`daa62e1`);
- the ONNX success-path-tests item is done (`cdca70d`);
- Sonnet review items (1), (2) and (7) are resolved.

The managed-mode residuals (Windows stop test/CI, `ManagedFromEnv` options, the
`Close`/cancel race test, the MyKad comment, the Windows grandchild kill) stay
open.

## 7. Same-session follow-up — item (3) resolved, item (6) partly resolved

Appended after the sections above. Where it contradicts a "still open" or
"overclaim" line, this section supersedes it; the earlier text is left unchanged
because this note is append-only.

### (3) Resolved — `ManagedFromEnv` options

`ManagedFromEnv` is now
`func ManagedFromEnv(ctx context.Context, opts ...Option) (*ManagedRecognizer, error)`
(`recognizers/nlp/nlp.go:243`) and forwards the options to
`StartManaged(ctx, binary, nil, opts...)` (`recognizers/nlp/nlp.go:252`). The
env opt-in is therefore configurable — for example `WithBestEffort`,
`WithTimeout` or `WithLogger` — without dropping to `StartManaged` directly.
Adding a variadic parameter is backward-compatible: existing
`ManagedFromEnv(ctx)` call sites compile unchanged. The doc comment states the
forwarding.

### (6) Partly resolved — MyKad filler comment

The comment at `recognizers/national/national.go:171-176` no longer claims that
`"no no"` is rejected in every form. It now records the caveat: keywords that
already contain "no" (`ic no`, `i/c no`, `kp no`, `k/p no`) let
`ic no no <digits>` match as keyword plus one filler, because the distinctness
rule applies only to the two filler slots, not to a "no" that is part of the
keyword. The change is comment-only, so behavior is unchanged: `mykad no no
<digits>` is still rejected, and the `ic no no` ambiguity stays deferred to the
LLM second pass per the 2026-11-06 decision. The alternative fixes listed under
item (6) above (plain `{0,2}` quantifier, Go-side compare) were not taken — the
decision is not to special-case this in the regex.

### Sonnet review status after this session

Items (1), (2) and (7) were already resolved; (3) is now resolved and (6) is
partly resolved (comment corrected, ambiguity deliberately deferred). **(4) and
(5) remain open**: no Windows stop test or CI job, and no test that races an
in-flight `Recognize` against `Close` or a cancelled ctx.

## Where this is recorded

- [concepts/development-conventions.md](/concepts/development-conventions.md)
  — managed-sidecar lifecycle and best-effort semantics.
- [concepts/piiscrub-usage.md](/concepts/piiscrub-usage.md) — caller-facing
  managed-mode contract.
- [superpowers/specs/2026-10-28-multilingual-ner-design.md](/superpowers/specs/2026-10-28-multilingual-ner-design.md)
  — ONNX design and Phase 1 licence block.
- [decisions/llm-second-pass-model.md](/decisions/llm-second-pass-model.md) —
  judge model and implementation status.
- [concepts/licensing-and-provenance.md](/concepts/licensing-and-provenance.md)
  — THIRD_PARTY.md commit status.
