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

- [ ] **Phone: an unseparated country code is not parsed.** `91 555 123 4567`
      (no `+`) is matched as `555 123 4567` — the national number only, since the
      bare `1`-country-code handling is US-specific and there is no generic
      "country code + national number" branch. With a `+` (`+91 555 123 4567`)
      the whole string matches. Left UNASSERTED in `TestPhoneRecognizer`: it is
      not clearly a false positive, and pinning it would be deciding a policy
      nobody has. Revisit if a seed case makes it matter.
- [ ] **`Anonymizer.Anonymize` does not validate rune boundaries.** Its bounds
      check is byte-based (`e.Start`/`e.End` within `len(result)`), so an entity
      whose offsets fall INSIDE a multi-byte rune would splice at a non-rune
      boundary and yield invalid UTF-8 — whatever the operator does. Unreachable
      today: every recognizer matches ASCII, so spans land on rune boundaries and
      the `Analyzer` only returns such spans. Recorded because the rune-aware
      `MaskOperator` (Fixed below) makes it tempting to assume the whole pipeline
      is rune-safe. A guard would reject a span whose start or end is not a UTF-8
      boundary.
- [x] **Overlap resolution property coverage — CLOSED.** `analyzer/overlap_test.go`
      exercises real recognizers over card-vs-phone, IBAN-vs-digit-patterns,
      bare-digits-vs-card and same-span collapse, and the `\b` regression is
      mutation-verified. `analyzer/property_test.go` adds `FuzzAnalyzeInvariants`
      (788k executions over 90s, no violations) plus corpus, determinism,
      never-panics and monotonicity tests. The other half of the byte-offset
      arithmetic — the operators and the Anonymizer's splice — is now covered:
      `operators/property_test.go` (`FuzzOperatorReplace`, 3.7M executions over
      30s, no violations) pins totality, determinism, the mask no-leak invariant,
      the mask rune-count identity and the hash length; `anonymizer/property_test.go`
      (`FuzzAnonymizeInvariants` 1.4M and `FuzzAnonymizeNeverPanics` 628k executions
      over 30s each, no violations) pins the left-to-right splice oracle for
      well-formed entities, the caller-slice-not-mutated rule, and no-panic /
      no-partial-result on malformed entity lists. Three mutants verified the new
      properties, each compiled before being counted and each caught by its
      intended test: leaking the input on a masked slot, dropping the trailing-slot
      fill, and inverting the splice direction. **This entry is closed; nothing
      remains outstanding.**
- [ ] **Four-part version strings are read as IPv4.** `neg-ip-version-four-part`
      ("Upgrade the agent to 3.10.2.7") has four valid octets, so the IP
      recognizer reports it. `neg-version-string` passes only because its
      version has three parts. **Decision needed:** (a) accept it, since a dotted
      quad is IP-shaped; (b) suppress it when a version word such as "version",
      "v" or "upgrade" precedes it, which is a heuristic and needs a sign-off; or
      (c) drop the IP recognizer's shape for values with small octets. Recorded,
      not fixed.
- [x] **Batch-1 notes in `seed.json` were stale or wrong.** Both rewritten to
      describe what the text tests; labels unchanged. `neg-luhn-failing-card`
      is clean (Luhn is wired in `NewScrubber`), confirmed by running the
      scrubber over every negative case: the only detection left is
      `neg-ip-version-four-part`. `pos-url-http` now describes a https URL with
      a path followed by prose, not a sentence period it never had.
- [ ] **Golden dataset is a 72-case seed set, not the ~500 SPEC.md calls for.**
      `testdata/golden/seed.json` and the `golden` runner now exist and are
      tested, so the ">99% precision/recall" claim is at last measurable, but
      the seed set is far too small to support that number. Batch 2 (13 cases)
      took the baseline off 1.0 on purpose. Batch 3 (13 cases) added the national
      IDs. Batch 4 (5 cases) added Malaysia passports. Batch 5 (4 cases) added a label-glued SG NRIC and keyed bare MyKad. Current: exact P=0.9787
      R=1.0000 (tp=46 fp=1 fn=0), overlap the same, 26 of 27 negatives clean. One of
      the two defects behind it is fixed (URL, now trimmed); the IP
      version-string defect remains and is its own entry above. Thresholds:
      `46.0 / 47.0` for exact precision, and a literal `1.0` for recall.
      Earlier history:
      the parenthesised-phone fix moved 12/13 to 13/13 on the 18-case seed, and
      wiring the checksum validators lifted exact P from 0.8571 and cleared all
      six negatives.
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
      ai4privacy. **Fetch is DONE:** `scripts/fetch-dataset.sh` (driven by
      `make download`) writes into the gitignored `testdata/external/` and never
      commits a corpus; both clean corpora are fetched. **The schema-aware
      converter and the corpus scoring mode are DONE** — see the closed entry
      below (`tools/corpusbench`, `make corpus`); the FP-inflation trap that
      lived in Open Questions was resolved by the schema sidecar, not worked
      around.
- [ ] **Context boosting accuracy still unmeasured.** The golden gate runs
      `NewScrubber`, which does NOT enable `WithContextBoost()`. Boosting is
      wired and tested for wiring only, and the gate above would not notice if
      boosting were enabled by default and degraded results. Needs a second gate
      scored with boosting on, compared against this one.
- [x] **`removeOverlapping`'s equal-confidence tie-break: intent vs test.** The
      behaviour is that on an exact tie in confidence the EARLIER span wins, and
      it is now pinned by `analyzer.TestEqualConfidenceTieIsDeterministic` at
      `analyzer/context_boost_test.go:336`. Both halves are covered — AGENTS.md
      §Confidence & Threshold Semantics documents it as a deliberate policy
      choice, not an accident of implementation. **This entry is closed;
      nothing remains outstanding.** Recorded here only so the earlier wording
      ("no test pins this") is not mistaken for a live gap — that claim was
      already stale when this line was first written, because AGENTS.md
      documented the policy and a later test pinned it.

- [ ] **BITCOIN recognizer has no checksum validator — shape-only.** Any
      base58-looking string matching P2PKH/P2SH or Bech32 shape is reported.
      Measured only on the 3 seed BITCOIN cases (P=1.0000); neither corpus
      labels BITCOIN, and gretel's 44 BITCOIN detections are excluded as
      out-of-schema rather than scored. Fix: add Base58Check and Bech32 checksum
      validators, then re-measure. Required by the no-silent-gap rule.
- [ ] **CREDIT_CARD recall is bounded by Luhn against synthetic data.** On
      nemotron, R=0.0945 with 230 FNs — and all 230 fail the Luhn check while
      exactly the 24 Luhn-valid ones are the true positives. The validator is
      working as designed (it is the same check that rejects a Luhn-failing seed
      negative). Open question, not a defect: is a corpus of non-Luhn-valid
      "card numbers" a fair recall target for a Luhn-gated detector? If not, the
      conversion should either exclude those gold labels or the metric should
      report them separately.
- [ ] **Corpus-side label quality depresses measured precision.** gretel EMAIL
      reads P=0.3281, but spot-checking shows the corpus labels the local part
      (`user_name 90..96 = 'lowe73'`) rather than the full address, markdown
      `[addr](mailto:addr)` renders each address twice while only one
      occurrence is labelled, and 106 of 647 email-shaped occurrences in
      email-bearing records carry no email label at all. A corpus miss looks
      exactly like our false positive. Before any threshold is recorded from
      these numbers, spot-check a sample of FPs by hand (spec §8 risk 4).
- [ ] **gretel PHONE precision (0.2772) is unexplained.** Unlike SSN and
      CREDIT_CARD, no hand-check has been done. Do not record a PHONE threshold
      until a sample has been reviewed.

## Closed in the golden-dataset pass

Neither of these is an open item; recorded so a later reader does not re-raise
them as gaps.

- Golden runner had no tests — `golden/golden_test.go` (18 tests) and
  `golden/accuracy_test.go` (3) cover it, 21 in the package. Mutation-verified:
  reverting the zero-denominator convention, the gold+pred type union, the
  ambiguous-label
  guard, the text-order check, the single-credit-per-gold array, or
  `DisallowUnknownFields` each fails a specific test. One rejected mutant was an
  EQUIVALENT mutant (the card recognizer's length window can never reject a
  match its own pattern produces), which no test could catch.
- `Case.Negative` was a declared-but-unread field — deleted. Negatives are now
  derived from an empty entity list, and the loader rejects unknown JSON fields,
  so a hand-written `"negative": true` fails loudly instead of being ignored.

## Fixed (kept as regression notes)

- [x] **Scorer counted detections on unlabelled spans as wrong, with no way to tell.** Fixed: the converter keeps the spans a corpus labels outside our type map (`Case.Unlabelled`, byte offsets, validated like labels). The scorer sets aside a false positive that lands on one and touches no mapped gold, and reports it as a bucket by "TYPE on source label". The headline is unchanged. Precedence: a true positive; any touch of mapped gold stays FP; an unlabelled span goes to the bucket; anything else is FP. Both matching policies use the same rule. The upper-bound precision is labelled as a bound, not accuracy. A corpus schema without `unlabelled_recorded` is refused, so a stale dataset can't report an empty bucket. Tests: `golden/unlabelled_test.go` (6 cases, including the precedence and stale-schema refusal, and a mutation that drops the touches-gold check fails two of them); `TestBuildUnlabelledKeepsOnlyUnmappedSpansInBytes` covers the multibyte byte-offset case. Evidence: `make corpus` re-run; headline numbers identical to before.
- [x] **URL regex kept a trailing sentence period.** Fixed in
      `recognizers/regex/regex.go`. `trimURL` drops trailing `.,;:!?*'` and
      backtick always, and a closing `)` or `]` only when the URL has no matching
      opener, so `Foo_(bar)` keeps its paren. A scheme left with nothing after it
      (`http://.`) is dropped. The trim set follows the corpus: 406 bare trailing
      `.`, 64 `,`, and a few `)`, `).`, `**.`, backtick. The doc comment states the
      policy. Tests: `TestURLRecognizerExactSpan` asserts exact `Text` per case,
      and was red before the fix. Mutation-checked: a single-pass trim fails the
      stacked-closer case, and no balance check fails the balanced-paren case.
      Evidence (`make corpus`, nemotron URL, exact span): P 0.3053→0.9900, R
      0.2846→0.9229, tp 214→694, fp 487→7. Overlap and every other type are
      unchanged. `pos-url-trailing-period` now passes.
- [x] **nemotron URL precision was a boundary problem.** Closed by the entry
      above: the disagreements were span boundaries, not missed detections, and
      most were a trailing `.` or `,`. The 58 URL exact misses (false negatives)
      are not analysed here; the tally covered only predictions, so they are
      gold URLs with no exact prediction, and remain unexplained.
- [x] **`countOverlappingPairs` undercounted spans overlapping three or more others.**
      Fixed in `tools/corpusbench/convert/labels.go`: a pairwise count replaces the
      furthest-end sweep. `TestCountOverlappingPairsCountsEveryPair` covers three
      mutually overlapping spans (3, was 2), touching spans (0) and identical
      spans (1). Re-converting both corpora leaves gretel `overlapping_pairs` at 9
      and nemotron at 0, so no corpus has a three-way overlap that moved a number.
- [x] **`countOverlappingPairs` panicked when no label had offsets.** The
      `seen[0]` index ran on an empty slice. It now returns an error naming the
      label, and the converter wraps it with corpus, file and row. Test:
      `TestCountOverlappingPairsRejectsMissingOffsets`. Not a silent skip, and no
      length guard.
- [x] **No committed test for the 500/501 chunk boundary.** Added
      `TestWriteChunkBoundaryKeepsEveryCaseInOrder`: 501 cases must write as 500
      plus 1, and `golden.Load` must return them in order with no loss or
      duplicates. Mutation-checked: dropping one case per chunk, a chunk index
      that never advances, and a missing final clamp each fail it.
- [x] **A `(0, nil)` read was treated as end of file.** `convert.go` now ends
      the scan only on `io.EOF` (rows returned with it are still converted), and
      refuses a zero-row read with no error, naming file and row. Test:
      `TestEndOfReadOnlyEOFEndsTheScan`. Mutation-checked: removing the guard
      fails it. Both real corpora still convert with unchanged totals.

- [x] **SSN matches any bare nine-digit run. FIXED.** `regex.go` now requires
      a separator at both positions in prose, and accepts a bare nine-digit
      value only when it is the whole (trimmed) input, so `CAS No.: 123456789`,
      `Account Number: 123456789` and `Passport Number: 322996077` no longer
      match. Evidence (`make corpus`, 2000 records each): gretel SSN P 0.0929 →
      0.9787 (FP 449 → 1); nemotron SSN P 0.3495 → 0.8780; recall unchanged
      (gretel 0.8846, nemotron 1.0000). Regression rows in `regex_test.go`.
- **`convert.Write` left stale `cases-*.json` in a reused output directory.**
  Found by the final whole-branch review. Reusing an output directory is normal
  — vary `LIMIT`, re-fetch a corpus revision — and a chunk left behind by an
  earlier, larger conversion survived the rewrite: `golden.Load` then read more
  cases than `schema.json` declared (500 beside 5,596 loaded records), so the
  benchmark header understated the sample it actually scored and a sampled
  report could be mistaken for a full-corpus one — the guarantee the sidecar
  exists to provide. **Fixed** by globbing and removing `cases-*.json` after
  `MkdirAll` and before the write loop, so `Write` emits exactly the sequence
  `schema.json` describes.
  **Regression test:** `convert.TestWriteReplacesStaleCaseFiles` (pre-populated
  directory → loads exactly the written cases and the stale file is gone).
  Mutation-verified: removing the clear compiles and fails it with
  `golden.Load returned 2 cases, want 1`.

- **`MaskOperator` split multi-byte UTF-8 runes (byte-wise masking).**
  Found by the new `operators/property_test.go`. `applyMask` iterated BYTES and
  treated every non-separator byte as one slot, so `é` with `#*` produced
  `"\xC3*"` — invalid UTF-8 — and `#` could keep a leading byte while `*` masked
  the continuation byte; the empty pattern emitted one `*` per byte. **Fixed** by
  making masking rune-based: `applyMask` iterates the input with
  `utf8.DecodeRuneInString`, the pattern with `[]rune`, and the empty pattern
  emits one `*` per rune. **Invalid UTF-8 is kept byte-faithfully** — an invalid
  byte is one significant unit and, on a `#` slot, is written back verbatim
  rather than replaced with U+FFFD (a scrubber must not rewrite bytes it was told
  to keep). Every ASCII case is byte-for-byte unchanged.
  **Regression tests:** `operators.TestMaskDoesNotSplitUTF8` (é/☃/日本語 keep+mask
  combinations, asserts exact output AND valid UTF-8) plus the re-derived
  rune-based `TestMaskOutputLength`, `TestMaskEmptyPatternMasksEverything`,
  `TestMaskDoesNotLeakInputData` and `TestMaskIsIndependentOfMaskedInput`; the
  fuzz target now asserts mask output of valid-UTF-8 input is valid UTF-8.
  Mutation-verified: reverting the loop to bytes, or the empty pattern to
  `len(s)`, each fails the tests. Documented in SPEC.md §Operators.
- **`Anonymizer.Anonymize` silently corrupted OVERLAPPING entity spans.**
  Found by the new `anonymizer/property_test.go`. Overlapping spans cannot be
  spliced independently — replacing the rightward one leaves the leftward one's
  offsets stale — and the old bounds check ran against the already-shrinking
  working string, so `[2,7)` + `[5,9)` over `0123456789` returned the corrupt
  `01<X>>9` with NO error. In a PII scrubber that can emit partly-unredacted
  text. **Fixed** by rejecting overlaps up front (before any splice), so a
  rejected input never yields a partial result: in the descending sort the next
  entity overlaps iff it ends after the previous start. Adjacent spans and an
  empty span at a boundary are correctly still accepted.
  **Regression test:** `anonymizer.TestAnonymizeRejectsOverlappingSpans`
  (rejects the overlap, accepts adjacent + empty-at-boundary). Mutation-verified
  by reverting the guard. The property tests' well-formed decoder drops
  overlaps, so the oracle never sees one — the explicit test is what pins this.
- **Phone recognizer dropped the leading `(` (and `+`) on anchored numbers.**
  The pattern was `\b(?:\+?1[\s.\-]?)?\(?\d{3}\)?...|\b\+\d{1,3}...`. A `\b`
  cannot match between a space (or the start of the text) and a non-word
  character, and both `(` and `+` are non-word, so the optional leading
  delimiters never participated: `(555) 867-5309` was reported as
  `555) 867-5309`, `+1-555-123-4567` as `1-555-123-4567`, and `+44 20 7946 0958`
  at the start of a string was not matched at all. **Fixed** by splitting the
  alternation into three branches and dropping the leading `\b` from the two
  whose first meaningful character is a delimiter — the parenthesised branch
  (bounded by `(`) and the international `+` branch (bounded by `+`) — while the
  bare-US branch, which starts with a digit, keeps its `\b`. A digit run cannot
  contain `(` or `+`, so those two branches need no left anchor; the bare branch
  still needs one or a 16-digit card is read as a phone. The three-branch shape
  is required: Go's RE2 has no lookbehind, so "not preceded by a word character"
  cannot be expressed as a single anchor that also admits `(`/`+`.
  **Scope note — the `+` half was not in the original entry.** The entry named
  only the dropped `(`; the `+` was the identical bug in the same anchor, so
  fixing one and not the other would have knowingly shipped half a fix.
  **Two follow-on guards the anchorless branches needed:**
  (a) The international branch now REQUIRES a separator after the country code
  (`\+\d{1,3}[\s.\-]...`). Anchorless and with an optional separator it matched
  any `+` followed by four or more digits, so at a string start or after a space
  `+12345`, `+4111111111111111` (a card with a `+`) and arithmetic like
  `1+23456789` were read as phones. The old leading `\b` had hidden this; the
  new branch has to earn its own precision. Cost: an unseparated `+442079460958`
  is not matched, which is the same as the old behaviour at a boundary.
  (b) The parenthesised branch's bare `1` country code carries its own `\b`
  (`\b1`), so `x1 (555) 867-5309` does not treat a glued `1` as a country code
  (it still matches the `(555) 867-5309`).
  **Consequence beyond recall:** `Scrubber.Anonymize` masked the wrong span,
  leaving the `(` and corrupting the digit run.
  **Found by** seed case `pos-phone-parenthesised` — an exact-match miss that
  overlap matching still credits, which is exactly the diagnostic gap the two
  granularities are reported to expose.
  **Regression tests:** `recognizers/regex.TestPhoneRecognizer` now asserts the
  exact span (not just the hit count) for `(555) 867-5309` at the start, in
  prose, without a space, with a `1` and with a `+1` country code, for
  `+1-555-123-4567` and `+44 20 7946 0958`, and for `x1 (555) 867-5309`
  (span `(555) 867-5309`); and asserts zero matches for `+12345`, `1+23456789`,
  `+4111111111111111`, a bare 16-digit card, a spaced card and `x555-123-4567`.
  The count-only table it replaced is precisely why the defect shipped — the
  recognizer reported one PHONE, one byte to the right.
  **Mutation-verified (each mutant compiled before being counted):** restoring
  the leading `\b` fails 7 subtests; removing the required separator from the
  international branch fails the three `+`-guard subtests; removing the `\b`
  from the bare `1` fails the `x1` guard. Fixing it lifted the golden exact
  baseline from 12/13 to 13/13 (see the golden item above).
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
  recognizer stays exported for callers who want shape-only detection.
  **No coverage was lost:** `checksum.LuhnValidator` produces byte-identical
  spans to the regex recognizer for every valid format tested — spaced,
  hyphenated, dotted, bare, mixed separators, Amex 15-digit, and the 16-digit
  Visa/Mastercard/Discover/JCB test values. The only behavioural differences are
  the intended ones: a Luhn-failing number and a 17-digit run are now rejected
  where the regex matched them.
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
- `findCardCandidates` rewinds trailing separators, so a card does not absorb the
  prose that follows it. Mutation-verified. The `findIBANCandidates` half once
  recorded here — "a four-character block after each space" — was ITSELF the bug:
  requiring exactly four dropped the short closing block of every conventionally
  spaced IBAN, so a valid number was skipped in silence. The block is now 1 to 4
  characters (`checksum.go:219-225`); see the spaced-IBAN entry above.
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
      `testdata/external/` (override with `DATAPROTECT_DATA_DIR`). Both clean
      corpora are fetched and on disk; the restricted three are skipped.
- [x] **Corpus converter and scoring mode — DONE (2026-10-08).** `tools/corpusbench`
      (its own Go module) converts both corpora into `golden.Case` files plus a
      `schema.json` sidecar; `scripts/benchmark.sh` applies that sidecar
      automatically, so detections of a type a corpus never labels are reported
      as out-of-schema instead of counted as false positives — the FP-inflation
      trap recorded below, resolved rather than worked around. `make corpus`
      converts and scores both; `make corpus-clean` removes the output (derived
      data stays in gitignored `testdata/external/`, see DATASET_LICENSES.md).
      Design: `docs/superpowers/specs/2026-10-08-corpus-converter-design.md`.

      **Trap (2026-10-06), for the record:** `scorePair` (`golden/golden.go`)
      counts a prediction as FP unless it matches a gold label of the SAME type,
      so a corpus labelling only PERSON/ORG/LOC scores a correct EMAIL detection
      as a false positive. The fix is the global-schema filter, not per-case
      filtering: a per-case schema would let the same detected type be credited
      in one record and excluded in another, making precision non-comparable
      across a dataset.

## Open findings from the first corpus measurement

These findings were invisible to the seed set. Numbers and evidence are in
SPEC.md § "Corpus measurement (2026-10-09)"; reproduce with `make corpus`.

- [ ] **Passports: Malaysia done, Singapore and Thailand blocked.** Other Asian countries are not started.
      - **Malaysia (done, 2026-10-09; user accepted the secondary sources):** `MY_PASSPORT` matches `[AHK]\d{8}` only after a keyword from Purview's list (passport, pasport, nombor pasport, passport no, passport number, and so on). The shape is never reported alone, because it is too short to be safe. Sources: Wikipedia and Microsoft Purview. The EU PRADO record (`mys`) is the primary source and is unverified: PRADO returned 403. The old and 30 June 2026 booklets are not split.
      - **Singapore (blocked):** Purview gives E or K, seven digits, one letter. A National Archives of Singapore 2006 record, per a search summary (the page fetch came back empty, so unverified), says the biometric passport number begins with E, has seven digits, and ends in a letter. If confirmed, it is the only government source, and it is about 20 years old. The Sumsub sample `E93874729W` has eight digits and conflicts. The ICA 2025 reply does not state the format. PRADO (`sgp`) is unreachable. Decision needed: pick the format and accept its source, or supply the PRADO field text.
      - **Thailand (blocked):** no source states the passport-number format. The ICAO Doc 9303 field is nine characters, but that is the machine-readable zone, not a number written in prose. The one sample (`B100990`, one letter then six digits) comes from a data-extraction tool, so it is not evidence. PRADO (`THA-AO-06001`) is a document-type code, not a number. Decision needed: a specimen or a primary source.
      - **No checksum route for prose.** The ICAO check digit exists only in the MRZ line. A passport number written in a sentence has no checksum, so the keyword gate is the only defence, as in Malaysia.
      - **Known limits of the Malaysia rule (2026-10-09):** (1) Words between the keyword and the number are not allowed, so `Passport Number of the Primary Driver: H26965339` in gretel is missed (the Purview proximity is 300 characters; we use none). (2) `structured/` analyzes each CSV cell alone, so a `passport_number` column header never reaches the analyzer; a passport column is detected only when its cell carries the keyword. (3) The type name says Malaysia, but the rule checks keyword and A/H/K shape only. Other countries use one letter and eight digits too, so a hit is a passport-like number, not a verified Malaysian one. (4) Real-text signal: gretel has one hit, `Passport number: K13198743`, out of schema (gretel has no passport label), so it is not scored either way; nemotron has none.
      - **PRADO attempts (2026-10-09):** WebFetch returned 403. The htrcli browser route failed (content-script injection error), `--cdp` fetch is unsupported, and the claude-in-chrome extension was not connected. Retry from a browser session the user opens.
- [ ] **National ID coverage gaps.** (a) Singapore M-prefix is unsupported: its check-letter table is unconfirmed. (b) Malaysia MyKad place-of-birth codes are not validated: the code list is unconfirmed, and the MyKad check-style digit is not validated either. (c) Thailand bare 13-digit form has about a 1-in-10 false-match rate on random digits, so it needs a measured FP rate on labelled text before trust. (d) Batch 3 added synthetic golden cases for them (4 SG, 2 MY, 2 TH positives; 5 negatives). The corpora don't label these types, so no recognizer is measured on real text.
- [ ] **National-ID gaps from the earlier national-ID commit (9594065), fail-open.** A background security review flagged the national-ID recognizers as fail-open. Its finding bodies never arrived, so the items below are what my probes through `Scrubber.Anonymize` found, not the review's wording. Fixed: a label-glued SG NRIC (`NRICS1234567D`) is found, only when the NRIC or FIN label comes before it. The earlier fix dropped the leading `\b` for every NRIC, so a 0.99 NRIC inside a shape-only 0.95 bitcoin token (`1Ab3CdEf5Gh7Jk9Mn2Pq4RsS1234567D`) won the overlap and left 23 characters visible. The review flagged this as an overlap displacement. The bare-token case is now back to one full BITCOIN span, and a differential over 4072 seed and corpus texts shows no lost detections. Fixed: bare 12-digit MyKad is found only after an identity-card keyword (Purview's list; date check kept). Still open, strict by choice: a Thai ID glued to a letter (`ID3100600445635`) is not redacted, because relaxing `\b` would also redact order and reference numbers such as `ORD3100600445635`. Decision needed: accept that as documented scope, or pay the false-positive cost.
- [ ] **Valid Thai citizen ID that passes Luhn is reported as CREDIT_CARD.** `1000000000009` has a valid mod-11 check digit and also passes Luhn. The card recognizer (0.99) beats TH_ID (0.95), so the number is typed as CREDIT_CARD and masked as payment data. Probed: both `1000000000009` and `1-0000-00000-00-9` give CREDIT_CARD. Decision needed: which type should win when a valid Thai ID also passes Luhn. No golden case is written for it, because labelling it TH_ID would settle that decision.
- [ ] **Phone pattern matches ID-number digit runs (nemotron).** Of the 419
      nemotron PHONE false positives, 408 sit on spans the corpus labels as
      ID fields: `biometric_identifier` 117, `customer_id` 83,
      `account_number` 79, `medical_record_number` 76,
      `health_plan_beneficiary_number` 34, and smaller counts. Likely cause:
      the bare 10-digit branch of the phone pattern matches any 10-digit run.
      Not yet confirmed by hand. CREDIT_CARD has a similar pattern: 18 FPs,
      9 `device_identifier` and 9 `account_number`. Next step: hand-check a
      sample of the phone FPs, then decide whether the phone branch needs
      context or a stronger shape rule. This is a detector fix, not a scope
      question.
- [ ] **SSN residual false positives are a label-scope decision, not a regex defect.**
      Hand-checked by bucket over the same 2000-row samples `make corpus` scores
      (scratch analysis, not committed). **Nemotron: 25 SSN FP**, every one at a
      span the source labels with an unmapped type: 23 `tax_id`, 1
      `certificate_license_number`, 1 `medical_record_number`. All 25 are dashed,
      in tax-ID wording. **Gretel: 1 SSN FP**, dashed, labelled by nothing.
      Figures below use the overlap policy, so they approximate the exact-span
      report. **Decision needed:**
      - **(a) keep as FPs.** No change. Nemotron SSN stays P=0.8780.
      - **(b) map `tax_id` to SSN.** Nemotron has 31 `tax_id` spans in the sample:
        23 are dashed 3-2-4 (today's FPs, which would become TPs), and 8 are
        EIN-shaped (`99-9999999`, 5) or `P`-prefixed (3), which the SSN pattern
        cannot match, so they become FNs. Estimate SSN P≈0.99 (203/205), R≈0.96
        (203/211). This labels US tax IDs as SSNs, which is a claim about the
        corpus, not a detector fix.
      - **(c) exclude unmapped-label spans, as out-of-schema types already are.**
        This changes every type. Nemotron PHONE P 0.5703→0.9806 (408 of its 419
        FPs sit on unmapped spans), CREDIT_CARD P 0.5714→1.0 (all 18 FPs), SSN
        0.8780→1.0. Gretel CREDIT_CARD 0.5094→0.5870, EMAIL 0.3702→0.3768 (the
        `user_name` case is only 10 detections, so the effect is small), PHONE
        unchanged. The nemotron PHONE result means the current PHONE precision
        figure is mostly a scope artefact, which SPEC does not yet say.
      Option (c) moves every corpus number, so it is not applied. Until one of
      (a)-(c) is chosen, record no SSN precision target.
- [ ] **BITCOIN coverage.** Neither corpus labels it, so it stays unmeasured.
      Needs a third corpus or a hand-authored seed extension.
- [ ] **Per-language scoring of gretel.** One conversion uses `combined_test`;
      scoring `English_test`, `German_test` etc. separately would show whether
      the recognizers are English-shaped.
- [ ] **Corpus results as a CI gate.** Blocked by design: the corpora and their
      converted output are gitignored, so CI cannot see them. The committed gate
      remains the seed set.
- [ ] **An end-to-end anonymize check over corpus cases.** Would require
      resolving gold overlaps first, and `Anonymize` rejects overlapping
      entities by contract (SPEC.md §Operators).

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
  written and mutation-verified, then removed on 2026-10-05 because at that time
  there was no Makefile, CI or hook — it would never have run, and an unrun check
  reads as protection that does not actually exist. A `Makefile` now exists
  (`make download`, commit 3541169) but carries no docs-sync target, so the
  detector would still never execute; the removal stands.
- [ ] **Re-syncing is now a manual obligation.** When `SPEC.md`, `README.md` or
      `AGENTS.md` changes, re-copy the body into the matching `docs/` file and
      re-run `doc_write`. Re-apply all four link rewrites above when syncing
      `docs/concepts/piiscrub-usage.md`, or its links dangle again. Nothing
      enforces this, which is the accepted cost of keeping the copies.