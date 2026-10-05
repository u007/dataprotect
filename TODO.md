# TODO

## v2+ (after MVP)

- [ ] NLP recognizer (prose-based NER for person/org/location)
- [ ] Secret recognizers (API keys, tokens, passwords, private keys, entropy-based)
- [ ] Format-preserving encryption (FPE) operator
- [ ] Image PII detection (facial recognition via go-face)
- [ ] REST API sidecar
- [ ] Multi-language support
- [ ] Benchmark suite (OpenNER, Financial PII, Kaggle PII, CoNLL, ai4privacy, PII-Bench, SecretBench)

## Known gaps in the current MVP

- [ ] **`checksum.LuhnValidator` and `checksum.IBANValidator` are never registered.**
      `NewScrubber` (piiscrub.go) adds only the eight `recognizers/regex`
      recognizers. The Luhn validator is therefore dead code in the default
      pipeline: `regex.CreditCardRecognizer` accepts any 13-19 digit run and
      scores it 0.95, with no checksum. Found by the seed dataset — case
      `neg-luhn-failing-card` labels `4111 1111 1111 1112` (verified Luhn-failing)
      as a negative, and the detector reports it as a CREDIT_CARD false positive.
      The `len(card) >= 13 && len(card) <= 19` check in that recognizer is
      unreachable-as-a-filter: the pattern can only ever match 15- or 16-digit
      runs, verified exhaustively, so widening the window to 6 is an
      EQUIVALENT MUTANT that no test can catch. Registering the validators is
      the real fix; the length check is not.
- [ ] **Phone recognizer drops a leading `(` on parenthesised area codes.**
      The pattern is `\b(?:\+?1[\s.\-]?)?\(?\d{3}\)?...`. A `\b` cannot match
      between a space and a non-word character, so with no preceding word
      character the optional `\(` never participates: `(555) 867-5309` is
      reported as `555) 867-5309`. Found by seed case
      `pos-phone-parenthesised` — an exact-match miss that overlap matching
      still credits, which is exactly the diagnostic gap the two granularities
      are reported to expose. Consequence beyond recall: `Scrubber.Anonymize`
      masks the wrong span, leaving the `(` and corrupting the digit run.
- [ ] **Overlap resolution has no general property test.** `analyzer/overlap_test.go`
      now exercises real recognizers over card-vs-phone, IBAN-vs-digit-patterns,
      bare-digits-vs-card and same-span collapse, and the `\b` regression is
      mutation-verified. `analyzer/property_test.go` adds `FuzzAnalyzeInvariants`
      (788k executions over 90s, no violations) plus corpus, determinism,
      never-panics and monotonicity tests. **Remaining:** no fuzz target exists for
      the `operators` package or for `Anonymizer.Anonymize`, which is the other
      half of the byte-offset arithmetic.
- [ ] **Golden dataset is an 18-case seed set, not the ~500 SPEC.md calls for.**
      `testdata/golden/seed.json` and the `golden` runner now exist and are
      tested, so the ">99% precision/recall" claim is at last measurable, but
      the seed set is far too small to support that number. Measured baseline is
      exact P=0.8571 R=0.9231 (tp=12 fp=2 fn=1) and overlap P=0.9286 R=1.0000
      (tp=13 fp=1 fn=0) over 18 cases — recorded in `golden/accuracy_test.go`
      as the gate thresholds, expressed as tp/total ratios so a rounded literal
      cannot drift above the true value. Two real detector defects are already
      reflected in those numbers (see the Luhn and parenthesised-phone entries);
      fixing them raises the baseline instead of breaking the gate.
      **Remaining:** grow toward ~500 cases and re-tighten the thresholds each
      time; the real corpora are the intended source. **Licence review is DONE —
      see DATASET_LICENSES.md.** It corrected a wrong claim here: ai4privacy is
      NOT CC-BY-SA, it is a custom dual-tier licence (free for individuals,
      for-profits of ≤3 staff, non-profits, or evaluation; larger profit-oriented
      organisations must buy a corporate licence, and derivatives may not be
      sublicensed). ai4privacy should therefore be AVOIDED for anything
      commercial. Start instead with `nvidia/Nemotron-PII` (CC-BY 4.0) and
      `gretelai/synthetic_pii_finance_multilingual` (Apache-2.0), both clean
      synthetic corpora. Treat PIIBench's `apache-2.0` tag as unreliable — its own
      card says constituent licences govern, and it includes CoNLL-2003 and
      ai4privacy. Still to do: fetch scripts that download to an ignored
      directory, and never commit a corpus.
- [ ] **Context boosting accuracy still unmeasured.** The golden gate runs
      `NewScrubber`, which does NOT enable `WithContextBoost()`. Boosting is
      wired and tested for wiring only, and the gate above would not notice if
      boosting were enabled by default and degraded results. Needs a second gate
      scored with boosting on, compared against this one.
- [ ] **`removeOverlapping`'s equal-confidence tie-break: intent vs test.** The
      behaviour is that on an exact tie in confidence the EARLIER span wins, and
      it is now pinned by `analyzer.TestEqualConfidenceTieIsDeterministic` at
      `analyzer/context_boost_test.go:336`. Both halves are covered — AGENTS.md
      §Confidence & Threshold Semantics documents it as a deliberate policy
      choice, not an accident of implementation. **This entry is closed;
      nothing remains outstanding.** Recorded here only so the earlier wording
      ("no test pins this") is not mistaken for a live gap — that claim was
      already stale when this line was first written, because AGENTS.md
      documented the policy and a later test pinned it.

## Closed in the golden-dataset pass

Neither of these is an open item; recorded so a later reader does not re-raise
them as gaps.

- Golden runner had no tests — `golden/golden_test.go` (21 tests) plus
  `golden/accuracy_test.go` cover it. Mutation-verified: reverting the
  zero-denominator convention, the gold+pred type union, the ambiguous-label
  guard, the text-order check, the single-credit-per-gold array, or
  `DisallowUnknownFields` each fails a specific test. One rejected mutant was an
  EQUIVALENT mutant (the card recognizer's length window can never reject a
  match its own pattern produces), which no test could catch.
- `Case.Negative` was a declared-but-unread field — deleted. Negatives are now
  derived from an empty entity list, and the loader rejects unknown JSON fields,
  so a hand-written `"negative": true` fails loudly instead of being ignored.

## Fixed (kept as regression notes)

- The `Operator` interface changed from `Operate(ctx, text, entities) (string,
  error)` to `Replace(ctx, value) (string, error)`. Operators no longer splice
  text; `Anonymizer` owns splicing and rewrites right-to-left. Grouping entities
  into a map by type replayed original offsets in random map order and corrupted
  the output. Mutation-verified.
- `RegexPhoneRecognizer` needed `\b` anchors: unanchored, it matched the first ten
  digits of any longer run, so a 16-digit card was reported as a phone number and
  lost. Mutation-verified.
- `findCardCandidates` rewinds trailing separators; `findIBANCandidates` requires a
  four-character block after each space. Without either, valid values absorbed the
  surrounding prose and failed their checksum. Mutation-verified.
- `ProcessCSV` returns `(raggedCount, error)` and pads short rows to the header
  width instead of silently accepting them. Mutation-verified.

## Open Questions

- [ ] FPE library evaluation (capitalone/fpe vs alternatives)
- [ ] go-face model bundling strategy (download on first use vs bundle)
- [ ] Module path is `dataprotect`, which is not a valid import path for
      third-party consumers (`go get dataprotect` cannot resolve it). Rename to a
      domain-qualified path before publishing.
- [x] **LICENSE: MIT.** `LICENSE` exists with `Copyright (c) 2026 Mercstudio`.
      Chosen over Apache-2.0 because the module has zero dependencies, so
      Apache's patent grant buys less here, and MIT is the most common licence in
      the Go ecosystem, which lowers the friction for outside contributions.
      The datasets measured against are explicitly NOT covered by it — see
      DATASET_LICENSES.md, which is the record of their separate terms.
- [x] **Fetch scripts for the benchmark corpora.** `scripts/fetch-dataset.sh`
      exists: `list`, `info <name>`, `fetch <name> [--dry-run]`, `fetch-all`.
      It pins an explicit commit SHA per dataset (a floating `main` would let the
      corpus change under a published number), records `PROVENANCE.txt` on
      success, aborts on a partial download, and **refuses to download into any
      path git would track** — that guard is what keeps dataset terms out of this
      repository. `fetch-all` skips the restricted corpora. Land in
      `testdata/external/` (override with `DATAPROTECT_DATA_DIR`).
      **Remaining:** a converter from each corpus's schema to `golden.Case`, and
      a scoring mode that reads the corpus instead of `testdata/golden/`.

## Docs bundle follow-ups

Both items below are resolved; recorded here so a later reader does not re-raise
them as gaps.

- **Fixed: two relative links dangled inside `docs/`.**
  `docs/concepts/piiscrub-usage.md` linked `[SPEC.md](SPEC.md)` and
  `[AGENTS.md](AGENTS.md)`, which resolve inside `docs/concepts/` where neither
  file exists. They now point at `[spec-v1](../decisions/spec-v1.md)` and
  `[development-conventions](development-conventions.md)`. Those two rewrites are
  the ONLY intentional body differences between the bundle copies and their root
  sources.
- **Fixed: root/bundle drift is now detected.** `scripts/check-docs-sync.sh`
  compares each bundle body (frontmatter stripped) against its root source and
  exits non-zero on drift. It applies the two known link rewrites to the root
  text before comparing, so those are not reported as false positives.
  Read-only: re-syncing stays a deliberate act — re-copy the body, then re-run
  `doc_write`. Mutation-verified: appending a line to a bundle body makes it exit
  1 and print the diff.
- [ ] **Not yet wired into automation.** This repo has no Makefile or CI, so
      `scripts/check-docs-sync.sh` only runs when invoked by hand. Add it to CI
      or a pre-commit hook if either is introduced; until then drift is
      detectable but not automatically caught.