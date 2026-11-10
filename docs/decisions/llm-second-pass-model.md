---
type: Decision
title: LLM second pass — model choice (Qwen3-0.6B, Apache-2.0)
description: 'Resolves decision 7 of the optional LLM second-pass design: Qwen3-0.6B (Apache-2.0) is the judge model, with licence verification and the rejected alternatives. Phases 1 (weak-candidate marker) and 2 (judge client in recognizers/judge) are implemented.'
resource: DESIGN-llm-second-pass.md
tags:
  - llm
  - second-pass
  - model
  - licence
  - mykad
  - implementation
timestamp: 2026-11-09T10:04:41Z
---
# LLM second pass — model choice (Qwen3-0.6B, Apache-2.0)

## Context

The optional LLM second pass is opt-in code: it lets a small
local model confirm weak candidates that the regex recognizers deliberately skip
(first case: a MyKad written as 12 bare digits with no identity-card keyword).
The design lives in `DESIGN-llm-second-pass.md` at the repo root, which is not
part of this bundle. Its decision 7 ("Model") was the open item; it is now
resolved. Phases 1 and 2 are implemented, and the pass still ships off by
default with no change to default output.

## Decision

**Model: Qwen3-0.6B** (Hugging Face `Qwen/Qwen3-0.6B`), **licence Apache-2.0**.

- **Licence verification:** the HF model card declares `apache-2.0`, and the
  `QwenLM/Qwen` repository `LICENSE` file is the full Apache 2.0 text. Both were
  checked, not inferred from the model family.
- **Size:** ~0.8B parameters (0.6B total, 0.44B non-embedding).
- **Languages:** 119 languages and dialects per the Qwen3 technical report
  (arxiv 2505.09388), including Malay and Chinese.
- **Training-data provenance:** documented at arxiv 2505.09388.
- **Serving:** a separate local HTTP server with an OpenAI-compatible API
  (design decision 3b), bound to loopback only (decision 4b), behind an explicit
  opt-in (decision 2b). Keeping it out of process keeps the default build
  CGO-free, like the NER sidecar.

## Rejected alternatives

| Candidate | Why rejected |
|-----------|--------------|
| Qwen 3.7 | Closed weights only; no downloadable weights; proprietary. |
| Qwen 3.8 | The 27B variant is Apache-2.0 but too large; the 2.4T MoE variant uses a custom licence, not Apache-2.0. |
| Qwen 4 | "In training": no weights, no release date, no licence. |
| SmolLM2-1.7B | Apache-2.0, but only 6 European languages — no Malay or Chinese. |

## Implementation status

**Phase 1 — weak-candidate marker and the disabled path — is implemented
(2026-11-09). Phase 2 — judge client, prompt, cache and loopback check — is
implemented (2026-11-09).** The pass still ships off by default and default
output is byte-identical.

### Phase 1 — weak-candidate marker

- `recognizers/recognizers.go`: `Entity` gains a `Weak bool` field. Its zero
  value is `false`, so existing callers are unaffected.
- `recognizers/national/national.go`: `MYICRecognizer` gains a `bare`
  `*regexp.Regexp` (`\b\d{12}\b`) and `weakConfidence = 0.50`. A bare 12-digit
  run that passes `MYICValidator` (birth-date check), does not overlap a span
  already claimed by the keyed pattern, and has no adjacent digit is emitted as
  a weak candidate. Neighbour checks are explicit because RE2 has no lookaround.
  Helpers: `keyedSpans`, `overlapsAny`, `isDigit`.
- `analyzer/analyzer.go`: the `Judge` interface
  (`Judge(ctx, entityType, candidate, window string) (bool, error)`),
  `WithLLMJudge` / `SetLLMJudge` (construction-time only; setting it during
  `Analyze` is a data race), and the constants `keywordGateConfidence = 0.85`
  and `judgeWindowChars = 60`, plus `windowAround()`. The judge step runs
  before the confidence filter: a confirmed candidate is promoted to 0.85 and
  cleared of `Weak`; a rejected or errored candidate is dropped. Any weak
  candidate still unconfirmed after the step is dropped regardless of
  `minConfidence`. With no judge configured, weak candidates are dropped and
  output is unchanged.
- `piiscrub.go`: `WithLLMJudge` is a `ScrubberOption`. A nil judge means no
  judge; a typed nil is a programming error.
- Tests: four analyzer tests (weak dropped without a judge, confirmed promoted
  to 0.85, rejected dropped, judge error propagated) and five piiscrub tests
  (dropped without a judge, confirmed, rejected, dropped below the floor at
  `minConfidence = 0.50`, nil judge is a no-op); `national_test.go` covers a
  bare valid MyKad becoming weak and an invalid one producing nothing.

### Phase 2 — judge client (`recognizers/judge`)

`recognizers/judge/judge.go` (new package, stdlib-only) implements the
`analyzer.Judge` interface against a local OpenAI-compatible chat-completions
endpoint. It uses only `net/http` and `encoding/json`, so the root module stays
stdlib-only.

- `New(baseURL string, opts ...Option) (*Judge, error)` validates the URL and
  enforces loopback: only `127.0.0.1`, `localhost`, `::1` and the
  `127.0.0.0/8` range are accepted unless `PII_SCRUB_NLP_JUDGE_ALLOW_REMOTE=1`.
  The default `http.Client` sets `CheckRedirect` to return
  `http.ErrUseLastResponse`, so redirects are refused.
- `FromEnv() (*Judge, error)` reads `PII_SCRUB_NLP_JUDGE_URL` and returns
  `nil, nil` when it is unset or empty, so the pass is disabled by default.
  `PII_SCRUB_NLP_JUDGE_MODEL` sets the model name (default `qwen3-0.6b`).
- `Judge(ctx, entityType, candidate, window string) (bool, error)` POSTs to
  `{baseURL}/v1/chat/completions` and parses `choices[0].message.content` as
  `yes`/`no`, case-insensitively. The system prompt is "You are a precise PII
  judge. Answer only yes or no."; the user message names the entity type and
  quotes the candidate and its delimited context window. Qwen3 thinking blocks
  (`### Start of Thinking` / `### End of Thinking`) are stripped before parsing.
- Options: `WithClient`, `WithTimeout`, `WithLogger`, `WithModel`.
- Cache: bounded FIFO, in memory only, max 1000 entries, keyed by
  `model|entityType|candidate|window`; no persistence.
- Errors are always returned, never a silent `false`, so design decision 5 is
  honored; the analyzer drops a rejected or errored weak candidate.
- Tests: `recognizers/judge/judge_test.go` has 15 tests against an `httptest`
  fake server: yes/no, case-insensitive, think-tag stripping, unparseable
  answer, 500 error, cache hit, cache keyed by model, non-loopback rejection,
  loopback variants, `FromEnv` disabled and URL, and concurrency under `-race`.
- `.env.example` documents the three commented placeholders
  `PII_SCRUB_NLP_JUDGE_URL`, `PII_SCRUB_NLP_JUDGE_MODEL` and
  `PII_SCRUB_NLP_JUDGE_ALLOW_REMOTE`.
- Verified: `go build ./...`, `go vet ./...` and
  `go test -race -count=1 ./recognizers/judge/...` are clean, and gofmt is
  clean on the new files.

Design decisions 2b (explicit opt-in: no judge means weak candidates are
dropped) and 5 (no silent fallback: a judge error aborts `Analyze`) are honored.

## Where this is recorded

- `DESIGN-llm-second-pass.md` — decision 7 row updated to "Qwen3-0.6B, Apache-2.0"
  with the rejection notes.
- `SOURCES.md` — new "Model checkpoints" section with a Qwen3-0.6B row.
- `DATASET_LICENSES.md` — new Qwen3-0.6B row, Apache-2.0.

Those three files are repo-root sources, not bundle docs. This document is the
bundle's record of the decision; the log entry is dated 2026-11-09.
