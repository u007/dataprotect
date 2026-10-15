# TODO

## Pseudonymize follow-ups (2026-10-15)

- [ ] **`structured/` (CSV) has no pseudonymize path.** `Scrubber.Pseudonymize` covers free text only. A CSV column is analysed one cell at a time, so a repeated value gets one placeholder per cell only if the same cell text repeats. Design the column-level numbering before adding it.
- [ ] **Pseudonym equality is exact string.** `4111 1111 1111 1111` and `4111111111111111`, or the same email in different case, get different placeholders. Normalising the value for equality is a separate decision; it changes which values count as the same person.
- [ ] **Merged card spans are one placeholder.** A span merged by the card window search (see the review response) can cover two cards or a card plus a year. Its placeholder then restores both values as one original. Revisit with the merge decision.

## Review response (2026-10-15)

A background security review of commit `ff3c08b` reported two findings in `recognizers/checksum/checksum.go`. The finding text never arrived, so the shapes below are what I confirmed by probing, not the reviewer's wording.

- [x] **Algorithmic-complexity DoS (confirmed, fixed).** The card window search tried every later group for each start group, and recounted digits and grouping on each window. A run of `1 ` repeated 500000 times did not finish in 5 seconds. Fixed: each start reads at most 19 digits of groups, so the search is linear in the run. Test: `TestLuhnRecognizeLongDigitRunIsBounded` (fails on timeout).
- [x] **PII redaction bypass (confirmed, fixed).** The guard I added skipped any run that began with two digits after two letters, so `Ref AB12 4111 1111 1111 1111` and `ID01 4111111111111111 123` left the card visible. A second case: a 4-group window that passes Luhn by chance (`1004 4111 1111 1111`) claimed the card's first 16 digits and left its last `1111` visible (`1004 4111 1111 1111 1111` came back as `****-****-****-1111****`, which shows the trailing four digits only as a mask). Fixed: the IBAN guard now skips only a run inside a span the IBAN recognizer reports as valid. Every valid window is found and overlapping windows are merged, so every card digit is covered. Tests: `TestLuhnRecognizeCardAfterLettersAndDigitsIsCovered`, `TestLuhnRecognizeChancePassingWindowDoesNotCutCard`, `TestScrubberRedactsCardAfterLettersAndChancePrefix`.
  - **Cost, accepted:** the merge can widen a card span by a neighbouring group when a chance-passing window overlaps it. The seed case `pos-card-then-year` now reports `4111 1111 1111 1111 2024`. Exact precision and recall on the seed moved to 60/62 and 60/61. Overlap recall stays 1.0. Gate bounds were re-measured and recorded in `golden/accuracy_test.go`. This is a decision: an extra redaction is preferred to a visible card. Reverting the merge restores exact 1.0 but reintroduces the bypass.
  - **Cost, measured:** nemotron exact false positives 471 to 475, all card windows on account-number spans the corpus labels as unlabelled. Gretel unchanged. Presidio card module exact tp 14, fp 2, fn 2 (overlap 15, 1, 1), because a merged span covers two adjacent gold cards.
- Residuals from the fix are closed 2026-10-15: the dash-joined card (see the card entry above) and the spaced IBAN followed by two or more uppercase blocks (see the entry below).
- Review of `06b84dc` (background security notices, no finding text): the label parser differential (`IP:2001:db8::1` not reported) and the superlinear scan (1 MB of `1.1.1.1 ::1` did not finish in 5 s) are both confirmed by probe and fixed in `e532474`. The superlinear IBAN containment check is fixed in `8eb884c`. The label residual that the fix leaves is in the IPv6 entry below.
- **Decided 2026-10-15 (user): keep the merge.** Restoring exact spans would reintroduce the bypass above. Recorded in SPEC.md.

## Presidio test cases: open findings (2026-10-15)

Source: Presidio `presidio-analyzer/tests` at `2523c7b74a469270c5c78bb253f140eafca21e31` (MIT). `make presidio` extracts and scores the in-scope modules. Scored apart from the CI gate. Numbers and method are in SPEC.md "Presidio test cases". Nothing here changes a recognizer yet.

- [x] **Fixed 2026-10-15: fail-open, cards and IBANs next to another digit group.** A card followed by a space and digits (`4111111111111111 123`), two cards separated by a space, and an IBAN followed by an uppercase word or digit group were not redacted. Seed batch 9 and `TestLuhnRecognizeCardNextToDigitGroup`, `TestIBANRecognizeIBANNextToCapitalisedWordOrDigits`, `TestScrubberRedactsCardNextToDigitGroup` pin them. The fix is in `checksum.cardSpans` (card windows, used only when the whole run fails Luhn; single groups must be bounded by a space, hyphen or dot; a run inside a valid IBAN is not searched) and `checksum.ibanSpan` (first-block or minus-final-block prefixes, with `canonicalIBANShape`). Measured before and after in SPEC.md "Presidio test cases". Corpora: no change in recall, precision unchanged (nemotron fp 471, gretel fp 1105). Corpus check: 0 of the corpus CREDIT_CARD misses pass Luhn on their own, so the real-text leak had no measured instance and the earlier "keep headline" decision stands.
  - **Fixed 2026-10-15: dash- or dot-joined card.** A card next to a CVV or number joined by `-` or `.` (`4111111111111111-123`) was not redacted, and the card digits leaked. A single group is now bounded by a space, hyphen or dot. Cost, measured and accepted: Luhn-passing 13 to 19 digit groups joined by `-` or `.` are reported (gretel CREDIT_CARD fp 26 to 32, nemotron 22 to 23; recall unchanged). See SPEC.md.
  - **Fixed 2026-10-15: spaced IBAN followed by two or more uppercase blocks.** `GB82 WEST 1234 5698 7654 32 ALL FEES` and an invoice line such as `DE89 3704 0044 0532 0130 00 BIC TESTDEFF` now keep the IBAN. `ibanSpan` tries cuts at each space from the right and takes the longest that has the printed shape and passes mod-97. Cost, measured and accepted: Presidio IBAN fp 0 to 3 (P 1.0 to 0.9812). The three are chance mod-97 passes on shorter cuts of negatives (`MC58 A122 2000 0101`, `JO94 CBJO 0010 0000 0000 0131`). The two-shape rule this replaces existed to prevent exactly that. Gretel IBAN unchanged. See SPEC.md.
  - **Decision needed:** whether to add a country-length table for IBAN prefixes. The table would reject a cut whose length is not the country's, which removes the chance passes this fix adds.

- [x] **IPv6: in scope, detected 2026-10-15 (`06b84dc`).** `2001:db8::1`, `fe80::1`, `::1` and the full form are reported. `netip` validates each candidate, so clock times and MAC addresses stay out. Residual: EDIFACT component separators (`387::9`, `220::5`) are reported as IPv6, and this is a gretel false-positive class. See SPEC.md.
- [x] **IPv6 label residual. Decided 2026-10-16: keep current behaviour, documented.** `text2001:db8::1` reports `db8::1`, so `2001:` stays visible. The example `2a02:4d60:1031:4c3f::85e1:79f2:9122:5234` is not a valid IPv6 address (eight groups plus `::`, so `netip` rejects it); the span reported is its valid suffix, so no real address loses a group. A rule that reports a whole glued hex run would fix the prefix case, but it breaks `id:fe80::1`: `netip` parses `d:fe80::1`, so the `d` of `id` would be swallowed. The two cases cannot be told apart by shape, so the label rule stays. Residual accepted: a letter-glued prefix such as `text2001:db8::1` stays partly visible.
- [x] **IPv6 span conventions, Presidio.** CIDR suffixes (`/24`) and zone IDs (`%eth0`) are not in our span. The zone is an interface name, not personal data. The unspecified `::` is not redacted by design, since it is common in code. A URL with an IPv6 host wins the overlap, so the address is redacted inside the URL span.
- [x] **IPv4-mapped IPv6 leaves its prefix visible.** Fixed 2026-10-15 with IPv6 detection: `::ffff:192.0.2.1` is one span.
- [x] **Phone: truncated international numbers. Decided 2026-10-16 (user): option (a), keep the truncation and accept the leaked tail digits. Parenthesised area code fixed 2026-10-16.** `+44 (20) 7123 4567` is now one span (branch 2 accepts `(NN)` after the country code, no digits added). **Still open:** `+55 11 98456 5666` is reported as `+55 11 98456`, leaving `5666` visible, and `+33 1 42 68 53 00` as `+33 1 42 68`. The phone regex stops at a grouping boundary. **Tried and reverted 2026-10-16:** widening the middle group to `\d{1,5}` fixes the Brazilian case, but the span then reaches `5666`, which a card window can also start on. A card wins overlap on confidence (0.99 over 0.98), so the phone digits `98456` leaked (`TestScrubberRedactsPhoneNextToCardOrDigitGroup` guards this). Longer tails need more groups, so they hit the same overlap. **Decided (a):** keep the truncation, accept the leaked tail digits. Options (b) and (c) were not taken. The guard is `TestScrubberRedactsPhoneNextToCardOrDigitGroup`. Measured for the paren fix: gretel PHONE fp 691 to 693 (P 0.2772 to 0.2766), nemotron unchanged; Presidio phone exact tp 18 to 19 (P 0.7826 to 0.7917, R 0.6429 to 0.6786), overlap R 0.8214 to 0.8571.
- [x] **Phone: national formats. Decided 2026-10-15 (user): out of scope, documented.** Israel `09-7625400`, Japan `090-1234-5678` and UK `020 7946 0958` are not matched. Presidio gets these from the `phonenumbers` library (`generic/phone_recognizer.py`), which the dependency-free design rules out. Follow-up, not started: MY, SG and TH national formats, each needing a numbering-plan source.
- [ ] **Phone: glued boundary.** `my international one is415-555-0132` is missed. The `\b` boundary is the documented glued-ID scope (0d602f9), so this is consistent with that decision.
- [x] **SSN: shape-only. Decided 2026-10-15 (user): keep shape-only, documented.** Presidio rejects area 000 and 666, group 00, serial 0000, all-same digits, and three sample numbers. Adopting that set would change a seed positive (`pos-ssn`, `123-45-6789`, plus dot and space variants), 9 of 52 gretel SSN gold spans, and five test files that use `123-45-6789` as a positive. That needs your approval for seed and test changes. 9xx stays: Presidio's own `us_itin_recognizer` matches 9xx as ITIN, which is personal data.
- [ ] **SSN: weak-match shapes.** Presidio expects `078-051121 07805-1121` as two matches at low confidence. We miss both spans. Scored as-is; no decision needed until the SSN rules above are decided.
- [x] **URL: bare domains. Decided 2026-10-15 (user): out of scope, documented.** `microsoft.com` and `google.co.il` are not detected. A bare domain is rarely personal, and file names and versions (`config.yaml`, `v1.2`) would be false positives. The email recognizer covers personal domains. Quoted URLs: the gold includes the quotes and our span does not; ours is the better span.
- [x] **Email: punycode TLDs. Fixed 2026-10-16.** `a@xn--d1acufc.xn--p1ai` was cut to `a@xn--d1acufc.xn`. The TLD now accepts an `xn--` label (`recognizers/regex/regex.go`, `TestEmailRecognizer` punycode cases). The URL recognizer already takes any non-space character, so it needed no change.
- [ ] **Email: Unicode IDN addresses are not matched at all.** `a@example.рф` returns no EMAIL, so the whole address stays visible. The domain and TLD classes are ASCII-only. Defect; the fix needs a decision on the domain class (Unicode letters in the domain and TLD, or punycode conversion before matching) and what counts as a valid TLD.
- [ ] **IBAN: separators and caps.** `AL47-2121-1009-0000-0002-3569-8741`, `VG96 VPVG 0000 0123 4567 8901 X` (inside a sentence) and `AL47212110090000000235698741 ALL CAPS` are missed. Dash separators and sentence-embedded IBANs are IBAN-shape questions. Decide with the IBAN normalisation work.
- [ ] **Card windows inside invalid IBAN-shaped bodies.** After the 2026-10-15 fix, seven Presidio IBAN-module cases still get a CREDIT_CARD window on a digit group inside the IBAN body (`1250 0000 0001 2300 695`, and similar). The IBAN module does not label CREDIT_CARD, so these are out of schema, not scored. The corpora have none. The 2026-10-15 review fix dropped the 2-digit IBAN-body guard. It now skips only runs inside a valid IBAN, so an invalid IBAN-shaped body is searched. Out of schema for the IBAN module, so not scored; a count-based guard would reduce it.
- [x] **Thai TNIN: keep mod-11. Decided 2026-10-15 (user).** `0034567890124`, `1974567890124`, `1984567890124` and `1994567890124` pass our check. Presidio also rejects a first digit of 0 and province codes 28, 29, 59, 68, 69, 78, 79, 87, 88, 89, 97, 98, 99. Its docstring cites Wikipedia and ISO 3166-2:TH, which this session could not confirm. Revisit if the ISO list is checked.
- [ ] **Asian ID port: `ph_tin` then `in_gstin`.** Approved 2026-10-15, blocked on the format documents. Both have government owners, but the files cite only homepages (`bir.gov.ph`, `gst.gov.in`), not a format or checksum specification. **Search result 2026-10-16:**
  - **BIR TIN (`ph_tin`).** Found on the Supreme Court E-Library: Revenue Memorandum Circular No. 58-91 (https://elibrary.judiciary.gov.ph/thebookshelf/showdocs/10/49690) and Revenue Memorandum Order No. 23-91 (https://elibrary.judiciary.gov.ph/thebookshelf/showdocs/10/49676). RMC 58-91 gives a 9-digit numeric TIN: first digit is the taxpayer type (0 corporation, 1–9 individual), digits 2–8 sequential, digit 9 a check digit. It gives no check-digit formula. RMO 23-91 says "refer to Annex A for TIN structure", and Annex A is not in the scraped page. Third-party sources disagree on the branch suffix (three or five digits). No published check-digit algorithm was found, and Lookuptax says the BIR has not published one. **Annex A is not online:** the RMO 23-91 page ends "Text available at U.P. Law Center upon request" for the annexes. **Open:** obtain Annex A from the U.P. Law Center (NAR office), or find another primary source for the check digit. Until then, the weighted mod-11 in Presidio's `ph_tin_recognizer.py` (weights 9..2 on the first 8 digits) has no primary source.
  - **GSTIN (`in_gstin`).** `gst.gov.in` and `services.gst.gov.in` were unreachable from this machine (timeouts); `gstn.org.in` loads, but its homepage links no format or check-digit document. The ICAI knowledge-sharing PDF (http://idtc-icai.s3.amazonaws.com/download/knowledgeShare18-19/Structure-of-GSTIN.pdf) and third-party sources give the 15-character layout (2-digit state code, 10-character PAN, entity number, fixed `Z`, check character). Their check-character methods (Luhn-style mod-36, with the starting weight disputed) are not official. **Correction:** Presidio's `in_gstin_recognizer.py` has no check-character algorithm. Its `_validate_gstin` only tests that the 15th character is alphanumeric, so do not treat Presidio as a checksum source. **Reputable secondary source, 2026-10-16:** python-stdnum's `stdnum/in_/gstin.py` (LGPL, Arthur de Jong; the source is at https://raw.githubusercontent.com/arthurdejong/python-stdnum/master/stdnum/in_/gstin.py, since arthurdejong.org blocks automated fetches) computes the check character as Luhn mod 36 over the first 14 characters, using `stdnum/luhn.py`. Its docs cite no GSTN document. A scratch re-implementation reproduces the library's vectors: `27AAPFU0939F1ZV` passes and `27AAPFU0939F1ZO` fails. **Still open:** the official GSTN or CBIC specification, to confirm the mod-36 method and the starting weight. Until then, a port could follow python-stdnum with that source recorded. A format-only port would need a keyword gate.
  - **Dataset search, 2026-10-16:** Hugging Face has three GST datasets: `ligaments-dev/gst-india-sft` (small, no license, generated by an ML agent), `ram1o1/gstin-app-dependencies` (no license or card), and `transitionGap/gst-india-preference-dataset-prep-small` (MIT, small). None is a spec. Kaggle search returned no labelled GSTIN set. data.gov.my is reachable, but its catalogue has no TIN or GSTIN specification. No Philippine TIN spec or sample set was found on Hugging Face, Kaggle or data.gov.ph.

  Needed before porting: Annex A of RMO 23-91 and the official GSTIN check-digit specification, checked against each checksum. Later, each with its own source: `sg_uen` (checksum; cites a gist, needs ACRA), `kr_rrn`, `kr_frn`, `kr_brn` and `in_aadhaar` (checksums; Wikipedia or aggregator sources). Shape-only IDs (`kr_passport`, `ph_passport`, `ph_umid`, `in_pan`, `in_voter`, `in_vehicle_registration`, `kr_driver_license`) would need a keyword gate. `in_passport` cites a blog and should not be ported.
- [ ] **All-zero card number.** `0000000000000` is a Luhn-valid CREDIT_CARD. Presidio marks it a non-match. Low risk; a zero-run guard would be a new rule. Decide with the Thai and card rules.
- [ ] **SG NRIC: gold disagrees with the check letter.** Presidio expects `T1234567Z` and `G1122144L` to match. Under the weights in SOURCES.md the check letter for `T1234567` is `J`, so `Z` fails. We reject them, which is the right behaviour. `M4332674T` (M prefix) is the known unsupported case. `A1234567Z` and `B1234567Z` are not NRIC prefixes. Gold issue, no change.
- [ ] **Not scored.** IP count-only rows (3; Presidio asserts how many matches, not where). Phone leniency rows (6; Presidio varies the match with a leniency setting, and only the default recognizer is scored). Empty-text negatives (2; `golden.Load` rejects empty text). All are in `skipped.json` with the reason.

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
- [x] **Four-part version strings are read as IPv4.** Accepted as a known defect on 2026-10-09 (option a below). `neg-ip-version-four-part`
      ("Upgrade the agent to 3.10.2.7") has four valid octets, so the IP
      recognizer reports it. `neg-version-string` passes only because its
      version has three parts. **Decided 2026-10-09: (a), accept it as a known defect.** Options that were considered: (a) accept it, since a dotted
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
- [ ] **Golden dataset is an 81-case seed set, not the ~500 SPEC.md calls for.**
      `testdata/golden/seed.json` and the `golden` runner now exist and are
      tested, so the ">99% precision/recall" claim is at last measurable, but
      the seed set is far too small to support that number. Batch 2 (13 cases)
      took the baseline off 1.0 on purpose. Batch 3 (13 cases) added the national
      IDs. Batch 4 (5 cases) added Malaysia passports. Batch 5 (4 cases) added a label-glued SG NRIC and keyed bare MyKad. Batch 6 (2 cases) added Luhn-valid Thai IDs, typed TH_ID by decision (d). Batch 7 (3 cases) added Singapore passports (2 positive, 1 negative). Current: exact P=0.9804
      R=1.0000 (tp=50 fp=1 fn=0), overlap the same, 27 of 28 negatives clean. One of
      the two defects behind it is fixed (URL, now trimmed); the IP
      version-string defect remains and is its own entry above. Thresholds:
      `50.0 / 51.0` for exact precision, and a literal `1.0` for recall.
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
- [ ] **Context boosting: measured, and it is worse on nemotron. Boosting stays off by default.** `piiscrub_test.go` `TestContextBoostIsNoWorseOnSeed` (`boost_gate_test.go`) scores the seed set both ways: identical (exact P=0.9811 R=1.0000, tp=52 fp=1). The corpora, scored the same way with the corpus schema (`make corpus` headline numbers match "off"): **nemotron exact P 0.8340→0.8320 and R 0.8699→0.8681** (tp 2367→2362, fp 471→477); **gretel unchanged**. The cause, from a per-span diff over all 2000 nemotron cases: 7 URLs (0.99) lost their whole span, each a URL that contains an IP (`http://192.168.1.100:8080/...`) or a card number in its query string. Boosting raised the inner IP_ADDRESS (1.00) or CREDIT_CARD (1.00) above the enclosing URL on overlap, so the URL is replaced by the inner span and its path and query stay visible. **Flow Context (not applied):** *How it currently works:* `Analyze` boosts every entity whose type has a matching context word, then resolves overlaps by confidence, so a boosted inner entity can beat a wider one. *Where it breaks:* the inner entity's boost crosses the boundary of a URL that contains it. *Proposed fix:* either cap the boost so it cannot lift a span above a containing URL, or let URL win any overlap it encloses. **Decision needed:** this is a policy choice about what boosting may override. Until it is made, `WithContextBoost` should not be enabled for URL-bearing text.
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

- [ ] **CREDIT_CARD recall is bounded by Luhn against synthetic data.** On
      nemotron, R=0.0945 with 230 FNs — and all 230 fail the Luhn check while
      exactly the 24 Luhn-valid ones are the true positives. The validator is
      working as designed (it is the same check that rejects a Luhn-failing seed
      negative). Open question, not a defect: is a corpus of non-Luhn-valid
      "card numbers" a fair recall target for a Luhn-gated detector? If not, the
      conversion should either exclude those gold labels or the metric should
      report them separately.
      **Conclusion (advisor 2026-10-14): keep the headline; no change.** A separate bucket for non-Luhn gold is optional later work, not a user question.
- [ ] **Corpus-side label quality depresses measured precision.** gretel EMAIL
      reads P=0.3281, but spot-checking shows the corpus labels the local part
      (`user_name 90..96 = 'lowe73'`) rather than the full address, markdown
      `[addr](mailto:addr)` renders each address twice while only one
      occurrence is labelled, and 106 of 647 email-shaped occurrences in
      email-bearing records carry no email label at all. A corpus miss looks
      exactly like our false positive. Before any threshold is recorded from
      these numbers, spot-check a sample of FPs by hand (spec §8 risk 4).
- [ ] **gretel PHONE precision (0.2766) is unexplained.** Unlike SSN and
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

- [x] **BITCOIN was shape-only: any base58 or bech32 string of the right shape was reported. Fixed 2026-10-14.** `recognizers/checksum/bitcoin.go` adds `BitcoinValidator`: Base58Check (double SHA-256, 25-byte payload) for `1`/`3` addresses, and bech32 (BIP-173, version 0) or bech32m (BIP-350, version 1 and later) for `bc1`, with the program-length rules. `regex.BitcoinRecognizer` reports a shape match only when its checksum verifies. The confidence stays 0.95, so overlap outcomes are unchanged. The shape also widens to 87 characters after `bc1`, so the taproot `bc1p` form is matched and checked by bech32m. Tests: `checksum/bitcoin_test.go` (BIP and wiki vectors, plus flipped-checksum negatives), `regex_test.go` (taproot), golden batch 8 (taproot and P2SH positives, two bad-checksum negatives). Golden gate: exact P=0.9811 (tp=52 fp=1), recall 1.0, 29 of 30 negatives clean; `minCases` 81. Corpora: `make corpus` headlines unchanged. gretel's 44 shape matches are now 21 validated; the 23 dropped are all BITCOIN and no new detection appeared (diffed over the seed and all 4000 corpus texts). **Trade-off, recorded:** a token that is not a valid address is no longer redacted, so a shape-only string that a document uses as an ID is visible. `piiscrub_test.go` `TestScrubberBitcoinSpanNotDisplacedByEmbeddedNRIC` was rewritten: the bad-checksum token `1Ab3CdEf5Gh7Jk9Mn2Pq4RsS1234567D` now yields no entity, and its tail `S1234567D` stays visible because a glued SG NRIC is only found after its label (see the national-ID entry). A valid address next to an NRIC is pinned by the new test. **Review response (2026-10-14, advisor):** a background review flagged `regex.go` as fail-open. The gate stays: a real address always passes its checksum, so a failing shape match is not a usable address. **Disclosed:** `TestScrubberBitcoinSpanNotDisplacedByEmbeddedNRIC` was pinned by the 9594065 security review, and the rewrite changes what it asserts. The tail `S1234567D` of `1Ab3CdEf5Gh7Jk9Mn2Pq4RsS1234567D` is now visible, because a glued SG NRIC is found only after its label. That is the same open glued-ID question as Thai (see the national-ID entry). **Dropped-span check:** of the 23 gretel shape matches that no longer validate, 3 sit on spans the corpus labels with an out-of-map type (`street_address` 2, `api_key` 1), and 20 on no label. The 3 were already outside the headline, but the corpus does label them as PII, so their loss is real coverage loss. **Deviation:** `regex.NewBitcoinRecognizer` is now checksum-gated, so no shape-only bitcoin recognizer remains exported. Cards still have `regex.NewCreditCardRecognizer` as a shape-only option.
- [x] **`Anonymizer.Anonymize` did not validate rune boundaries. Fixed.** The bounds
      check ran per splice against the shrinking working string. Now every span is
      checked against the original text before any splice: out of bounds, or a start or
      end inside a multi-byte UTF-8 rune, is an error naming the type and span, and
      the result is empty (fail closed, as in the operator-default change). Tests:
      `TestAnonymizeRejectsSpanInsideRune` (inside-rune start and end are rejected;
      an entity on rune edges is still accepted); `TestAnonymizeRejectsOutOfBoundsSpan`
      still passes. Contract in SPEC.md §Anonymizer.
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
- [x] **Module path renamed to `github.com/u007/dataprotect`. Done 2026-10-15.** The bare `dataprotect` path could not be imported by consumers. Every Go import, the root `go.mod`, the corpusbench tool module (`github.com/u007/dataprotect/tools/corpusbench`, with `replace github.com/u007/dataprotect => ../..`), `scripts/benchmark.sh` and the README and usage examples now use the new path. Verified: `go test ./...` and `make test-tools` pass. The dated `docs/superpowers/` specs and plans keep the old name; they are historical records and are not edited.
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

- [x] **Anonymizer passed through any type without an operator (fail-open for custom maps). Decided 2026-10-09: option 1 (fail closed).** `Anonymize` now returns an error, before any splice, for an entity whose type has no operator and no default. Breaking for partial maps; the error names the type and span. Options 2 and 3 were done earlier, and the default `NewScrubber` is unchanged. Tests: `TestAnonymizeUnconfiguredTypeIsAnError`, `TestAnonymizePerTypeRoutingOracle`. Contract in SPEC.md §anonymizer. The history below is kept for the record. Probed 2026-10-09: a card-only operator map, the manual-wiring example in README and the usage doc, leaves `Citizen ID 1000000000009` unredacted. Before the Thai decision (d) the same number was a card and was masked. The default `NewScrubber` is safe: it sets an operator for every type the default recognizers emit. The same gap exists for SG_NRIC, MY_IC, MY_PASSPORT, TH_ID and IBAN in any custom map that omits them. Options: (1) make `Anonymize` fail closed, returning an error for an entity with no operator and no default. This is a breaking change for partial maps, but it fails loudly. (2) Keep the default and change the manual example to `anonymizer.New(anonymizer.WithDefaultOperator(operators.NewRedactOperator()))`, so callers opt in to redact-all. (3) Keep both and add a test that every type the default recognizers can emit has an operator in `NewScrubber`. Recommendation: (2) plus (3) now, and (1) only if you want the breaking change. **Done:** (2) the README and usage manual examples now use `WithDefaultOperator(operators.NewRedactOperator())`. (3) `recognizers.AllEntityTypes` lists every type, and `TestScrubberEveryEntityTypeHasOperator` checks each has an operator in `NewScrubber`. A new constant must be added to that list by hand, so the test cannot catch a type left out of the list.
- [ ] **Passports: Malaysia and Singapore done, Thailand uncovered (decided 2026-10-15).** Other Asian countries are not started.
      - **Malaysia (done, 2026-10-09; user accepted the secondary sources):** `MY_PASSPORT` matches `[AHK]\d{8}` only after a keyword from Purview's list (passport, pasport, nombor pasport, passport no, passport number, and so on). The shape is never reported alone, because it is too short to be safe. Sources: Wikipedia and Microsoft Purview. The EU PRADO record (`mys`) is the primary source and is unverified: PRADO returned 403. Re-checked 2026-10-14 via the NL path: `MYS-AO-03001` (2017) has no numbering composition, and `MYS-AO-02001` (2010) has no numbering entry. The eight-digit shape stays unconfirmed by PRADO. The old and 30 June 2026 booklets are not split.
      - **Singapore (done, 2026-10-09; option 1 chosen by the user, still unverified):** `SG_PASSPORT` matches `[EK]\d{7}[A-Z]` only after a passport keyword, the same gate as Malaysia. Shape from Purview (E or K, seven digits, one letter; its newer definition allows K) and the 2006 ICA BioPass description, which gives the E form. PRADO `SGP-AO-05001` (ordinary 2017), read via the NL and fr paths on 2026-10-14, gives `ANNNNNNNA`: seven digits between two letters, so the digit count now has a primary source. E vs K is accepted on Purview (E or K) and the regex takes both behind the keyword gate; it is not an open question. It shares the Malaysia known limits below: no words between keyword and number, and `structured/` analyzes CSV cells alone. The 2017 redesign is not checked against a source. No hits in either corpus.
      - **Thailand (reopened 2026-10-15 for the 2020 booklet):** no source states the passport-number format. **Read 2026-10-15:** `THA-AS-06001` (official passport, 2020, loaded at `https://www.consilium.europa.eu/prado/NL/THA-AS-06001/index.html` in htrcli tab `1100666842`, after the Cloudflare challenge cleared). The page lists "Nummering" only as a security-feature label (laser engraving, letterpress, laser perforation). It has no numbering composition, so the official booklet adds no shape. Earlier attempts 2026-10-15: `prado.consilium.europa.eu/NL/THA-AS-06001` redirected to the PRADO home, and the lowercase `www .../prado/nl/tha-as-06001/` returned 404. **Read 2026-10-15, same pattern:** `THA-AD-06001` (diplomatic) and `THA-AP-06001` (emergency). Both loaded after the Cloudflare challenge cleared (about 10 to 15 seconds, no interaction). Each has "Nummering" as a label only and no `Samenstelling`. All four 2020 booklets are now read with no number format. Thailand stays uncovered. The ICAO Doc 9303 field is nine characters, but that is the machine-readable zone, not a number written in prose. The one sample (`B100990`, one letter then six digits) comes from a data-extraction tool, so it is not evidence. PRADO (`THA-AO-06001`) is a document-type code, not a number. Re-checked 2026-10-14 via the NL and fr PRADO paths: `THA-AO-06001` (2020, current) has no numbering composition; "Nummering" is only a label, and "serienummer" is a print method. `THA-AO-02002` (2012, superseded) has `Samenstelling: AANNNNNNN` at the top-right corner: two letters then seven digits. That shape is the 2012 booklet's only; it does not cover the current passport. Facts checked 2026-10-14: the 2012 booklet's maximum validity is 5 years, so every one has expired (first issued 23/11/2012). Its own two Numbering entries disagree on digit count (`AANNNNNNN` 7 digits, and `AAN NNNNNNN` 8 digits, the serial). Microsoft Purview's Japan passport definition is two letters then seven digits, the same shape. Converted corpora (4000 cases, nemotron and gretel) have 0 passport-keyword hits with that shape. Recommendation: leave Thai uncovered, because the only shape is historical and overlaps Japan. **Decided 2026-10-15 (user, on advisor's recommendation): Thai 2012 passport stays uncovered.** No recognizer change. The Thai 2020 format is still unknown, so this entry stays open for that question only. Attempt 2026-10-14 at `THA-AS-06001`: the `/prado/NL/` path redirected to `/prado/en/` behind a Cloudflare managed challenge. Not bypassed; tab `1100666842` left open.
      - **Alternative sources for PRADO (2026-10-10).** PRADO record pages (`SGP-AO-05001`, `SGP-AS-03001`, `THA-AO-06001`) return HTTP 403 to WebFetch. The claude-in-chrome browser extension was not connected, so the browser route is blocked. No mirror or alternative source states the format.
        - **Confirmed 2026-10-12 via htrcli (managed Chrome, page text):** PRADO `SGP-AS-03001` (the official passport, AS, not ordinary) lists "lion's head / passport number (format ANNNNNNNA)". This supports the letter, seven digits, letter shape. It is the official passport only and does not settle E vs K. `THA-AO-06001` (Thai ordinary passport) text has no number format: "Numbering" appears only as a security-feature label. `SGP-AO-05001` (ordinary, 2017) loaded a blank body with no title on 2026-10-12, after repeated checks, so it is still unread. A Cloudflare "Just a moment..." challenge on `THA-AO-06001` cleared on its own within about 8 seconds; no interaction was made.
        - **Re-checked 2026-10-14 via htrcli (Chrome tabs):** `SGP-AO-05001` is still unread. The four search-result URL variants (`prado/EN`, `prado/en`, `prado/En`, `prado/EN`) all load blank: title equals the URL, body text empty. The user reported a 404 for the link they had. The listing link does not navigate on click. `SGP-AO-04001` (ordinary 2006) and `SGP-AS-05001` (official 2017) were tried too: `SGP-AO-04001` loads and has no numbering entry (booklet 64 pages, e-passport 2006 model); `SGP-AS-05001` is blank. `SGP-AO-03001` (ordinary 2003, the page the user had open) loads and has no numbering entry either. **Found the 2017 numbering text on 2026-10-14:** the `/prado/NL/` and `/prado/fr/` language paths for `SGP-AO-05001` load and show "Numbering", composition `ANNNNNNNA` (letter, seven digits, letter). The same composition is on the official `SGP-AS-03001`. That confirms the shape for the ordinary 2017 passport, but not E vs K. The EN and lowercase paths stay blank. The `/prado/IT/` path is blank.
        - Wikipedia (fetched, 2026-10-10): biometric passport numbers are unique, "instead of being identical to the holder's NRIC number". Legacy passports carried NRIC-shaped numbers. Probe 2026-10-10: `Passport No: S1234567D` and `Passport: T1234567J` are caught as `SG_NRIC` at 0.99 and redacted. Legacy coverage is therefore via SG_NRIC, not SG_PASSPORT.
        - Thailand: still no format in any source. Wikipedia gives the data-page label `หนังสือเดินทางเลขที่` (passport number). Keep it as a candidate keyword for later. Implement nothing until a format is sourced.
        - Source independence: `dotnet.territoriali.olinfo.it` mirrors Purview, so it is not a second source. Elysiatools is a sample generator. HL7 and Regula public pages give no format. Regula IRS is subscription-only.
      - **No checksum route for prose.** The ICAO check digit exists only in the MRZ line. A passport number written in a sentence has no checksum, so the keyword gate is the only defence, as in Malaysia.
      - **Known limits of the Malaysia rule (2026-10-09):** (1) Words between the keyword and the number are not allowed, so `Passport Number of the Primary Driver: H26965339` in gretel is missed (the Purview proximity is 300 characters; we use none). (2) `structured/` analyzes each CSV cell alone, so a `passport_number` column header never reaches the analyzer; a passport column is detected only when its cell carries the keyword. (3) The type name says Malaysia, but the rule checks keyword and A/H/K shape only. Other countries use one letter and eight digits too, so a hit is a passport-like number, not a verified Malaysian one. (4) Real-text signal: gretel has one hit, `Passport number: K13198743`, out of schema (gretel has no passport label), so it is not scored either way; nemotron has none.
      - **PRADO attempts (2026-10-09):** WebFetch returned 403. The htrcli browser route failed (content-script injection error), `--cdp` fetch is unsupported, and the claude-in-chrome extension was not connected. Retry from a browser session the user opens.
- [ ] **National ID coverage gaps.** (a) Singapore M-prefix is unsupported: its check-letter table is unconfirmed. (b) Malaysia MyKad place-of-birth codes are not validated: the code list is unconfirmed, and the MyKad check-style digit is not validated either. (c) **Decided 2026-10-10: (a), keep the bare 13-digit Thai form as is, with no keyword gate.** Its mod-11 check passes about 1 in 10 random 13-digit runs. Corpus check: nemotron has 0 bare 13-digit runs and gretel has 38, none mod-11 valid. That is too few to measure a real FP rate, so the risk is accepted, not measured. (d) Batch 3 added synthetic golden cases for them (4 SG, 2 MY, 2 TH positives; 5 negatives). The corpora don't label these types, so no recognizer is measured on real text.
- [x] **National-ID gaps from the earlier national-ID commit (9594065), fail-open.** A background security review flagged the national-ID recognizers as fail-open. Its finding bodies never arrived, so the items below are what my probes through `Scrubber.Anonymize` found, not the review's wording. Fixed: a label-glued SG NRIC (`NRICS1234567D`) is found, only when the NRIC or FIN label comes before it. The earlier fix dropped the leading `\b` for every NRIC, so a 0.99 NRIC inside a shape-only 0.95 bitcoin token (`1Ab3CdEf5Gh7Jk9Mn2Pq4RsS1234567D`) won the overlap and left 23 characters visible. The review flagged this as an overlap displacement. The bare-token case is now back to one full BITCOIN span, and a differential over 4072 seed and corpus texts shows no lost detections. Fixed: bare 12-digit MyKad is found only after an identity-card keyword (Purview's list; date check kept). **Decided 2026-10-15 (user, on advisor's recommendation): glued national IDs are documented scope.** Probed through `Scrubber.Analyze` on 2026-10-15: `ID3100600445635` (Thai, label glued on) and `ORD3100600445635` are not redacted; `xS1234567D` (SG, letter before, no label) is not redacted. `Thai ID:3100600445635`, `ID 3100600445635`, `NRIC: S1234567D` and `NRICS1234567D` are redacted. Reason: relaxing `\b` would also redact order and reference numbers, so the precision cost is larger than the recall gain. Recorded in SPEC.md, MVP scope, national ID bullet.
- [x] **Valid Thai citizen ID that passes Luhn was reported as CREDIT_CARD. Decided 2026-10-09: TH_ID wins (decision d).** Before the decision, `1000000000009` (and `1-0000-00000-00-9`) passed both the Thai mod-11 check and Luhn, so the card recognizer (0.99) beat TH_ID (0.95). The masked output was `****-****-****-9***`, which shows the check digit. Fuzzing generated check-valid IDs in unlabelled contexts found 6 of 30 typed CREDIT_CARD, in every non-glued context and both forms. A background review flagged it as a false negative in the redaction control. Implemented (current design): the recognizers still report CREDIT_CARD. `Scrubber.Analyze` retypes a resolved card span to TH_ID when the span is a Thai citizen ID with the same word boundaries the Thai pattern uses. The retype runs after overlap resolution, so the span keeps its 0.99 through threshold and overlap. Two earlier designs were rejected. (1) The deferral in the card recognizer left a Thai-valid number unredacted when the Thai recognizer was not wired or the threshold was above 0.95. (2) Typing it TH_ID inside the Luhn recognizer changed the type for callers with card-only maps, so a Thai ID they used to mask went out unredacted. **Trade-off, recorded:** decision (d) applies to the `Scrubber` API only. Raw analyzers and the `structured/` CSV path still report a masked card for a Luhn-valid Thai ID. That is the old behavior, not a leak. Other spans stay CREDIT_CARD, including glued candidates, so nothing goes unredacted. Golden cases `pos-th-id-luhn-valid` and `pos-th-id-luhn-valid-hyphen` pin the result. Rejected alternative: raising TH_ID above 0.99. That would also beat URL, email and SSN at 0.99 on overlap, so a URL containing a Thai-shaped number would lose its path to the mask.
- [ ] **Phone pattern matches ID-number digit runs (nemotron).** Of the 419
      nemotron PHONE false positives, 406 sit on spans the corpus labels as
      ID fields: `biometric_identifier` 117, `customer_id` 83,
      `account_number` 79, `medical_record_number` 76,
      `health_plan_beneficiary_number` 34, and smaller counts. Checked
      2026-10-14 by a scratch scan of all 2000 nemotron cases (not the hand
      check of individual spans): 975 PHONE predictions, 419 false positives
      (matches the count above). Of the 419: 406 sit on ID-type labels
      (`account_number`, `biometric_identifier`, `customer_id`,
      `medical_record_number`, `health_plan_beneficiary_number`,
      `certificate_license_number`, `unique_id`, `http_cookie`), 2 on
      `coordinate` or `street_address`, and 11 on no label. 377 are exactly
      10 digits with no separators; 42 are longer runs (10–11 digits with
      separators or a leading 1). **Cause, confirmed in code:** the bare
      branch of `NewPhoneRecognizer` (`recognizers/regex/regex.go`)
      `\b(?:\+?1[\s.\-]?)?\d{3}[\s.\-]?\d{3}[\s.\-]?\d{4}\b` matches any
      10-digit run. It has no keyword or context check and reports at 0.98.
      The golden seed has no bare 10-digit PHONE positive (checked), so the
      seed does not measure this branch.
      **Recommendation changed 2026-10-14 (advisor): no change.** The 406 ID-type spans are PII in the corpus's own labels (account numbers, medical record numbers, customer IDs, biometric IDs). No other recognizer catches them: SSN needs separators and Luhn needs 13 to 19 digits. A keyword gate would leave about 406 such spans per 2000 nemotron records fully visible. The precision figure is a scope artefact (see the SSN entry, option c). The proposal below is withdrawn and kept for the record.
      **Proposed fix (withdrawn):**
      *How it currently works:* every 10-digit run (with or without separators)
      that is not inside a longer run is reported as PHONE at 0.98.
      *Where it breaks:* ID numbers that are 10 digits are reported as PHONE
      at 0.98. The scan output (419 spans) shows they reach `Analyze`.
      *Proposed fix:* gate the bare branch on a phone keyword within a short
      window (`phone`, `tel`, `mobile`, `cell`, `call`, `contact`), the same
      way passports are gated. Alternatives: require separators on the bare
      branch (keeps `555-123-4567`, drops `5551234567`), or drop the bare
      branch. Withdrawn as above; if a keyword gate is ever wanted, it needs a labelled policy decision first, and the golden gate would need bare-with-keyword and bare-without-keyword cases. CREDIT_CARD has a similar pattern (18 FPs: 9
      `device_identifier`, 9 `account_number`); not changed in this pass.
- [x] **SSN residual false positives are a label-scope decision, not a regex defect. Decided 2026-10-09: (a), keep as FPs, shown in the headline.**
- [x] **`credit_debit_card` stays in NemotronLabels. Decided 2026-10-09: (c), no removal.** The label keeps the 18 CREDIT_CARD false positives on `device_identifier` and `account_number` counted against precision. Removing the label would hide them, so it was rejected.
      Hand-checked by bucket over the same 2000-row samples `make corpus` scores
      (scratch analysis, not committed). **Nemotron: 25 SSN FP**, every one at a
      span the source labels with an unmapped type: 23 `tax_id`, 1
      `certificate_license_number`, 1 `medical_record_number`. All 25 are dashed,
      in tax-ID wording. **Gretel: 1 SSN FP**, dashed, labelled by nothing.
      Figures below use the overlap policy, so they approximate the exact-span
      report. **Decision needed:**
      - **(a) keep as FPs (chosen).** No change. Nemotron SSN stays P=0.8780.
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
- [ ] **BITCOIN coverage.** Neither corpus labels it, so it stays unmeasured
      on real text. The seed set now has five BITCOIN cases (batch 8). Closing
      this needs a third labelled corpus, which is data, not code.
- [x] **Per-language scoring of gretel. Measured 2026-10-14; answer: recall is lower in every other language.** The per-language `*_test` parquet files are already local, so the seven were converted with `convertParquet` (all rows, no limit), scored with `WithInSchema` and the default scrubber. Exact P / R, and the share of negatives clean, were: English (2891 cases) 0.3652 / 0.7709, 1850 of 2140; French 0.2039 / 0.4205, 295 of 338; German 0.2926 / 0.5154, 299 of 353; Italian 0.3680 / 0.6346, 298 of 346; Spanish 0.4475 / 0.6864, 285 of 324; Swedish 0.1912 / 0.4906, 276 of 345; Dutch 0.2974 / 0.5141, 299 of 339. Recall falls by 8 to 35 points against English in every other language, so the recognizers do worse on non-English text. The cause per type is not diagnosed here. Precision is low everywhere, which the corpus-label caveats above already explain. This was a one-off scratch run, not committed; the converter still reads `combined_test` only (`tools/corpusbench/convert/gretel.go`). Making it a repeatable target would mean a language parameter on the converter, which is a CLI change and is not done.
- [ ] **Corpus results as a CI gate.** Blocked by design: the corpora and their
      converted output are gitignored, so CI cannot see them. The committed gate
      remains the seed set.
- [x] **An end-to-end anonymize check over corpus cases. Done 2026-10-14, narrow reading.** `cmd/corpusanon` (`make corpus-anonymize`) runs `Scrubber.Anonymize` over every converted case in both corpora and fails on the first error or invalid UTF-8 output. It does not compare the output with the gold spans, so it does not need overlaps resolved first. Result: 2000 + 2000 cases, no errors, all output valid UTF-8. It is a developer target, not CI, because the corpora are gitignored.

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
      `AGENTS.md` changes, re-copy the body into the matching `docs/` file. Keep
      the file's frontmatter, and re-apply the four link rewrites when syncing
      `docs/concepts/piiscrub-usage.md`, or its links dangle again. **Synced
      2026-10-14:** all three bodies, the four rewrites, the two stale seed
      counts in the frontmatter and `docs/index.md`, and a `docs/log.md` entry.
      The `doc_write` tool named in earlier notes is not in this repo or this
      session, so the copy was done by hand. Nothing enforces the sync, which
      is the accepted cost of keeping the copies.