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
      exact P=0.9231 R=0.9231 (tp=12 fp=1 fn=1) and overlap P=1.0000 R=1.0000
      (tp=13 fp=0 fn=0) over 18 cases — recorded in `golden/accuracy_test.go`
      as the gate thresholds, expressed as tp/total ratios so a rounded literal
      cannot drift above the true value. Every remaining non-perfect number is
      the parenthesised-phone defect (the one exact FN and the one exact FP), so
      fixing that raises the baseline instead of breaking the gate. Wiring the
      checksum validators already lifted this from P=0.8571 and made all six
      negative cases clean.
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

- **`checksum.LuhnValidator` and `checksum.IBANValidator` were never registered.**
  `NewScrubber` wired only the eight `recognizers/regex` recognizers, leaving
  both validators dead code, so cards were matched on shape alone at 0.95 with
  no checksum. Found by the seed dataset: `neg-luhn-failing-card` labels
  `4111 1111 1111 1112` (verified Luhn-failing) as a negative and the detector
  reported it as a CREDIT_CARD false positive. **Fixed** by wiring the
  validators into the default pipeline and dropping `regex.CreditCardRecognizer`
  from it — wiring both cannot work, because for a Luhn-FAILING number the
  validator emits nothing while the regex recognizer still matches the shape,
  so the false positive survives with nothing to overlap it away. The regex
  recognizer remains exported for callers who want shape-only detection.
  CREDIT_CARD went from P=0.7500 to P=1.0000; all six negative cases are now
  clean. Mutation-verified: unwiring Luhn, or restoring the regex card
  recognizer, each fails `TestNewScrubberIncludesChecksumValidators` or
  `TestScrubberRejectsLuhnFailingCard`.
  Note: `len(card) >= 13 && len(card) <= 19` in that recognizer was
  unreachable-as-a-filter — the pattern can only ever match 15- or 16-digit runs,
  verified exhaustively — so widening that window is an EQUIVALENT MUTANT no
  test can catch. It was never the real fix.
- **`checksum.IBANValidator` silently dropped every spaced IBAN.** Found while
  wiring the entry above, not by the golden dataset. `findIBANCandidates`
  required exactly four alphanumerics after each space, but an IBAN is at most
  34 significant characters and 34 is not a multiple of 4, so the closing block
  of `GB82 WEST 1234 5698 7654 32` is 2 characters and the final space was
  always rejected. The candidate stopped one character short, the truncated
  string failed mod-97, and a valid IBAN vanished with no error — passed through
  unredacted. Every existing test used the unspaced form, which is why it
  shipped. **Fixed** by accepting a 1-to-4 character block and by rejecting
  blocks containing lowercase letters, which is what separates an internal space
  from a space before prose. Mutation-verified both halves.

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

Recorded here so a later reader does not re-raise these as gaps.

- **Fixed: two relative links dangled inside `docs/`.**
  `docs/concepts/piiscrub-usage.md` linked `[SPEC.md](SPEC.md)` and
  `[AGENTS.md](AGENTS.md)`, which resolve inside `docs/concepts/` where neither
  file exists. They now point at `[spec-v1](../decisions/spec-v1.md)` and
  `[development-conventions](development-conventions.md)`. Two later passes added
  the same fix for two more: `[LICENSE](LICENSE)` becomes `[licence](../../LICENSE)`
  and `[DATASET_LICENSES.md](DATASET_LICENSES.md)` becomes `[dataset
  licences](../../DATASET_LICENSES.md)`. These four rewrites are the ONLY
  intentional body differences between the bundle copy and its root source; every
  other body byte matches `README.md` exactly.
- **Deliberately unmonitored: root/bundle drift.** `docs/decisions/spec-v1.md` and
  `docs/concepts/piiscrub-usage.md` are copies of `SPEC.md` and `README.md`, and
  `docs/concepts/development-conventions.md` is a copy of `AGENTS.md`, so all three
  can go stale. Drift already happened once: a concurrent session rewrote both
  roots minutes after they were copied, leaving the bundle describing a package
  layout the spec says never existed. A `scripts/check-docs-sync.sh` detector was
  written and mutation-verified, then removed on 2026-10-05 because this repo has
  no Makefile, CI or hook — it would never have run, and an unrun check reads as
  protection that does not actually exist.
- [ ] **Re-syncing is now a manual obligation.** When `SPEC.md`, `README.md` or
      `AGENTS.md` changes, re-copy the body into the matching `docs/` file and
      re-run `doc_write`. Re-apply all four link rewrites above when syncing
      `docs/concepts/piiscrub-usage.md`, or its links dangle again. Nothing
      enforces this, which is the accepted cost of keeping the copies.