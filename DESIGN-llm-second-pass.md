# Optional LLM second pass (design, 2026-11-05)

Status: proposal for review. No code written. Default behaviour must not change.

## Goal
Detect weak candidates that the regex recognizers deliberately skip, and confirm them with a small local LLM (about 0.8B parameters). First case: a MyKad written as 12 bare digits with no identity-card keyword (`ref 801214105423`). About 9% of bare 12-digit runs in the gretel and nemotron corpora pass the date check, so reporting them all adds false positives. The model decides only those few candidates.

## Decisions (mine, for you to override)
| # | Question | Options | Choice | Why |
|---|----------|---------|--------|-----|
| 1 | Role of the LLM | (a) scan whole text; (b) judge only candidates the regex already found | **(b)** | Exact spans stay deterministic. Cost scales with the number of weak candidates, not text length. |
| 2 | Switch | (a) on by default; (b) explicit opt-in option on the scrubber and sidecar | **(b)** | With it off, output is byte-identical to today. You asked for an optional flag. |
| 3 | Where the model runs | (a) in-process via cgo; (b) separate local HTTP server with an OpenAI-compatible API | **(b)** | Keeps the default build CGO-free, like the NER sidecar. The model can be swapped without a rebuild. |
| 4 | Privacy | (a) any endpoint; (b) loopback only unless explicitly allowed | **(b)** | The context window around a candidate is PII. Sending it to a remote API would defeat the library. |
| 5 | On LLM error or timeout | (a) fall back to regex-only; (b) return an error | **(b)** | Project rule: no silent fallbacks. A caller who enabled the pass must know it failed. |
| 6 | Output | free text vs constrained yes/no | **constrained yes/no, temperature 0** | Deterministic, cheap to parse, no hallucinated spans. |
| 7 | Model | not chosen | **open** | Must meet the same Apache-2.0/MIT bar as the NER checkpoint. I have not surveyed licences. |

## Flow (high level)
1. A recognizer marks a match as a weak candidate instead of dropping it. For MyKad: 12 bare digits, valid date, no keyword. It carries the entity type and a low confidence.
2. The analyzer, when the second pass is enabled, takes each weak candidate plus a short window of surrounding text and asks the judge: is this a Malaysian identity-card number?
3. Accepted candidates are promoted to a normal confidence and reported. Rejected candidates are dropped. Results are cached by candidate and window.
4. With the pass disabled, weak candidates are discarded before the min-confidence filter, so nothing changes.

## Files and functions to touch (names only)
- `recognizers/national/national.go`: `MYICRecognizer` emits weak candidates for the bare form.
- `recognizers/recognizers.go`: a marker on `Entity` for "weak, needs confirmation".
- `analyzer/analyzer.go`: a new option, plus the judging step before the confidence filter.
- new package, for example `recognizers/judge/`: HTTP client for the local LLM endpoint, the prompt, the yes/no parser, and the cache. It mirrors `recognizers/nlp/nlp.go` (timeout, logger, loopback check).
- `piiscrub.go`: a `ScrubberOption` that wires it in.
- `sidecar/`: only if you want the same switch on the HTTP service.
- `Makefile`: a target to run an evaluation against a running local model.
- `.env.example`: endpoint URL and model name variables, placeholders only.
- `TODO.md`, `SPEC.md`, `AGENTS.md`, and `docs/` through `okf:context-writer`.

## Evaluation before it can be recommended
- Hand-label bare MyKad positives and look-alike negatives (12-digit order numbers, timestamps, dummy runs) as a new golden set. CI can't run an LLM, so the set is scored by a separate make target, like `test-ner-onnx`.
- Report, on the corpora and the new set: false positives removed, true positives lost, latency per candidate, and run-to-run stability.
- The pass ships off by default. It is only suggested for use if the false-positive drop is large and recall loss is near zero.

## Risks
- A 0.8B model may not separate an IC number from other date-like numbers. The evaluation decides this, not this document.
- Added latency and a runtime dependency, but only for users who opt in.
- Prompt injection: the window of text is untrusted. The output is constrained to yes/no, and the model has no tools, so the worst case is a wrong yes/no.

## Phases
0. Review this design and pick the model and its licence. (blocked on you)
1. Weak-candidate marker and the disabled path, with a test that output is unchanged.
2. Judge client, prompt, cache and loopback check, tested against a fake HTTP server.
3. Evaluation set and make target, then real measurements.
4. Generalise to other weak candidates (bare Thai 13-digit, other bare IDs) only if phase 3 supports it.
