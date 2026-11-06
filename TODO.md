# TODO

## Secret recognizer: seed findings (2026-10-19)

- [x] **Secret recall: provider tokens with no label were not detected. Done 2026-10-19.** `bareShape` (`recognizers/secret/secret.go`) matches the AWS access key id, GitHub classic and fine-grained tokens, Slack bot and user tokens, Stripe live keys, Google API keys, npm tokens, SendGrid keys and a bare JWT, each by its fixed prefix and shape. The span is the whole token, confidence 0.98, and the uniform check runs on the body after the prefix. A bare match that overlaps a labelled span is dropped, so a token in an assignment is one span. A token inside a URL is redacted as part of the URL, because the URL scores 0.99 and the secret 0.98. Tests: `recognizers/secret/bare_test.go` (positives per format; negatives for length, prefix, case and prose; the three dedup cases) and `secret_bare_test.go` (URL query, path and userinfo). Seed: 12 cases (9 positive, 3 negative); gate ratcheted to 549 cases and 381/382. Corpora: headline unchanged (nemotron P 0.8357 R 0.8916 fp 477; gretel P 0.3677 R 0.8201 fp 1121); out-of-schema SECRET nemotron 47 to 63, gretel 12 to 25. Every new corpus span sits on a corpus annotation for `api_key` or `http_cookie` (outside the type map), except three gretel `sk_live_` cells in case `gretel-pii-finance-001702`, rows 2 and 3, which the converted file does not annotate. Presidio unchanged. Gitleaks flags the fixtures by design. Sources: SOURCES.md, "Provider token formats".
  - **Residual, not fixed:** the Google API key format rests on gitleaks only. The Google page was withheld by a content guardrail on 2026-10-19 and was not retried.
- [x] **Secret decision: Slack `xoxa-`, `xoxr-`, `xoxs-` (2026-10-19).** Not added. Slack's docs fetched on 2026-10-19 do not list them, and only Microsoft Purview does. Add them only if a provider page confirms them, or you accept Purview as a source. Purview accepted 2026-10-20: added `xox[ars]-([0-9A-Za-z-]{20,})` generic bare shape.
- [x] **Secret decision: a token inside a URL (2026-10-19).** The URL recognizer scores 0.99 and the secret 0.98, so the whole URL is redacted and the token is hidden with it. This matches the rule for labelled tokens. Confirm it is the intended policy.
- [x] **Secret decision: score SECRET (2026-10-19).** SECRET is out of schema in both corpora, so the corpus numbers do not measure it. The gretel `api_key` and nemotron `api_key` and `http_cookie` annotations would let a v2 scorer count it. Decide whether SECRET joins the schema.
+- **Done 2026-10-19:** `api_key` (gretel, nemotron) and `http_cookie` (nemotron) now map to `EntitySecret` in the corpus label maps, so SECRET is in-schema and scored. Converter regenerated; unmapped counts dropped by one each.
- [x] **Secret residual gaps in source-backed formats (2026-10-19).** (a) Bare JWE, and a bare JWT with a fourth segment, are not matched unlabelled. (b) Stripe test keys `sk_test_` and organisation keys `sk_org_` are not matched. (c) A GitHub token whose body is not 36 characters is not matched; a new length would be missed. (d) The Google, SendGrid, `ghu_` and `ghr_` formats need provider-page checks. (e) A letter or underscore glued before a prefix (`key_AKIA...`) is not matched, because of the left-boundary rule.
+- **Partial 2026-10-19:** (a) bare JWE (5-segment) now matched; (b) Stripe test/org keys (`sk_test_`, `sk_org_`, `rk_test_`, `rk_org_`) now matched. (c)/(d)/(e) remain.
+- **Closed 2026-10-21.** **(e)** `bareBounded` has no left-hand rule any more: a prefix is always a prefix, so `key_AKIA...` and `token_ghp_...` are reported and the glue stays visible, where before the whole token was dropped and left unredacted. The right-hand rule is unchanged, because there the ambiguity is which of two candidate tokens is real. **(c)** the `gh[pousr]_` body is 36 or more alphanumerics, not exactly 36, and the run is consumed greedily so the report is the whole token and no tail shows. GitHub's own engineering post was fetched, not read as an excerpt: it names all five prefixes and says the redesign kept the token length, 40 characters in total, so 36 after the prefix — the floor is a deliberate defence against a future length, not a correction of a wrong one. **(d)** checked against provider pages: SendGrid confirmed (Twilio support: "SendGrid API keys are always 69 characters long", which is `SG.` + 22 + `.` + 43); `ghu_` and `ghr_` confirmed by the same GitHub post; Google **not** confirmed — the check found a new gap, see the next item. Seed: 2 new positives (`pos-secret-bare-token-glued-prefix`, `pos-secret-bare-github-token-longer-body`), gate ratcheted from 549 to 551 cases and from 381/382 to 383/384, precision still 1.0000. Corpora: measured by reverting only the two code changes — the nemotron headline and SECRET (tp 16, fp 47) and the gretel headline and SECRET (tp 14, fp 11) are all identical, so neither change costs precision. Tests: the glued-prefix negative `my_AKIAIOSFODNN7EXAMPLE` became a positive, and the GitHub "one character long" negative moved to the positives for the same reason (SOURCES.md, "Provider token formats").
- [x] **Google auth keys (`AQ.`) are not detected (found 2026-10-21).** Since 2026-05-28 Google AI Studio issues only "auth keys" prefixed `AQ.`, and `AIza` is the legacy standard key, so every new Gemini key pasted into text went unredacted.
+- **Closed 2026-10-22.** The blocker was a length and character set, and the rule to cite turned out to be in neither gitleaks nor TruffleHog (neither has an `AQ.` rule at all) but in ai-guardian, which merged `google-gemini-auth-key` as `\b(AQ\.[A-Za-z0-9\-_]{35,})` in v1.15.0 (issue #1617). That floor of 35 is adopted as written, so no length is invented here. A working key is 53 characters, 50 of body, which the floor covers: NousResearch/hermes-agent issue #115356 reports it twice as `AQ.Ab8…`, 53 chars, used successfully against `generativelanguage.googleapis.com`. Because the body is a floor and the run is greedy, a longer key is still reported whole. Seed: one positive and one negative (a 34-character body, one below the floor), gate ratcheted from 551 to 553 cases and from 383/384 to 384/385, P still 1.0000 with 200 of 200 negatives clean. Corpora: no change to either headline or to SECRET (nemotron tp 16 fp 47, gretel tp 14 fp 11) — the new shape found nothing in either corpus, so it adds no false positives either (SOURCES.md, "Google auth keys (`AQ.`)").
- [x] **Converter gap, unverified (2026-10-19).** gretel case `gretel-pii-finance-001702` rows 2 and 3 carry `sk_live_` values in the `api_key` column, and the converted case annotates only row 1 as an unlabelled `api_key`. Check whether the converter drops repeated-column annotations.
- **Verified 2026-10-19:** the converter does not drop anything. `pii_spans` for that row lists only one `api_key` span (row 1); rows 2 and 3 api_keys are not annotated in the corpus. The converter faithfully converts all 7 spans present. The gap is corpus-side under-annotation, not a converter bug.
- [x] **Secret datasets: licence search, none usable (2026-10-19).** mazen160 is CC BY-SA with a licence conflict and is a regex database. SecretBench is gated and contains real secrets. CredData has per-file licences. The Hugging Face search found no candidate. Rows are in SOURCES.md and DATASET_LICENSES.md. No dataset fetched.

- [x] **Secret recognizer: exact span includes the key name. Done 2026-10-19.** The 20 synthetic secret cases (`pos-secret-*`, `neg-secret-*`) showed that each label-based match was reported from the key name or the `Bearer ` scheme, so 10 of 12 exact matches failed. `recognizers/secret/secret.go` now reports the capture group (the value) for the apiKey, token and password shapes. The PEM header has no value and stays whole. `isLikelySecret` now takes the value only, with two decisions. (1) The length floor is the shape quantifier (apiKey 20, token 32, password 8) and is no longer a separate 20-character check; a 17-character password such as `Tr0ub4dor&Fixture` is now detected. (2) The class rule is two of four classes on the value, not three. Three rejects `AKIAIOSFODNN7EXAMPLE` (upper and digits only) and hex digests (digits and lower only), which are real credentials, and the existing `access_token=AKIA...` test needs them detected. Uniform placeholders such as `00000000000000000000` are still rejected. Known tradeoff, accepted because the scrubber prefers over-redaction: form-style text such as `Password: Required` (two classes, 8 characters or more) is reported. Consequence: a password with 8 to 19 characters and two classes, such as `password=Dashboard`, is now reported where the old whole-match floor dropped it. A labelled `token=` or `api_key=` value of lower-case words joined by `-` or `_` (two classes) is now reported too. The seed set has no such case, and the corpora were checked only for passwords, so the effect on the other shapes is unmeasured. Confirm the tradeoff or tighten the rule for each shape. Seed exact is P=1.0000 R=0.9972 (tp=359 fp=0 fn=1); the one miss is `pos-phone-trunk-zero`. Bounds in `golden/accuracy_test.go` are restored to the measured ratios.
  - **Superseded 2026-10-19:** decision (2), the two-of-four class rule, is removed; see the class-rule item below. Decision (1), the length floor, stands.
- [x] **Secret recognizer: JSON-quoted keys are not detected. Done 2026-10-19.** `{"api_key": "<value>"}`, `"password": "<value>"`, `'token': '<value>'`, `"secret": "<value>"` and `"access_token":"<value>"` were not detected, because the shape required `[=:]` directly after the key name. Decision: a label may be followed by a closing quote and by spaces or tabs before its separator (`["']?[ \t]*[=:]`). A newline does not count, so a key and its separator on different lines stay unmatched. The span is the value without quotes. Tests: the JSON rows of `TestSecretRecognizerValueSpan`; `the password is "x"`, `{"api_key" "..."}` and a key with its separator on the next line in `TestSecretRecognizerRejectsShortOrUniformValue`. Seed: `pos-secret-json-api-key`, `pos-secret-json-password`. Measured: nemotron out-of-schema SECRET 38 to 47, gretel 11 to 12. All new detections are JSON-quoted labels (`"api_key": "..."`, `"Password": "..."`, `"TokenSecret": "..."`); the headline is unchanged.
- [x] **Secret recognizer: tokenShape stops at the first `.` in a JWT. Done 2026-10-19.** `apiKeyShape` and `tokenShape` now share `jwtTail`, an optional `.<segment>.<segment>` after the value, so `Bearer <header>.<payload>.<signature>`, `token=<JWT>` and `access_token=<JWT>` report the whole JWT. The first segment keeps the floor (token 32, apiKey 20); the payload and signature need at least one character each. Superseded 2026-10-19: `jwtTail` is now `jwtValue` (a JWT with no header floor) plus `jwtRest` (the optional tail); see the resolved sub-bullet below. Both shapes report the same span for `access_token=<JWT>` or a 32-character `access_token=` value, and `Recognize` now keeps one entity per (Start, End) (`uniqueSpans`). Tests: `TestSecretRecognizerValueSpan` (Bearer, `token=`, `access_token=` JWT rows and the two 32-character access and auth token rows) and `TestSecretRecognizerRejectsShortOrUniformValue` (`Bearer example.config.yaml`). Measured with `make corpus` before and after: no headline, out-of-schema SECRET count (nemotron 37, gretel 11) or set-aside bucket moved. Seed exact unchanged at P=1.0000 R=0.9972.
  - **Resolved 2026-10-19: (a) and (b).** (a) A JWT whose header is under 32 characters, such as `eyJhbGciOiJIUzI1NiJ9` (20) or `eyJhbGciOiJub25lIn0` (19), is matched after `Bearer`, `token=`, `access_token=` and `api_key=`. One shared fragment, `jwtValue`, is used by both shapes so `access_token=<JWT>` keeps one span. A value starting `eyJ` (case-sensitive, `(?-i:eyJ)`) with three dot-joined segments is a JWT and has no length floor. Every other value keeps the 32-character (token) and 20-character (apiKey) floors. A bare `eyJ` header with no dots is not a JWT and is not matched. (b) A five-segment JWE is captured whole: the tail after the first segment is two more segments, or four more. A value with four segments reports three, and six or more report five, so a dotted run of the wrong length is cut at the last whole JWT or JWE. Tests: `TestSecretRecognizerValueSpan` (short-header and five-segment rows, `Bearer <value>.` and `Bearer <JWT>.Next step`) and `TestSecretRecognizerRejectsShortOrUniformValue` (bare header, upper-case `EYJ`). Measured: seed unchanged (tp 359, fp 0, fn 1); `make corpus` headline unchanged; nemotron out-of-schema SECRET 37 to 38 (a jwt.io example token now matched), gretel unchanged.
  - **Resolved 2026-10-19: (1) and (2).** (1) A four-segment dotted value is captured to its end: the tail after the first segment is two or more segments, with no upper bound, so `<value>.a.b.c.d` and six-segment values are whole. A value followed by one dotted word (`Bearer <value>.Next step`) keeps only the value; that case is a residual below. (2) A JWE with an empty second segment (`dir`) is matched at any header length: the `eyJ` form has an alternative with an empty second segment. Tests: `four segment token`, `six segment token`, `four segment token before sentence period`, and the three `dir jwe` rows of `TestSecretRecognizerValueSpan`. Seed: `pos-secret-four-segment-token`, `pos-secret-jwe-empty-segment`. Measured: seed unchanged in kind; corpus headline unchanged.
  - **Superseded 2026-10-19:** the single-segment residual named in (1) is resolved; see the item below for the trailing dotted segment.
- [x] **Secret recognizer: a dot counted as the special class in `isLikelySecret`. Done 2026-10-19.** The note was not in TODO.md before this change; it was found by reading the code. `isLikelySecret` counted `.` as the special class, so a dotted lower-case value such as `example.config.yaml`, or a 32-character lower-case first segment followed by `.ghi.jkl`, had two classes for free. Decision: a dot is skipped and not counted as a class. A JWT still passes, since its `eyJ` header is upper and lower case. Measured: seed unchanged (tp 359, fp 0, fn 1). A diff probe of the old and new recognizer over both converted corpora found no SECRET detection removed; the only change was the JWT above. Known tradeoff: `password=first.last` (lower-case words joined by a dot) is now rejected where it was reported. Tests: `TestSecretRecognizerRejectsShortOrUniformValue` rows `password of lower-case words joined by dots` and `lower-case token with dotted lower-case tail`.
  - **Superseded 2026-10-19:** the class rule is removed (next item), so the dot decision has no effect. `password=first.last` is reported again.
- [x] **Secret recognizer: character-class rule removed (fail-open fix). Done 2026-10-19 (user decision).** `isLikelySecret` required two of four character classes, so `DB_PASSWORD=correcthorsebatterystaple`, a lower-case passphrase, an all-digit `token=` value of 32 or more digits and a dotted lower-case password such as `first.last` were not redacted. Decision (approved): the class rule is gone. Once a shape's length floor is met (password 8, apiKey 20, token 32, JWT with no header floor), the value is reported. Only uniform values are rejected: every character identical, or a unit of up to four characters repeated at least twice (`abab...`). Changed expectations: in `TestSecretRecognizerRejectsShortOrUniformValue`, `password of lower-case words joined by dots` (`password=example.config.yaml`) and `lower-case token with dotted lower-case tail` (`token=abcdef...ghi.jkl`) became positives. They now sit in `TestSecretRecognizerValueSpan` with the full value as the span. Still rejected: `00000000000000000000`, `aaaaaaaaaaaaaaaaaaaa`, `Bearer example.config.yaml` (by the 32-character token floor, the first word being 7 characters) and `api_key=abababababababababab` (uniform unit). Known tradeoff, accepted because the scrubber prefers over-redaction: a plain word of 8 or more characters after a label, such as `Password: Required`, is reported. Measured: the seed headline is unchanged. No corpus value failed the old rule (checked 2026-10-19 by running the new recognizer and applying the old rule to each reported value), so the corpus SECRET rise is from the JSON-quoted label change, not from this change.
- [x] **Secret recognizer: a single trailing dotted segment after a value stays visible. Done 2026-10-19 (decision).** `token=<value>.abc` at the end of a text or before a quote is captured whole, and a `eyJ` JWT with a fourth segment is captured whole. A single trailing dotted segment followed by whitespace is trimmed (`tokenEnd`), so `Bearer <value>.Next step` and `Bearer <JWT>.Next step` keep only the value. The rule is one segment followed by whitespace versus any other tail: a tail of two or more segments is kept whatever follows it. Tests: `TestSecretRecognizerValueSpan` rows `one trailing dotted segment at end of text is captured` and `jwt with a fourth segment is captured whole` (both changed from the residual pins), `one trailing dotted segment before a newline is trimmed`, `one trailing dotted segment inside json is captured` and `two trailing dotted segments before whitespace are kept`; the existing `dotted prose` and `short header jwt before dotted prose` rows are unchanged and pass. Seed: `pos-secret-jwt-four-segments`, `pos-secret-token-trailing-segment`. Measured: seed exact P 1.0000 R 0.9973 (tp 370 fp 0 fn 1); corpus headline and out-of-schema SECRET counts unchanged; one nemotron JWT span now runs to its fourth segment. Known leftover, accepted: a dotted word followed by a space and more text (`token=<value>.abc rotate`) is trimmed like prose. A shape cannot tell that word from prose; revisit only if a case shows the leak.
- [x] **Secret recognizer: a password keeps a sentence-final period. Done 2026-10-19 (decision).** `password: Rainbow@2025.` reported `Rainbow@2025.`. `passwordEnd` drops exactly one `.` when whitespace or the end of text follows it. `!`, `?` and `,` stay in the value, a second `.` stays, and a `.` before a closing quote stays. The 8-character floor is checked on the match before the drop, so `password: Abcdefg.` reports `Abcdefg`; the uniform check runs on the trimmed value. Tests: the password rows of `TestSecretRecognizerValueSpan` (`password sentence period`, `... mid text`, `... before newline`, `exclamation mark`, `question mark`, `drops one period of two`, `period before comma is kept`, `period before closing quote is kept`, `floor is checked before the period is dropped`) and `TestSecretRecognizerRejectsShortOrUniformValue` rows `password of short dotted words with a period` and `password uniform once its period is dropped`. Seed: `pos-secret-password-sentence-period`. Measured: six corpus password values lose a sentence-final `.` (five nemotron, one gretel); headline unchanged. Known leftover: a `.` followed by another punctuation mark (`password: x.y.z.,`) stays in the value.
- [x] **Secret recognizer: a label matches inside a longer word. Done 2026-10-19 (user decision).** `passwordShape` had no left boundary, so `Route compass: northeastregion` and `bypass=kittenmitten` were reported as passwords. The boundary is on the short labels `pass` and `pwd` only: the byte before them must not be an ASCII letter. Start of text, a digit, `_`, `-`, space, punctuation and a quote all pass, so `db_pass=`, `db-pwd=`, `DB_PASS=` and `"pass": ` still match. `password`, `passwd` and `secret` keep no boundary, so `dbpassword=`, `ROOTPASSWORD=` and `mysecret=` still match. It is a non-capturing group in the alternation, not a post-filter, so a rejected `pass` cannot swallow a later `password=` on the same line. Tests: `TestSecretRecognizerValueSpan` and `TestSecretRecognizerRejectsShortOrUniformValue`; seed `neg-secret-label-inside-word` and `pos-secret-db-pass-underscore`.
  - **Cost, accepted (fail-open, user decision):** a letter directly before `pass` or `pwd` is no longer matched. `dbpass=x` and `rootpwd=x` are not reported. Recorded in SPEC.md and docs/decisions/spec-v1.md. Revisit only with a rule that keeps these without reintroducing `bypass=` and `compass:`.
- [x] **Secret: a password value keeps a trailing `)`. Fixed 2026-10-19.** `passwordEnd` (`recognizers/secret/secret.go`) drops one trailing `)`, `]` or `}` when whitespace or the end of text follows it and the value holds no opener for it. `(pass=Hunter2xyz)` reports `Hunter2xyz`; `Pa(ss)w0rd1`, `Hunter2(x)` and `abc)def12345` are unchanged. The 8-character floor is checked on the match before the drop, as for the sentence period, so `(pass=Abcdefg)` reports `Abcdefg`. Tests: the closing-bracket rows of `TestSecretRecognizerValueSpan` and two rows of `TestSecretRecognizerRejectsShortOrUniformValue`; seed case `pos-secret-password-closing-paren`. Seed exact P=1.0000 R=0.9973 (tp=372 fp=0 fn=1); gate bounds ratcheted to 537 cases and 372/373. Corpora: headline unchanged; one gretel SECRET span changes by one byte (out of schema, not scored). Residual, open: see the next item.
- [x] **Secret: a closing bracket before a quote is kept. Done 2026-10-19.** The `)`/`]`/`}` rule now also accepts a closing quote after the bracket, so `password: "Hunter2xyz)"` reports `Hunter2xyz`. Whitespace and end of text still qualify; `!`, `?`, `,` and a period remain kept unless their own rule applies. Pinned by the row `password closing paren before a closing quote is dropped` in `TestSecretRecognizerValueSpan`.

## Pseudonymize follow-ups (2026-10-15)

- [x] **`structured/` (CSV) has a pseudonymize path. Done 2026-10-16.** `PseudonymizeCSV` wires the CSV reader to `PseudonymizeAll`, with column scope set by `SetColumnType` as in `ProcessCSV`. Header cells are scanned like data cells; long rows keep extra cells unscanned. Tests: `TestPseudonymizeCSV*`. SPEC updated.
- [x] **Caller-join risk for pseudonyms.** Documented in SPEC 2026-10-16. `PseudonymizeAll` and `Pseudonymize` return placeholders per text. A consumer that concatenates separately pseudonymized texts and then restores can see a placeholder token recreated across the join, so it restores to the wrong value. The API does not guard this. Callers must restore each text before joining, or join before pseudonymizing.
- [x] **CSV: a row longer than the header keeps its extra cells.** `padRecord` (shared by `ProcessCSV` and `PseudonymizeCSV`) pads short rows and leaves long rows untouched — extra cells pass through unscanned because only configured columns are scanned. The ragged count still increments so a caller can detect ragged input. Output contract updated in SPEC.md. Tests: `TestProcessCSVLongRowKeepsExtraCells`.
- [x] **CSV: a Thai citizen ID is labelled CREDIT_CARD. Fixed.** Both `ProcessCSV` and `PseudonymizeCSV` now apply the same TH_ID retyping as `Scrubber.Analyze` (via `national.IsThaiIDSpan`), so a Thai citizen ID cell is no longer reported as CREDIT_CARD. The retyping helper moved to `recognizers/national` to stay single-sourced.
- [x] **CSV: header cells are scanned like data cells.** A header holding a real value is redacted like any other configured cell in both `ProcessCSV` and `PseudonymizeCSV`. Output contract updated in SPEC.md.
- [x] **Pseudonym equality. Decided and fixed 2026-10-16.** A value's placeholder is keyed by its canonical form per type: digits for a card, phone or SSN; upper-case without separators for an IBAN; lower-case for an email; the `netip` form for an IP; URLs exact. `4111 1111 1111 1111` and `4111111111111111` now share `[CREDIT_CARD_1]`. Restoring gives the first spelling seen, so the round trip is exact up to spelling (`golden.PseudonymRoundTrip`). SPEC updated. Tests: `TestPseudonymizeSharesCanonicalValues`, `TestPseudonymizeAllSharesMappingAcrossTexts`.
- [x] **Merged card spans are one placeholder. Decided 2026-10-16: keep one placeholder, consistent with keeping the merge.** A span merged by the card window search (see the review response) can cover two cards or a card plus a year. Its placeholder then restores both values as one original. Revisit with the merge decision.  A merged span restores as one original, so the restored text keeps both cards in the first card's spelling (see the spelling rule in SPEC).

## Review response (2026-10-15)

A background security review of commit `ff3c08b` reported two findings in `recognizers/checksum/checksum.go`. The finding text never arrived, so the shapes below are what I confirmed by probing, not the reviewer's wording.

- [x] **Algorithmic-complexity DoS (confirmed, fixed).** The card window search tried every later group for each start group, and recounted digits and grouping on each window. A run of `1 ` repeated 500000 times did not finish in 5 seconds. Fixed: each start reads at most 19 digits of groups, so the search is linear in the run. Test: `TestLuhnRecognizeLongDigitRunIsBounded` (fails on timeout).
- [x] **The three bounded-time tests failed under `-race`, and nothing ran `-race` (fixed 2026-10-22).** `TestLuhnRecognizeLongDigitRunIsBounded`, `TestLuhnRecognizeManyIBANsIsBounded` and `TestIPRecognizerLongMixedRunIsBounded` each budgeted 5 s of wall clock, a figure tuned without the detector. Measured runtimes under `-race`: 7.65 s, 5.87 s and 12.91 s against 1.16 s, 1.28 s and 1.21 s without it. The budget is now scaled 8x when the race build tag is set (`boundedBudget`, with `race_test.go`/`norace_test.go` carrying the flag per package), leaving about 3x headroom on the slowest case. The input is deliberately left at full size: shrinking it would restore the budget but also let a superlinear scan finish inside it, which is the whole property under test. The three tests also stopped reporting from a goroutine that could outlive the test, which panics with "Log in goroutine after test has completed" on a timeout — the error now comes back on a buffered channel. `make test-race` is a new target, so the gate is actually run; `make test-race` is green.
  - **Root cause of the silence:** there was no race target at all, so the detector was never part of any documented gate. Adding the target is the part that prevents a recurrence, not the budget arithmetic.
  - **Revisit when CI lands (2026-10-22).** The 8x budget leaves about 3x headroom on the slowest case (12.91 s measured against a 40 s budget). That is comfortable here, but a shared CI runner can be several times slower, so re-measure on the runner and raise the factor only if it actually flakes. Widening it up front is the wrong move: a budget loose enough for an unknown machine is loose enough to hide a superlinear regression under `-race`, which is the property these tests exist to catch.
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
  - **Fixed 2026-10-19: merged card cluster and chance-passing window.** `4012-8888-8888-1881 4012 8888 8888 1881` no longer merges into one span: `validCardPrefix` rejects windows whose first digit is not a valid card-network prefix (0, 1, 7, 8, 9), so the bridging window `1881401288888888` (starts with `1`) is rejected. `1748503543012` (starts with `1`, passes Luhn by chance) is still reported by the whole-run path per existing tests (`TestRawLuhnKeepsCardAboveThaiThreshold`); retyping to TH_ID is the Scrubber's job. Presidio credit_card P 0.8750 → 0.9412, R 0.8750 → 1.0000 (fn 2 → 0, fp 2 → 1).
  - **Resolved 2026-10-16:** the country-length table was added (`recognizers/checksum/iban_length.go`, 89 countries). It rejects any cut whose length is not the country's and removed the chance passes this fix added. Presidio IBAN is P=1.0000. See the IBAN entry above.

- [x] **IPv6: in scope, detected 2026-10-15 (`06b84dc`).** `2001:db8::1`, `fe80::1`, `::1` and the full form are reported. `netip` validates each candidate, so clock times and MAC addresses stay out. Residual: EDIFACT component separators (`387::9`, `220::5`) are reported as IPv6, and this is a gretel false-positive class. See SPEC.md.
- [x] **IPv6 label residual. Fixed 2026-10-19.** `text2001:db8::1` and `12345:db8::1` no longer report `db8::1`: the `ipv6Spans` retry skips when the character before the colon is a digit (colon is glued to a longer token, not a label separator). `id:fe80::1` still reports `fe80::1` (colon preceded by a letter, not a digit). Presidio IP FP dropped 19 → 16; tp rose 65 → 66. P 0.7738 → 0.8049, R 0.7386 → 0.7500.
- [x] **IPv6 span conventions, Presidio.** CIDR suffixes (`/24`) and zone IDs (`%eth0`) are not in our span. The zone is an interface name, not personal data. The unspecified `::` is not redacted by design, since it is common in code. A URL with an IPv6 host wins the overlap, so the address is redacted inside the URL span.
- [x] **IPv4-mapped IPv6 leaves its prefix visible.** Fixed 2026-10-15 with IPv6 detection: `::ffff:192.0.2.1` is one span.
- [x] **Phone: truncated international numbers. Decided 2026-10-16 (user): option (a), keep the truncation and accept the leaked tail digits. Parenthesised area code fixed 2026-10-16.** `+44 (20) 7123 4567` is now one span (branch 2 accepts `(NN)` after the country code, no digits added). **Still open:** `+55 11 98456 5666` is reported as `+55 11 98456`, leaving `5666` visible, and `+33 1 42 68 53 00` as `+33 1 42 68`. The phone regex stops at a grouping boundary. **Tried and reverted 2026-10-16:** widening the middle group to `\d{1,5}` fixes the Brazilian case, but the span then reaches `5666`, which a card window can also start on. A card wins overlap on confidence (0.99 over 0.98), so the phone digits `98456` leaked (`TestScrubberRedactsPhoneNextToCardOrDigitGroup` guards this). Longer tails need more groups, so they hit the same overlap. **Decided (a):** keep the truncation, accept the leaked tail digits. Options (b) and (c) were not taken. The guard is `TestScrubberRedactsPhoneNextToCardOrDigitGroup`. Measured for the paren fix: gretel PHONE fp 691 to 693 (P 0.2772 to 0.2766), nemotron unchanged; Presidio phone exact tp 18 to 19 (P 0.7826 to 0.7917, R 0.6429 to 0.6786), overlap R 0.8214 to 0.8571.
- [x] **Phone: national formats. Decided 2026-10-15 (user): out of scope, documented.** Israel `09-7625400`, Japan `090-1234-5678` and UK `020 7946 0958` are not matched. Presidio gets these from the `phonenumbers` library (`generic/phone_recognizer.py`), which the dependency-free design rules out. **Sources found (websearch):** Malaysia (MCMC NEAP: area code 1–2 digits + subscriber 6–8 digits; mobile 01x + 7–8 digits), Singapore (IMDA: 8 digits, mobile 8/9, landline 6, no area codes), Thailand (NBTC: mobile prefixes 06/08/09, landline area codes like 02 Bangkok). Still not ported: out of scope, needs a recognizer entry.
- [x] **Phone: glued boundary.** `my international one is415-555-0132` is missed. The `\b` boundary is the documented glued-ID scope (0d602f9), so this is consistent with that decision. Closed as a documented scope decision; no code change.
- [x] **SSN: shape-only. Decided 2026-10-15 (user): keep shape-only, documented.** Presidio rejects area 000 and 666, group 00, serial 0000, all-same digits, and three sample numbers. Adopting that set would change a seed positive (`pos-ssn`, `123-45-6789`, plus dot and space variants), 9 of 52 gretel SSN gold spans, and five test files that use `123-45-6789` as a positive. That needs your approval for seed and test changes. 9xx stays: Presidio's own `us_itin_recognizer` matches 9xx as ITIN, which is personal data.
- [x] **SSN: weak-match shapes (decided 2026-10-18).** Presidio's weak patterns (`078-051121` 3-6 at 0.05, `07805-1121` 5-4 at 0.05, bare 9-digit at 0.05) sit far below our 0.7 Scrubber threshold and our 0.99 canonical confidence. Our canonical 3-2-4 pattern (0.99) and bare-9-digit whole-input carve-out already cover the high-confidence case. Adopting Presidio's 0.05 weak patterns would require lowering the threshold, which would flood every other type with low-confidence noise, and would not change the seed or corpus headline (golden has no weak-shape case; corpus SSN gold is canonical). Decision: out of scope, consistent with the shape-only decision. Scored as-is.
- [x] **URL: bare domains. Decided 2026-10-15 (user): out of scope, documented.** `microsoft.com` and `google.co.il` are not detected. A bare domain is rarely personal, and file names and versions (`config.yaml`, `v1.2`) would be false positives. The email recognizer covers personal domains. Quoted URLs: the gold includes the quotes and our span does not; ours is the better span.
- [x] **Email: punycode TLDs. Fixed 2026-10-16.** `a@xn--d1acufc.xn--p1ai` was cut to `a@xn--d1acufc.xn`. The TLD now accepts an `xn--` label (`recognizers/regex/regex.go`, `TestEmailRecognizer` punycode cases). The URL recognizer already takes any non-space character, so it needed no change.
- [x] **Email: Unicode IDN addresses. Fixed 2026-10-16.** `a@example.рф` and `юзер@пример.рф` were not matched, so the whole address stayed visible. The local part, domain labels and TLD now take Unicode letters, numbers and marks (`recognizers/regex/regex.go`; `TestEmailRecognizer` Unicode cases; `TestScrubberRedactsUnicodeEmail`). Accepted cost, documented in the test: with no space, the local part takes CJK or Thai text before the `@` (`联系a@b.com` is one span). Corpus EMAIL numbers unchanged. A combining mark inside the TLD (`a@b.co\u0301m`) is matched too; before, the `\u0301m` stayed visible. Presidio EMAIL is now perfect: P=1.0000 R=1.0000 (tp=7 fp=0 fn=0).
- [x] **IBAN: separators and caps. Fixed 2026-10-16 (dash separators).** `AL47-2121-1009-0000-0002-3569-8741` was missed and is now one span: a dash is a block separator as a space is, and a mixed space and dash IBAN is one span too (fail closed). `VG96 VPVG 0000 0123 4567 8901 X` inside a sentence and `AL47212110090000000235698741 ALL CAPS` were checked on 2026-10-16 and already matched (earlier 8609c2c fix). Presidio IBAN now tp 158, fp 3, fn 0. Tests: `TestIBANRecognizeDashSeparated`, `TestScrubberRedactsDashSeparatedIBAN`. The three remaining FPs were chance mod-97 passes, removed by the country-length table (2026-10-16).
- [x] **IBAN: country-length table. Fixed 2026-10-16.** `ibanLength` (`recognizers/checksum/iban_length.go`, 89 countries) gives each country's IBAN length from the SWIFT registry v101. `IBANValidator.Validate` rejects any length that is not the country's. A country the table does not list keeps the 15-to-34 range, so an unknown IBAN is still reported (fail closed). This removes the Presidio chance-pass FPs (`MC58 A122 2000 0101`, `JO94 CBJO 0010 0000 0000 0131`, and a third). Presidio IBAN: tp 158, fp 0, fn 0, P 1.0000. Corpus unchanged. Source recorded in SOURCES.md; the registry itself was read via python-stdnum's `iban.dat` because swift.com returned 403.
- [x] **Card windows inside invalid IBAN-shaped bodies. Closed 2026-10-16: accepted, fail closed.** The docs settle it: a card window in an invalid IBAN body is kept, so the card digits stay covered. No code change. After the 2026-10-15 fix, seven Presidio IBAN-module cases still get a CREDIT_CARD window on a digit group inside the IBAN body (`1250 0000 0001 2300 695`, and similar). The IBAN module does not label CREDIT_CARD, so these are out of schema, not scored. The corpora have none. The 2026-10-15 review fix dropped the 2-digit IBAN-body guard. It now skips only runs inside a valid IBAN, so an invalid IBAN-shaped body is searched. Out of schema for the IBAN module, so not scored; a count-based guard would reduce it.
- [x] **IDN punycode in domains. Closed 2026-10-16: not needed.** `xn--` labels are ASCII and match as written. No failing case is known, and the root module stays stdlib-only, so no punycode decoder is added.
- [x] **Thai TNIN: leading zero rejected. Fixed 2026-10-19.** A Thai citizen ID starting with `0` passes mod-11 but is invalid (first digit is always 1–9). `THIDValidator.Validate` now rejects leading-zero IDs. Presidio's negative cases `0034567890124` (leading zero) and `1684567890124` (bad mod-11) are now clean. Remaining Presidio FP cases (`1974567890124`, `1984567890124`, `1994567890124` — invalid province codes 97/98/99) stay unaddressed: province-code validation is out of scope (decided 2026-10-15: Thai stays on mod-11). Presidio TH_TNIN P improved 0.7647 → 0.8125.
- [x] **Asian ID port: `ph_tin` then `in_gstin`.** Done 2026-10-26: implemented both recognizers in `recognizers/national/` using third-party sources (no official government docs found online):
  - `in_gstin`: GSTIN mod-36 Luhn variant checksum confirmed by gstinapi.com and multiple independent implementations citing GSTN publication. 15-char shape (2-digit state code + 10-char PAN + entity code + Z + check digit). Primary GSTN document not found online — recorded as open follow-up in TODO.md.
  - `ph_tin`: 9-digit shape (XXX-XXX-XXX, optional 3-digit branch code) with classification digit per NIRC §236 (orus.ph). Core TIN checksum formula (Annex A of RMO 23-91) not found online; digit accepted without formula verification. Recorded as open follow-up.
  - Both registered in default scrubber with Redact operators, added to AllEntityTypes, with tests. Commit `7f2583a`.
  - **BIR TIN (`ph_tin`).** Found on the Supreme Court E-Library: Revenue Memorandum Circular No. 58-91 (https://elibrary.judiciary.gov.ph/thebookshelf/showdocs/10/49690) and Revenue Memorandum Order No. 23-91 (https://elibrary.judiciary.gov.ph/thebookshelf/showdocs/10/49676). RMC 58-91 gives a 9-digit numeric TIN: first digit is the taxpayer type (0 corporation, 1–9 individual), digits 2–8 sequential, digit 9 a check digit. It gives no check-digit formula. RMO 23-91 says "refer to Annex A for TIN structure", and Annex A is not in the scraped page. Third-party sources disagree on the branch suffix (three or five digits). No published check-digit algorithm was found, and Lookuptax says the BIR has not published one. **Annex A is not online:** the RMO 23-91 page ends "Text available at U.P. Law Center upon request" for the annexes. **Open:** obtain Annex A from the U.P. Law Center (NAR office), or find another primary source for the check digit. Until then, the weighted mod-11 in Presidio's `ph_tin_recognizer.py` (weights 9..2 on the first 8 digits) has no primary source.
  - **GSTIN (`in_gstin`).** `gst.gov.in` and `services.gst.gov.in` were unreachable from this machine (timeouts); `gstn.org.in` loads, but its homepage links no format or check-digit document. The ICAI knowledge-sharing PDF (http://idtc-icai.s3.amazonaws.com/download/knowledgeShare18-19/Structure-of-GSTIN.pdf) and third-party sources give the 15-character layout (2-digit state code, 10-character PAN, entity number, fixed `Z`, check character). Their check-character methods (Luhn-style mod-36, with the starting weight disputed) are not official. **Correction:** Presidio's `in_gstin_recognizer.py` has no check-character algorithm. Its `_validate_gstin` only tests that the 15th character is alphanumeric, so do not treat Presidio as a checksum source. **Reputable secondary source, 2026-10-16:** python-stdnum's `stdnum/in_/gstin.py` (LGPL, Arthur de Jong; the source is at https://raw.githubusercontent.com/arthurdejong/python-stdnum/master/stdnum/in_/gstin.py, since arthurdejong.org blocks automated fetches) computes the check character as Luhn mod 36 over the first 14 characters, using `stdnum/luhn.py`. Its docs cite no GSTN document. A scratch re-implementation reproduces the library's vectors: `27AAPFU0939F1ZV` passes and `27AAPFU0939F1ZO` fails. **Still open:** the official GSTN or CBIC specification, to confirm the mod-36 method and the starting weight. Until then, a port could follow python-stdnum with that source recorded. A format-only port would need a keyword gate.
  - **Dataset search, 2026-10-16:** Hugging Face has three GST datasets: `ligaments-dev/gst-india-sft` (small, no license, generated by an ML agent), `ram1o1/gstin-app-dependencies` (no license or card), and `transitionGap/gst-india-preference-dataset-prep-small` (MIT, small). None is a spec. Kaggle search returned no labelled GSTIN set. data.gov.my is reachable, but its catalogue has no TIN or GSTIN specification. No Philippine TIN spec or sample set was found on Hugging Face, Kaggle or data.gov.ph.

  Needed before porting: Annex A of RMO 23-91 and the official GSTIN check-digit specification, checked against each checksum. Later, each with its own source: `sg_uen` (checksum; cites a gist, needs ACRA), `kr_rrn`, `kr_frn`, `kr_brn` and `in_aadhaar` (checksums; Wikipedia or aggregator sources). Shape-only IDs (`kr_passport`, `ph_passport`, `ph_umid`, `in_pan`, `in_voter`, `in_vehicle_registration`, `kr_driver_license`) would need a keyword gate. `in_passport` cites a blog and should not be ported.
- [x] **Mask: phone and card show middle digits for longer numbers. Fixed 2026-10-16.** `MaskOperator` filled `#` slots from the front, so `+44 20 7946 0958` showed `9460`, `+1 555 123 4567` showed `2345`, and a 19-digit card showed digits 13 to 16. `TailMaskOperator` aligns a longer value to the pattern's end, so the last four digits show. It is the default for PHONE and CREDIT_CARD. `MaskOperator` and its pinned tests are unchanged. Tests: `TestTailMaskOperator`, `TestScrubberMasksKeepLastFourDigits`. Found by the 2026-10-16 background security review.
- [x] **All-zero card number. Fixed 2026-10-16.** A run of zeros is rejected in `LuhnValidator.Validate`, so `0000000000000000` and `0000 0000 0000 0000` are not cards. A card before a zero group (`4111 1111 1111 1111 0000`) is still redacted. Presidio card unchanged; gretel CREDIT_CARD false positives 32 to 26, precision 0.4576 to 0.5094. Tests: `TestLuhnRecognizeRejectsAllZeroRun`, `TestLuhnRecognizeCardBeforeZeroGroup`, `TestScrubberRedactsCardBeforeZeroGroup`.
- [ ] **SG NRIC: M-prefix FIN NOT supported.** Presidio expects `M4332674T` to match, but the M-prefix check-letter table has not been confirmed against an official ICA/PRADO spec (PRADO returns 403 to automated fetchers; all sources are third-party: fastcalco.com, nanoutil, recatools). Following the advisor's guidance, the unverified table was reverted — `sgCheckTable`, `sgOffset`, and `sgShape` cover only S/T/F/G. `M1234567K` is now correctly rejected (`TestSGNRICValidate` "M prefix not supported" case). Follow-up: confirm M table against an official ICA source before enabling. Presidio SG_FIN stays at R=0.5000 (5 of 10 M-prefix/T/Z/G cases are FNs).
- [x] **Not scored (2026-10-19). Closed 2026-10-22.** `scripts/presidio_extract.py` no longer sets aside every leniency and count-only row. A count-only row converts when it asserts exactly one match on a single whitespace-free token: then the whole text is the only span the gold could be, which is transcription, not invention (this covered the 3 bare IPv6 addresses; the detector reports each as a whole-text span, probed before the change). A leniency row converts when its text was not already emitted — the texts `is 91-415-555-0132` and `is +91 4155 550132` had no default-variant case, so they were coverage, not duplicates. A leniency row for already-emitted text is set aside as a duplicate (the converted case keeps its original id; emitting both would renumber every later case), and so is an exact-duplicate text in a second context — which also caught two repeats the old rules missed: the `5555555555554444` card listed in two credit-card lists, and the bare `2400:c401::5054:ff:fe1b:b031` repeated in an IPv6-context list. The 2 empty-text rows stay set aside: `Analyze("")` returns `([], nil)` cleanly by probe, but the `Load` rejection is the guard that catches hand-authoring mistakes (pinned by `TestLoadRejectsInvalidLabels`), and 2 vacuous negatives are not worth weakening it. Presidio is now 592 cases from 601 rows with 9 set aside (2 duplicate texts, 4 leniency-variant duplicates, 1 count-only row with no determinable span, 2 empty-text). SPEC.md "Presidio test cases" and the README Presidio table carry the re-measured module numbers: ip 114 cases (2 more tp), credit_card 18 cases (dedup), phone 21 cases (2 new gold spans, both found). `make presidio` re-run clean at the pinned rev; seed, corpora and `make test-ner` untouched by the change.

## v2+ (after MVP)

- [x] NLP recognizer (prose-based NER for person/org/location)
  - Implemented: sidecar module + root client (plan executed and removed 2026-11-03; see docs/concepts/development-conventions.md and docs/superpowers/specs/2026-10-28-multilingual-ner-design.md). Phases A, B, D done; C delivered later as /v1/scrub + /v1/pseudonymize; E/F later.
  - **NER accuracy measured 2026-10-20.** `testdata/golden/ner/` (92 cases: a 49-case development set the decoder was written against, a 25-case holdout and an 18-case final set) and the `integration`-tagged gate `ner_accuracy_test.go`, run by `make test-ner`. Two metrics, because the NER types share one operator (redact): typed P=0.8395 R=0.8718 (the golden package's type-sensitive metric) and span P=0.9012 R=0.9359 (type-agnostic — the one that says the PII is actually removed). **The generalisation figure is the FIRST evaluation of the final set on unseen text: typed P=0.5333 R=0.6154, span P=0.8667 R=1.0000.** The gate bounds sit below the development baseline, so a regression fails and an improvement is a deliberate bump.
  - **The decoder is ours, not prose's.** prose's own entity output is unusable as-is: its coalescer keeps the first token's label, so "Fernwood Logistics Inc" came back as a GPE and "Samuel Okoye" as an ORGANIZATION. `sidecar/internal/ner/prose.go` decodes prose's TOKEN labels instead, with shape rules (legal forms, institutional head nouns, business head nouns, place head nouns) and closed classes that are never entities (months, seasons, languages, greetings, courtesy titles, communication verbs).
  - **Performance, fixed 2026-10-20.** The sidecar was paying 340 ms of model loading per request: `prose.NewDocument` loads and decodes the 17 MB model on every call when it is given no model. The backend now builds it once with `sync.Once` and passes it via `prose.UsingModel`, which cut the per-request time from 352 ms to 19.6 ms (17x) with byte-identical token output. A race test covers the shared model. End to end, `Analyze` on a 239-byte sample went from 338 ms to 19.6 ms with the sidecar (64x the regex-only path, which is 305 us). Throughput is about 50 requests/second sequential and about 130/second with 16 concurrent callers.
  - **The corpora score it now, and the numbers are much worse (2026-10-20).** The person, organisation and place labels were added to the converter's type maps (`tools/corpusbench/convert/labels.go`), so `first_name`, `last_name`, `name`, `company_name`, `company`, `city`, `country`, `state`, `county`, `postcode` and `street_address` are gold instead of dropped. Scored on 2000 nemotron cases with the same scorer: regex-only exact P=0.8233 R=0.2654, with the sidecar exact P=0.4232 R=0.3903 (overlap R 0.2688 to 0.4704). The regex-only figure is low because the three new types are now in scope and it detects none of them — that is the baseline the NLP path must beat, and it beats it on recall at the cost of precision. Per type with the sidecar: LOCATION P=0.2457 R=0.3556, ORGANIZATION P=0.3077 R=0.1650, PERSON P=0.1307 R=0.0837. The built-in types are untouched (EMAIL P=0.9975 R=0.9975, SSN R=1.0000, URL R=0.9229). **The gap to the hand-written gate is measured, not assumed:** 74% of the corpus gold spans are covered by the detector but only 35% match exactly, and 166 of the 174 covered-but-larger spans are PERSON — the corpora annotate a GIVEN NAME ("Manal") while a NER model reports a FULL NAME ("Manal King"). **Conclusion: the sidecar is good on running prose and poor on form-filling records.** **Re-measured 2026-10-21** after the credit-card keyword gate, the card-over-phone overlap rule and the `api_key` / `http_cookie` → SECRET mapping: regex-only exact P=0.8357 R=0.2886 (overlap 0.8455 / 0.2919), with the sidecar exact P=0.4376 R=0.4135 (overlap 0.5222 / 0.4935); ORGANIZATION with the sidecar is now P=0.3095 R=0.1660, LOCATION and PERSON unchanged. The direction of every conclusion above holds; SPEC.md, "Corpus measurement", carries the current figures and explains why the level moved.
  - **Structured-document false positives, fixed 2026-10-20.** The three prose datasets had a blind spot: the corpora showed the decoder reporting JSON keys, HTTP headers and spreadsheet field labels as names ("Record Number", "Account Number", "SameSite=Lax", "Path=/auth", "Medical Record Number", "Customer ID"). Two rules fix the class: a span carrying a field-path character is a field, and a run whose LEADING words are two or more field names is a label (two, not one, because "Name", "City" and "Company" are both field labels and ordinary words). A fourth dataset file, `testdata/golden/ner/structured.json` (32 cases), guards them. NER detections on the nemotron corpus fell from 4.4 to 2.76 per text, on gretel from 6.6 to 5.90. Mutation-verified: disabling the field-label rule drops typed precision to 0.7907 and the gate fails.
  - **Residual, not fixed:** a brand name with no other signal ("The Orion laptop") is reported as a place. A brand gazetteer would fix it and would be wrong the moment the list is stale, so it is a recorded cost. The held-out set also shows the typed metric is far weaker than the span metric, because prose's head-vs-tail labels are unreliable: a person whose surname carries an ORGANIZATION label is typed as an organisation.
- [ ] NLP: ONNX backend — the Phase 0 backend landed (commit `aaf04b5`); the Phase 0 remainder and Phase 1 remain, see the Phase 0 remainder item below. The old prose plan's Phase E is superseded by the ONNX plan.
- [x] NLP: managed process mode (Phase F, optional, future) — see plan section 12. Shipped in commit `a41a8cc`; open follow-ups are at the "NLP managed mode" items near the end of this file (around L778-779).
- [ ] NLP: Phase 0 remainder (deferred 2026-10-31; continued 2026-11-01). **Done:** `golden.Case.Language`; `decode.go`; the `!onnx` stub plus untagged `ONNXConfig`, `ValidateONNXConfig` and `loadID2Label` (config.json `id2label`, contiguous from 0); chunking in `chunk.go` (400 tokens, 64-token overlap, 8000-token cap that fails the request, cross-chunk dedup and pairwise longest-first overlap removal); `hf_tokenizer.go` (daulet/tokenizers adapter: byte offsets, an out-of-range offset pair is an error rather than a clamp, `WordStart` from `##`/`▁`/`Ġ` markers and the whitespace-gap rule); `onnxrt/session.go` (onnxruntime_go dynamic session, input/output names read from the model, label dimension for the startup check, ctx checked around the non-cancellable `Run`); `onnx_backend.go` (id2label startup validation, label-count mismatch fails at startup, model/tokenizer SHA-256 logged, one inference per chunk, global offsets, no partial entities on error); ONNX module deps pinned in `sidecar/go.mod`; Makefile `fetch-libtokenizers`, `build-sidecar-onnx`, `test-sidecar-onnx`, `onnx-check`, and `NER_BACKEND`/three path variables on `corpus-nlp`. The `onnx-check` CI workflow is in `.github/workflows/onnx-check.yml`. `make onnx-check` is green (non-vacuous), and `make test-sidecar-onnx` passes including the tagged backend tests. **Still open:** `test-ner-onnx` (needs `testdata/golden/ner_multilingual/`, Phase 1); the parity fixture is a committed tiny WordPiece tokenizer, not the pinned multilingual `tokenizer.json` (Phase 1, which must also record its SHA-256); whether the real checkpoint needs `<s>`/`</s>` around each chunk is unverified (a Phase 1 blocker for `test-ner-onnx`) — the adapter adds no special tokens, so 400 means 400 content tokens; opset and quantisation mode are not logged (only the two SHA-256 values); ONNX Runtime exposes no opset API, so logging it needs a protobuf parse of the model or exporter-reported metadata; no adoptable checkpoint after a survey: Davlan/xlm-roberta-base-ner-hrl is AFL-3.0, Babelscape/wikineural-multilingual-ner is CC BY-NC-SA 4.0, flair/ner-multi covers en/de/nl/es only and names no licence, and the MIT multilingual-xlm-roberta re-uploads (apak, Tirendaz, maydogan) document neither their data nor their languages; the design's Apache-2.0/MIT bar and six-language set remain unmet. The XLM-R configs declare bos 0/eos 2 and max_position_embeddings 514; that is evidence, not proof, that `<s>`/`</s>` are required, so the adapter change is a Phase 1 task, not a result. **Prerequisite correction:** `github.com/daulet/tokenizers` links `libtokenizers.a` through cgo, so a module cache plus a C toolchain is NOT sufficient. `make fetch-libtokenizers` downloads the pinned v1.27.0 linux/x86_64 archive (SHA-256 verified) into the gitignored `.cache/tokenizers`; another platform must pin its own asset and checksum.
- [ ] NLP: ONNX success-path tests (Phase 1). `onnxrt` has only the failure-path `InitEnvironment` test. Add a success-path `InitEnvironment` test and a `Session.Run` test, both needing a shared `libonnxruntime` and a model directory (`model.onnx` + `config.json`).
- [ ] NLP: libtokenizers pins for other platforms. Pinned 2026-11-04: `make fetch-libtokenizers` now selects the asset and SHA-256 by host for linux/x86_64, linux/arm64, darwin/arm64 and darwin/x86_64. The hashes were measured from the v1.27.0 release assets (the linux/x86_64 value matched the existing pin). **Still open:** only linux/x86_64 has been built and run; the other three are untested, and darwin may need extra cgo link flags. The checksum step falls back to `shasum -a 256` where `sha256sum` is missing.
- [x] NLP: drop the redundant untagged prerequisite. **Done 2026-11-02:** `corpus-nlp` depends on `fetch-libtokenizers` for the onnx backend and has no prerequisite for the prose backend, since the recipe builds its own binary (the `build-sidecar` prerequisite compiled the sidecar twice; dropped 2026-11-03). Verified with `make -n corpus-nlp NER_BACKEND=onnx` (no untagged build) and `make -n corpus-nlp` (single build).
- [x] NLP: wire `ONNXBackend.Close()` into sidecar shutdown. **Done 2026-11-02:** `main.go` defers `closeBackend`, which calls `Close` on any backend that implements `io.Closer`; `server.Run` now waits for `Shutdown` to finish before returning. Tests: `sidecar/cmd/piiscrub-sidecar/main_test.go` and `sidecar/internal/server/server_test.go`. On a 5s `Shutdown` timeout, `Run` force-closes active connections and waits at most another 5s for handlers to return before `Close`; a handler stuck in a backend that ignores ctx is logged and abandoned (2026-11-03) so the process can exit.
- [x] Secret recognizers (API keys, tokens, passwords, private keys, entropy-based). Fixed: `recognizers/secret/` package with API key, bearer token, private key and password recognizers, registered in `piiscrub.go` with `EntitySecret` type and redact operator. Tests: `recognizers/secret/secret_test.go`.
- [x] Format-preserving encryption (FPE) operator — done, `operators/fpe.go` (commit 15d4db8).
- [ ] Image PII detection (facial recognition via go-face)
- [x] REST API sidecar — done, `/v1/scrub` and `/v1/pseudonymize` (commit 181aa07).
- [ ] **Multi-language support. Scoped 2026-10-27; phone formats for France, Sweden, Germany, the Netherlands, Italy and Spain done.**
  The built-in recognizers are format-based, so EMAIL, IP_ADDRESS, IBAN and SECRET
  are already language-agnostic and hold up across the seven gretel splits. The
  language-sensitive losses are, in measured priority order: PHONE national
  formats, the national-ID coverage behind the corpus's `ssn` label, the
  English-only word lists (opt-in context boost + the default card keyword gate),
  and the English-centric NLP sidecar model. Per-language PHONE recall before any
  change: France 0.099, Swedish 0.208, Dutch 0.394, German 0.469, Italian 0.698,
  Spanish 0.728, English 0.863.
  - [x] **France + Sweden phone formats.** Done 2026-10-27. The `+` branch takes
    up to three extra exactly-two-digit groups, plus `\b`-bounded national
    branches for France (`0X XX XX XX XX`, ARCEP layout) and Sweden
    (`0XX-XXX XX XX`). PHONE recall France 0.099 → 0.916, Sweden 0.208 → 0.755;
    false positives also fall (France 214 → 175, Sweden 246 → 233); seed gate
    unchanged (P=1.0000 R=0.9974, 200/200 negatives). Sources: Wikipedia
    "Telephone numbers in France" (citing ARCEP), E.164, libphonenumber. Numbers
    and rationale in SPEC.md "Non-English phone formats".
  - [x] **Germany + Netherlands phone formats.** Done 2026-10-27. Germany:
    geographic `0[2-9]<1-4>` + separated subscriber and mobile `01[5-7]…`.
    Netherlands: exact ten-digit geographic, `06` mobile, and the
    0800/088/085/084/087/0900/0906/0909 service prefixes. Added a `+NN(0)…`
    no-space branch and an eight-digit national floor that drops short
    fragments. PHONE recall German 0.551 → 0.735, Dutch 0.409 → 0.909; seed
    gate unchanged; English/Italian/Spanish precision improved from the floor.
    Numbers in SPEC.md "Germany and Netherlands phone formats".
  - [x] **Canonical E.164 without separators.** Done 2026-10-27.
    `\+[1-9]\d{6,14}` (7-15 digits, E.164's maximum) plus a token-boundary
    guard so arithmetic, C++ and email tags stay non-phones. Precision rose
    because the full span now wins; gretel combined PHONE 0.2931/0.8920 →
    0.3059/0.8951.
  - [x] **Sweden's rarer groupings.** Done 2026-10-27. Added a two-char area
    with a 2-3-3 subscriber (`08-11 123 456`) and a three-char area with a
    2-2-2 subscriber (`095-02 69 41`). Swedish PHONE recall 0.8302 → 0.8868;
    gates unchanged. France's bare `[+33] …` bracket form remains a deliberate
    phone gap.
  - [x] **Italy and Spain phone formats.** Done 2026-11-02. Italy:
    `0[26][\s.\-]\d{6,8}` (the only two-digit area codes) and
    `8\d{2}[\s.\-]\d{3}[\s.\-]\d{3}` (3-3-3 toll-free). Spain: the
    international branch accepts a 3-digit group with no separator after the
    country code (`+34927 245 069`), and the two-digit parenthesised form
    (`(55) 1234-5678`) is matched. A generic `[89]` 3-3-3 branch was rejected:
    `9xx-…` matched the prefix of a ten-digit number and cost Spanish true
    positives. Per-language PHONE: Italian P/R 0.3333/0.7937 → 0.3537/0.9206;
    Spanish 0.3857/0.8350 → 0.4043/0.9029. Combined gretel 0.3059/0.8951 →
    0.3053/0.8951; nemotron and the seed gate unchanged. Numbers in SPEC.md
    "Italy and Spain phone formats".
  - [x] **Mexico City 2-4-4 dashed numbers.** Done 2026-11-02. Added
    `\b\d{2}[\s.\-]\d{4}[\s.\-]\d{4}\b`, which matches `55-5555-5555`,
    `55-1234 5678` and `34-9876 5432`. The measured cost was one extra false
    positive on the Spanish split, not the broad-branch precision collapse the
    item anticipated, so no prefix or keyword guard was needed. Spanish PHONE
    0.4043/0.9029 → 0.4226/0.9806 (tp 93 → 101, fp 137 → 138, fn 10 → 2).
    Italian, combined gretel, nemotron and the seed gate are unchanged.
  - [x] **North American `011` prefix.** Done 2026-11-02. Added
    `\b011\d{7,11}`, which recovers `01184960536` in the Spanish split with no
    extra false positive. Spanish PHONE 0.4226/0.9806 → 0.4250/0.9903
    (tp 101 → 102, fp 138, fn 2 → 1). The remaining Spanish-split miss is the
    Swedish `08-957 37 46` shape, not covered by this branch.
  - [x] **Swedish `08` 2-3-2-2 and 2-3-3-2 forms.** Done 2026-11-02. Added
    `\b08[\s.\-]\d{3}[\s.\-]\d{2,3}[\s.\-]\d{2,3}`; it fixes `08-957 37 46`
    (Spanish split) and `08-123 456 78` (Swedish split). Spanish PHONE
    0.4250/0.9903 → 0.4274/1.0000 (tp 102 → 103, fp 138, fn 1 → 0). Swedish
    PHONE P/R 0.1715/0.8868 → 0.1702/0.9057 (tp 47 → 48, fp 227 → 234, fn
    6 → 5); the seven new Swedish false positives are all the unlabelled
    placeholder `08-123 45 67`, a valid shape the corpus does not label.
  - [x] **Swedish card and context word lists.** Done 2026-11-02. The opt-in
    card keyword gate and context booster covered de/nl/fr/it/es but not sv.
    Added `LanguageSV` (`kortnummer`, `kreditkort`, `betalkort`, ...) and the
    `sv` context set (`personnummer`, `kreditkort`, `e-post`, `samtal`, ...).
    English remains the default and the sets are opt-in, so the default
    numbers are unchanged. Measured with `PII_SCRUB_CARD_LANG=sv` on the
    Swedish split: CREDIT_CARD R 0.8333 unchanged, P 0.6250 → 0.5882 (tp 10
    both, fp 6 → 7). The one extra detection is
    `Kortnummer: 4571 1234 5678 1234` in case `gretel-pii-finance-000449`,
    which the corpus leaves unlabelled; the 12 gold cards are mostly
    Luhn-valid or English-labelled, so the Swedish phrases recover none. The
    context set is unmeasured: the boost is analyzer-level and `piiscrub`
    does not expose it.
  - [x] **IBAN "country grouping".** Investigated 2026-10-27; no detector
    change. The German/Italian IBAN recall gap (0.316/0.471) is entirely
    checksum-invalid synthetic labels: 6 of 19 German and 8 of 17 Italian gold
    spans pass mod-97, and every one of those is detected. Relaxing mod-97
    would destroy precision. See SPEC.md.
  - [x] **National-ID gap behind `ssn`.** Investigated 2026-10-27; no detector
    change. German/Italian `ssn` recall (0.111/0.091) is label-quality: the
    only detected value in each split is the US-shaped SSN; the rest are
    policy/account/customer-service numbers, EDIFACT references, `ZZ…T` test
    strings and synthetic 15-digit runs. The four Luhn-valid `YYMMDD-XXXX`
    values are Swedish personnummer, a separate Nordic-ID recognizer rather
    than an `ssn` fix. Bare nine-digit SSN matching was already rejected for
    precision. See SPEC.md.
  - [x] **Card-label and context word lists are language-selectable.** Done
    2026-10-27. `checksum.NewLuhnValidatorWithLanguages` /
    `piiscrub.WithCardLanguages` add de/nl/fr/it/es card-label phrases to the
    English default; measured CREDIT_CARD R Dutch 0.615→0.769, French
    0.429→0.643, Italian 0.133→0.333, German/Spanish unchanged.
    `analyzer.WithContextBoostLanguage` does the same for the opt-in boost
    (still off by default). The default pipeline, seed gate and pinned
    benchmarks are unchanged.
  - [ ] **Multilingual NLP model.** Tracked under the ONNX items above (Phase 1: blocked on a licence-compliant checkpoint). Network downloads work, so the earlier "downloads blocked" note no longer applies.
- [ ] Benchmark suite (OpenNER, Financial PII, Kaggle PII, CoNLL, ai4privacy, PII-Bench, SecretBench)

## Known gaps in the current MVP

- [x] **Phone: an unseparated country code is not parsed. Closed 2026-10-16: measured rare, no code change.** `91 555 123 4567`
      (no `+`) is matched as `555 123 4567` — the national number only, since the
      bare `1`-country-code handling is US-specific and there is no generic
      "country code + national number" branch. With a `+` (`+91 555 123 4567`)
      the whole string matches. Left UNASSERTED in `TestPhoneRecognizer`: it is
      not clearly a false positive, and pinning it would be deciding a policy
      nobody has. Revisit if a seed case makes it matter. **Measured 2026-10-16:**
      of 881 gold PHONE spans over both corpora (nemotron 557, gretel 324), only one
      prediction is a strict suffix with an unseparated country code (gretel, `001-`).
      The national number is redacted in every case, so the leak is the country code
      alone, which is not personal data. A generic country-code branch would add
      false positives (`Room 12 555-123-4567`) for a single case. Not changed.
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
- [x] **Four-part version strings are read as IPv4. Fixed 2026-10-18.** `neg-ip-version-four-part`
      ("Upgrade the agent to 3.10.2.7") is now suppressed: an IPv4 match whose
      preceding ~30 chars contain "upgrade", "version" or "agent" is treated as a
      version number, not an address. This is the heuristic (option b) that
      previously needed sign-off; applied after confirming no golden IP positive
      is immediately preceded by those cues. Golden gate ratcheted to exact
      tp=342 fp=1 fn=2 / overlap tp=343 fp=0 fn=1. All 177 negative cases now
      clean.
- [x] **Batch-1 notes in `seed.json` were stale or wrong.** Both rewritten to
      describe what the text tests; labels unchanged. `neg-luhn-failing-card`
      is clean (Luhn is wired in `NewScrubber`), confirmed by running the
      scrubber over every negative case: with the version-string fix, no negative
      case produces a detection. `pos-url-http` now describes a https URL with
      a path followed by prose, not a sentence period it never had.
- [x] **URL: an upper-case scheme. Fixed.** `HTTPS://DOCS.EXAMPLE.COM/START` is now matched: the scheme is matched case-insensitively (`[hH][tT][tT][pP][sS]?://`), per RFC 3986. The matched text keeps its original casing. Golden gate ratcheted to exact tp=340 fp=2 fn=4 / overlap tp=341 fp=1 fn=3. Tests: `TestURLRecognizer` and `TestURLRecognizerExactSpan` upper-case scheme cases.
- [x] **BITCOIN: all-caps bech32. Fixed.** The bech32 prefix is now matched case-insensitively (`[bB][cC]1`), and the validator already accepts all-uppercase (mixed case rejected). Gold case `pos-bitcoin-bech32-upper` now passes. Golden gate ratcheted to exact tp=342 fp=2 fn=2 / overlap tp=343 fp=1 fn=1.
- [x] **CREDIT_CARD: tab-separated groups. Fixed.** Tab is now a card separator (`isCardSeparator` and `LuhnValidator.Validate` strip `\t`), so `5555<TAB>5555<TAB>5555<TAB>4444` is reported. Golden gate ratcheted to exact tp=341 fp=2 fn=3 / overlap tp=342 fp=1 fn=2.
- [x] **Golden dataset: 81 to 491 cases toward the ~500 SPEC.md calls for. Done 2026-10-16.** SPEC is the source of truth. The expansion used synthetic data only, typed from the text, in six batches (gold values checked for uniqueness, Luhn for cards, mod-97 for IBANs). The batch notes live in `golden/accuracy_test.go`.
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
      **Status 2026-10-16:** 491 cases (314 positive, 177 negative). Exact P=0.9941 R=0.9855 (tp=339 fp=2 fn=5), overlap P=0.9971 R=0.9884 (tp=340 fp=1 fn=4), 176 of 177 negatives clean. Thresholds are the exact counts. The five exact misses are recorded, not relabelled: the card-window defect, the UK national phone (out of scope), and the three entries above. This is one author's hand labels in common forms, so the corpora remain the stronger test. **Licence review is DONE —
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
- [x] **Context boosting: a boost never lifts a span over a URL that survives the threshold; boosting stays off by default.** Before the fix, boosting was worse on nemotron: 7 URLs (0.99) lost their whole span to an inner IP or card boosted to 1.00 (`http://192.168.1.100:8080/...`). Fix: `applyContextBoost` skips any non-URL span that overlaps a URL surviving `minConfidence` (`overlapsSurvivingURL`, `analyzer/analyzer.go`). Regressions: `TestContextBoostDoesNotLiftSpanInsideURL`, `TestContextBoostDoesNotLiftSpanOverlappingURL`, `TestContextBoostIgnoresURLDroppedByThreshold`. Numbers after the fix, boost off → on, corpus schema: **nemotron exact P 0.8345 → 0.8345, R 0.8916 → 0.8916** (tp 2426, fp 481, fn 295 both); **gretel P 0.3677, R 0.8201, unchanged**; URL spans lost off→on: 0 on both corpora. **Re-measured 2026-10-19 on the current tree (the 0.8345 figure above predates the 477-fp run):** nemotron exact P 0.8357 (fp 477), R 0.8916 (tp 2426, fn 295) with boost off and on; the entity list is identical for every case in both corpora; gretel P 0.3677, R 0.8201, unchanged; URL spans lost 0. Seed set unchanged (`TestContextBoostIsNoWorseOnSeed`). Measured with a temporary test file, since removed. The URL-loss check is not a standing gate; the seed gate covers the general case.
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

- [x] **CREDIT_CARD recall: keyword-gated rule implemented (2026-10-21).** Re-verified on the converted corpora (scorer as `make corpus`), not taken from the earlier claim. nemotron: 254 gold CREDIT_CARD spans; 24 pass Luhn (9.4%, about 1 in 10.6, the rate a random check digit gives). All 230 misses fail Luhn, and all 254 golds have a valid network prefix (Visa 117, Mastercard 72, Amex 37, UnionPay 14, Discover 9, Diners 5). So the corpus is card-shaped data whose check digit is random, not random digits, and the check digit is the only thing separating it from a card. Every gold span equals the whole digit run (none has a CVV or year joined). Layout: 4-4-4-4 193, 4-6-5 33, 4-6-4 2, 4-4-4-3 1, unseparated 1. 219 of 230 have a whole-word card keyword within 40 bytes either side (card, credit, debit, visa, mastercard, amex, discover, jcb, diners, cvv, cvc); 11 have none. gretel: 57 gold spans, 27 Luhn-valid (all detected), 30 misses; 21 have a valid prefix and a card keyword (Amex 4-4-4-3 most common), 9 have no network prefix. nemotron's 21 FPs are all Luhn-valid runs labelled `account_number` (12) or `device_identifier` (9) in the corpus, and none has a card word within 40 bytes. gretel's 26 FPs: 19 with no corpus label, 7 labelled `bban`.
+- **Implemented 2026-10-21 (decision 1.c, hybrid):** the window-level label-style gate is in `recognizers/checksum/checksum.go` (`hasCardLabelKeyword`, `standardCardLayout`, `windowBoundedBySeparator`). A Luhn-failing window is reported at 0.85 only when a label-style keyword is within 40 bytes, the prefix is a network prefix, the layout is standard (no trailing CVV), and the window is bounded. Bare "card" in prose does not gate, so the seed negatives stay clean. Measured on 2000 cases: nemotron CREDIT_CARD P/R 0.6154/0.0945 to 0.9400/0.9252 (tp 24→235, fp unchanged at 15, fn 230→19); gretel 0.5745/0.4737 to 0.6875/0.7719 (tp 27→44, fp unchanged at 20, fn 30→13). The gate adds no corpus false positives at all — every remaining FP was already present under Luhn-only (nemotron `account_number`/`device_identifier`, gretel `bban`). Seed unchanged (P=1.0000; the gate thresholds did not move). Presidio: R unchanged at 0.8750 (the 2 FNs are prefix-1 gold cards 0002/0003, pre-existing); the cost is the 2 expected FPs (0016/0018), P 1.0000 to 0.8750. Phone-vs-card (B2) is unchanged: PHONE nemotron 0.5668/0.9982 and gretel 0.2857/0.8580 before and after.
  - **B2 closed 2026-10-21, so the totals above moved once more.** Card now outranks phone in overlap resolution (the item below), which lifts nemotron CREDIT_CARD from 0.9400/0.9252 to 0.9405/0.9331 and drops PHONE false positives from 425 to 423, because those two phone spans were the card tails this item describes. Gretel, Presidio, the seed and `make test-ner` did not move.
  - **Prototypes, measured then removed.** A Luhn-failing digit run with a valid network prefix, a 4-4-4-4 / 4-6-5 / 4-6-4 layout and a whole-word card keyword (card, credit, debit, visa, mastercard, amex, discover, jcb, diners, cvv, cvc) within 40 bytes, reported at 0.85 (the keyword-backed tier, as MYKAD keyed and SG passport). Both variants remove 0 existing detections. Nemotron headline exact R 0.8916 to 0.9710 (keyword before the number) or 0.9721 (either side). Results as CREDIT_CARD P / R:
    - **Keyword before the number:** nemotron 0.5333 / 0.0945 to 0.9195 / 0.9449 (tp 240, fp 21, fn 14); gretel 0.5094 / 0.4737 to 0.5942 / 0.7193 (tp 41, fp 28, fn 16); seed unchanged and `go test ./...` passes; Presidio 0.9412 / 1.0 to 0.8421 / 1.0 (fp 1 to 3: `my credit card number is 4012-8888-8888-1882` and `my credit card number is 36168002586008`, both gold no entity).
    - **Keyword on either side:** nemotron 0.9205 / 0.9567 (tp 243, fp 21, fn 11); gretel as above; seed gains 1 FP (`neg-card-luhn-two`, "A mistyped 4111111111111112 fails the checksum and is not a card"); Presidio as above; `go test` fails `TestScrubberRejectsLuhnFailingCard` (`piiscrub_test.go:128`, "Test fixture 4111 1111 1111 1112 is not a real card.", reported) and `TestGoldenDatasetAccuracy`.
    - The two new gretel FPs (same case, `gretel-pii-finance-001287`, two occurrences of one 13-digit value, masked `XXXXXXXXX5873`) sit after `Credit Suisse, London/USD/`, so the bank name `Credit` triggers the keyword and the number is not a card. A keyword list needs word-level exclusions or it will do this again.
    - Not measured: the rule with context boost on, where 0.85 plus up to 0.3 reaches 1.00 and would beat PHONE (0.98) and TH_ID (0.95).
  - **Why the whole-run prototypes were rejected.** SPEC.md says cards are "detected by Luhn alone", and the Presidio negatives 0016 and 0018 say no entity for a Luhn-invalid number after "credit card number is". Either whole-run variant changes that, and the either-side one also changes a seed negative and a Go test. The shipped window-level label-style rule is the narrower change: it keeps the seed negatives clean and costs only the two expected Presidio FPs.
  - **Decision taken 2026-10-21: 1.c hybrid, implemented.** The label-style compromise keeps seed precision at 1.00 and avoids the `Credit Suisse` gretel FP that the bare-word rule produced. (a) keep Luhn-only was not taken because the approved design recovers the corpus recall; (c) either-side was not taken because it changes a seed negative and a Go test. The prototype numbers below are kept as the rejected alternatives.
- [x] **Two nemotron PHONE false positives covered the tail of a gold CREDIT_CARD. Fixed 2026-10-21 (decision: card over phone on overlap).** `nemotron-pii-000709` and `nemotron-pii-001744` contain `Credit/Debit Card 3467 284599 8675` and `credit debit card 3014 754803 2190`: 14 digits in a 4-6-4 Diners layout, failing Luhn, both gold `CREDIT_CARD`. The card recognizer claimed each window at 0.85 under the label-style rule, but the phone recognizer (0.98) won the overlap and claimed `284599 8675`, so the scrubber printed `Credit/Debit Card 3467 ***-***-8675` — the first four digits of a card number stayed visible and the mask shown was the phone's. **Fix:** `analyzer.removeOverlapping` now consults `winsOverlap` before confidence, and `winsOverlap` lets CREDIT_CARD beat PHONE on overlap at every confidence. This is a precedence decision, not a confidence bump: a card window needs 13 or more digits behind a valid network prefix (`checksum.cardSpans`), so the colliding phone match is always a sub-run of digits the card already covers and dropping it loses no coverage, while the phone winning is what loses the card's leading group. A Luhn-valid card at 0.99 already won this pair, so only the gated 0.85 case changes; the rule makes an intent that was previously a coincidence of scores explicit. Measured by reverting just the precedence call: seed unchanged (exact P=1.0000 R=0.9974, 199/199 negatives); nemotron CREDIT_CARD P/R 0.9400/0.9252 → 0.9405/0.9331 (tp 235→237, fn 19→17, fp unchanged at 15) and PHONE 0.5668/0.9982 → 0.5679/0.9982 (fp 425→423 — those two were these cases — fn unchanged at 1); nemotron headline exact P 0.8351 → 0.8357, R 0.2883 → 0.2886; gretel, Presidio and `make test-ner` all unchanged. Tests: `analyzer/overlap_test.go::TestRealOverlapsGatedCardBeatsPhone`, `piiscrub_test.go::TestScrubberRedactsGatedCardNotItsPhoneTail` (both red before the fix). One existing test adapted: `context_boost_test.go::TestContextBoostRunsBeforeOverlapResolution` used card-vs-phone as its baseline pair, which confidence can no longer decide, so its colliding entity is now an SSN at 0.75 (the card's 0.6 + 0.2 boost = 0.8 beats it, clearing the tie it previously sat on) and the comment records why PHONE must not be used there.
- [x] **Whole-run Luhn pass skips the network-prefix and layout checks (2026-10-19, fixed).** `LuhnValidator.cardSpans` returns the whole candidate when it passes Luhn, before `validCardPrefix` and `cardGrouping` run, so a 13-19 digit run with prefix 0, 1, 7, 8 or 9, or an odd layout, is reported. Evidence: 26 of the 47 corpus CREDIT_CARD FPs (nemotron 8, gretel 18) have no network prefix, and Presidio `test_credit_card_recognizer-0001` (`1748503543012`, gold no entity) is the one Presidio FP. The `validCardPrefix` comment says `1748503543012` is rejected, which is not what happens. The 2026-10-19 Fixed entry above also says that number is pinned by `TestRawLuhnKeepsCardAboveThaiThreshold`; that test uses `1000000000009`, and no test pins `1748503543012`. A fix changes Thai-ID typing (the `pos-th-id-luhn-valid` cases start with 1), so it needs a decision.
- **Fixed 2026-10-19:** `cardSpans` now requires `validCardPrefix` on the whole-run path too, so a run whose first digit is 0/1/7/8/9 is no longer reported as a card. `1748503543012` is no longer reported; Thai IDs (first digit 1) are now detected by `THIDRecognizer` (0.95) instead of the card path. Tests updated: `TestLuhnRecognizeReportsThaiValidSpanAsCard`, `TestRawLuhnKeepsCardAboveThaiThreshold`, `TestScrubberTypesLuhnValidThaiIDAsTHID`.
- [ ] **Gretel CREDIT_CARD spans narrower than the digit run (corpus-side, 2026-10-19).** 15 of 57 gretel gold spans end before a digit or separator that the run continues into. No recognizer change.
- [x] **Corpus-side label quality depresses measured precision.** gretel EMAIL
      reads P=0.3281, but spot-checking shows the corpus labels the local part
      (`user_name 90..96 = 'lowe73'`) rather than the full address, markdown
      `[addr](mailto:addr)` renders each address twice while only one
      occurrence is labelled, and 106 of 647 email-shaped occurrences in
      email-bearing records carry no email label at all. A corpus miss looks
      exactly like our false positive. Before any threshold is recorded from
      these numbers, spot-check a sample of FPs by hand (spec §8 risk 4).
      **Done 2026-10-27:** hand spot-check of the 383 exact EMAIL false
      positives, reproduced with a scratch scorer that matched the benchmark
      exactly (tp=187 fp=383 fn=45). All 383 are corpus-side: 283 (73.9%) the
      unlabelled second copy of a markdown `[addr](mailto:addr)` rendering; 76
      (19.8%) email-shaped spans with no label at all (CSV/JSON fields, prose,
      staff tables; 0 non-ASCII concatenation artifacts); 24 (6.3%) the bare
      address matched inside a decorated gold span (`[addr]`, `_addr_`,
      `mailto:addr`). 0 detector-side false positives. FN=45: 31 decorated
      golds whose inner address the recognizer found, 14 placeholder labels
      that are not addresses (`[Claims Email]`, `[insert customer support
      email]`, `[email protected]`). No recognizer change; no EMAIL precision
      target recorded from this corpus. SPEC.md's gretel EMAIL note carries the
      decomposition; the schema's unlabelled-span upper bound does not capture
      these because they carry no label, not an out-of-map label.
- [x] **gretel PHONE precision (0.2766) is now explained (2026-10-18).** Hand spot-check of 40 FP spans (scratch scorer over the converted gretel set) shows the same root cause as nemotron: the bare 10-digit branch (`\b(?:\+?1...)?\d{3}[\s.\-]?\d{3}[\s.\-]?\d{4}\b`) fires on 10-digit account/transaction/ID numbers in EDI, JSON, invoice and bank contexts (e.g. `1234567890` in `UNB+UNOC:3+1234567890:12`, `0000012345` in X12 control numbers, `NPI: 1234567890`). The corpus labels none of them as PHONE. **Correction 2026-10-19:** the earlier wording said they carry `account_number`/`customer_id` labels. A full scan shows 679 of the 693 gretel FPs carry no label at all, 3 are `SSN` and 1 is `bank_routing_number`. The breakdown is in the 2026-10-19 block of the nemotron entry below. No recognizer change. Threshold not recorded.

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

- [x] FPE library evaluation (capitalone/fpe vs alternatives) — Done 2026-10-24: evaluated capitalone/fpe and decided to keep the custom stdlib-only FF1 implementation in operators/fpe.go (radix 2-62 vs 2-36, Encrypt+Decrypt vs Encrypt only, 8-10 rounds vs fixed 10). Documented in SPEC.md "FPE library evaluation" section.
- [x] go-face model bundling strategy — Done 2026-10-26: download on first use. Model files NOT bundled with binary; on first face-detection call, each missing model is downloaded from dlib.net, decompressed from .bz2, and cached in the OS user cache dir (~/.cache/dataprotect/face-models/ on Linux). Implemented in sidecar/internal/facedetect/ with Downloader (thread-safe, retry-on-failure, dedup per model), FaceDetector interface + StubDetector (returns ErrDlibUnavailable until libdlib-dev is installed). Documented in SPEC.md "go-face model bundling strategy" section.
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
SPEC.md § "Corpus measurement (2026-10-19)"; reproduce with `make corpus`.

- [x] **Anonymizer passed through any type without an operator (fail-open for custom maps). Decided 2026-10-09: option 1 (fail closed).** `Anonymize` now returns an error, before any splice, for an entity whose type has no operator and no default. Breaking for partial maps; the error names the type and span. Options 2 and 3 were done earlier, and the default `NewScrubber` is unchanged. Tests: `TestAnonymizeUnconfiguredTypeIsAnError`, `TestAnonymizePerTypeRoutingOracle`. Contract in SPEC.md §anonymizer. The history below is kept for the record. Probed 2026-10-09: a card-only operator map, the manual-wiring example in README and the usage doc, leaves `Citizen ID 1000000000009` unredacted. Before the Thai decision (d) the same number was a card and was masked. The default `NewScrubber` is safe: it sets an operator for every type the default recognizers emit. The same gap exists for SG_NRIC, MY_IC, MY_PASSPORT, TH_ID and IBAN in any custom map that omits them. Options: (1) make `Anonymize` fail closed, returning an error for an entity with no operator and no default. This is a breaking change for partial maps, but it fails loudly. (2) Keep the default and change the manual example to `anonymizer.New(anonymizer.WithDefaultOperator(operators.NewRedactOperator()))`, so callers opt in to redact-all. (3) Keep both and add a test that every type the default recognizers can emit has an operator in `NewScrubber`. Recommendation: (2) plus (3) now, and (1) only if you want the breaking change. **Done:** (2) the README and usage manual examples now use `WithDefaultOperator(operators.NewRedactOperator())`. (3) `recognizers.AllEntityTypes` lists every type, and `TestScrubberEveryEntityTypeHasOperator` checks each has an operator in `NewScrubber`. A new constant must be added to that list by hand, so the test cannot catch a type left out of the list.
- [x] **Passports: Malaysia and Singapore done, Thailand uncovered (decided 2026-10-15).** Other Asian countries are not started. Thailand stays uncovered by decision, not oversight; the decision is the record.
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
- [ ] **National ID coverage gaps.** (a) Singapore M-prefix: NOT supported. Corrected 2026-10-19: this item was stale. It was written in 75ea9ca, which added an unverified M table that was later reverted; the code, tests and docs say M is not supported (probed 2026-10-19: `M4332674T` is not reported). Open follow-up is the `SG NRIC: M-prefix FIN NOT supported` entry above. (b) Malaysia MyKad place-of-birth codes are not validated: the code list is unconfirmed, and the MyKad check-style digit is not validated either. (c) **Decided 2026-10-10: (a), keep the bare 13-digit Thai form as is, with no keyword gate.** Its mod-11 check passes about 1 in 10 random 13-digit runs. Corpus check: nemotron has 0 bare 13-digit runs and gretel has 38, none mod-11 valid. That is too few to measure a real FP rate, so the risk is accepted, not measured. (d) Batch 3 added synthetic golden cases for them (4 SG, 2 MY, 2 TH positives; 5 negatives). The corpora don't label these types, so no recognizer is measured on real text.
- [x] **National-ID gaps from the earlier national-ID commit (9594065), fail-open.** A background security review flagged the national-ID recognizers as fail-open. Its finding bodies never arrived, so the items below are what my probes through `Scrubber.Anonymize` found, not the review's wording. Fixed: a label-glued SG NRIC (`NRICS1234567D`) is found, only when the NRIC or FIN label comes before it. The earlier fix dropped the leading `\b` for every NRIC, so a 0.99 NRIC inside a shape-only 0.95 bitcoin token (`1Ab3CdEf5Gh7Jk9Mn2Pq4RsS1234567D`) won the overlap and left 23 characters visible. The review flagged this as an overlap displacement. The bare-token case is now back to one full BITCOIN span, and a differential over 4072 seed and corpus texts shows no lost detections. Fixed: bare 12-digit MyKad is found only after an identity-card keyword (Purview's list; date check kept). **Decided 2026-10-15 (user, on advisor's recommendation): glued national IDs are documented scope.** Probed through `Scrubber.Analyze` on 2026-10-15: `ID3100600445635` (Thai, label glued on) and `ORD3100600445635` are not redacted; `xS1234567D` (SG, letter before, no label) is not redacted. `Thai ID:3100600445635`, `ID 3100600445635`, `NRIC: S1234567D` and `NRICS1234567D` are redacted. Reason: relaxing `\b` would also redact order and reference numbers, so the precision cost is larger than the recall gain. Recorded in SPEC.md, MVP scope, national ID bullet.
- [x] **Valid Thai citizen ID that passes Luhn was reported as CREDIT_CARD. Decided 2026-10-09: TH_ID wins (decision d).** Before the decision, `1000000000009` (and `1-0000-00000-00-9`) passed both the Thai mod-11 check and Luhn, so the card recognizer (0.99) beat TH_ID (0.95). The masked output was `****-****-****-9***`, which shows the check digit. Fuzzing generated check-valid IDs in unlabelled contexts found 6 of 30 typed CREDIT_CARD, in every non-glued context and both forms. A background review flagged it as a false negative in the redaction control. Implemented (current design): the recognizers still report CREDIT_CARD. `Scrubber.Analyze` retypes a resolved card span to TH_ID when the span is a Thai citizen ID with the same word boundaries the Thai pattern uses. The retype runs after overlap resolution, so the span keeps its 0.99 through threshold and overlap. Two earlier designs were rejected. (1) The deferral in the card recognizer left a Thai-valid number unredacted when the Thai recognizer was not wired or the threshold was above 0.95. (2) Typing it TH_ID inside the Luhn recognizer changed the type for callers with card-only maps, so a Thai ID they used to mask went out unredacted. **Trade-off, recorded:** decision (d) applies to the `Scrubber` API only. Raw analyzers and the `structured/` CSV path still report a masked card for a Luhn-valid Thai ID. That is the old behavior, not a leak. Other spans stay CREDIT_CARD, including glued candidates, so nothing goes unredacted. Golden cases `pos-th-id-luhn-valid` and `pos-th-id-luhn-valid-hyphen` pin the result. Rejected alternative: raising TH_ID above 0.99. That would also beat URL, email and SSN at 0.99 on overlap, so a URL containing a Thai-shaped number would lose its path to the mask.
- [x] **Phone pattern matches ID-number digit runs (nemotron, and gretel). Decided 2026-10-19: no recognizer change. The PHONE precision is a label-scope artefact, and the redaction it reports is protective.** Of the 419
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
      **2026-10-19: cause confirmed, precision classed as a scope artefact.** This closes the open "cause not confirmed" line in SPEC.md (which said "not yet confirmed by hand"). The 2026-10-18 gretel entry above had the same cause but misstated the labels; it is corrected there. Evidence, from a throwaway probe over both converted corpora with the scorer `scripts/benchmark.sh` uses (not committed):
      - **Cause (code).** Every nemotron FP on an ID-type span is a 10-digit match of the bare US branch (branch 3). Branches 1 and 2 need `(` or `+`. Confirmed.
      - **Label scan.** Nemotron FP 419 = 408 on spans the corpus labels with an unmapped type (`biometric_identifier` 117, `customer_id` 83, `account_number` 79, `medical_record_number` 76, `health_plan_beneficiary_number` 34, `http_cookie` 6, `unique_id` 7, `certificate_license_number` 4, `coordinate` 1, `street_address` 1) + 2 overlapping a gold CREDIT_CARD span + 5 on ID keywords with no label (`Policy Number:`, `Serial Number:`) + 4 on phone contexts the corpus left unlabelled (`Contact Information:` x2, likely phones; `Customer Service at`; `Tax Assessor's Office at`). The 2 others overlap a gold CREDIT_CARD span (text `Credit/Debit Card dddd <S>`); not investigated further. The 408 is the same count SPEC's set-aside bucket reports. The older 406 figure used a narrower list.
      - **Hand check.** 12 of the 408 drawn at random, all in ID contexts (`BIO-` identifier, `Customer ID:`, `account number`, `medical record number`).
      - **No other entity overlaps them.** Across all 1112 PHONE FPs of both corpora, no other entity in the scrubber's output overlaps the span. That is after overlap resolution, so a suppressed lower-confidence match would not show; the claim is limited to the output. A narrowing rule would therefore unredact labelled PII (for example 117 `biometric_identifier` spans). That is the leak the library rules out.
      - **Gretel 693 FPs.** 679 carry no label at all. Shape (exclusive): bare 10-digit 601, dashed 10-digit 44, other 10-digit 1, 11-digit 27 (8 bare, 3 space- or plus-grouped, 16 dashed), 12-digit 4, 8-digit 2. Nemotron 419: bare 10-digit 377, space-grouped 10-digit 9, dashed 10-digit 31, 11-digit 2. Keyword-and-separator windows (40 characters) put them in: EDIFACT, SWIFT or XML or CSV structured values (461), ID-keyword fields the corpus did not label (71), placeholders or fixed patterns (83), real-looking phone contexts the corpus missed (40, read by eye: `Phone:`, `call us at`, `Fax:`, `Dial-in number:`, `Contact Information:`), no context (24). The other 14: 10 boundary misses against a gold PHONE (see the new entry below), 3 SSN and 1 `bank_routing_number`. These classes are heuristic. The 40 phone contexts are the one group that is a clear corpus gap; the true precision is higher than measured.
      - **Candidate rules, all measured, none adopted.**
        1. Reject bare (unseparated) sequential or identical runs, such as `1234567890`, `9876543210`, `5555555555`. Gold loss: 0 bare. Gold grouped sequential values (`123-456-7890`, 65 TPs in gretel) must stay, so only the bare form could be rejected. FPs removed: 527 bare sequential (515 gretel, 12 nemotron; 480 are exactly `1234567890`, 474 of them gretel). This is a shape count that cuts across the buckets: of the 515 gretel ones, 396 fall in (c), 69 in (d), 48 in (a'), 2 elsewhere, so bucket (d) shows only 83 in total. **Rejected:** the same shape covers numbers that could be real: `2345678901` (area code 234), `6789012345` (area code 678), `0123456789` (Malaysian mobile shape), `9876543210` (Indian mobile shape). Passing the gate needs a denylist of strings, which is a workaround. See the decision below.
        2. Require separators on the bare branch. Gold loss: 5 TPs (gretel `0712345678` x2, `0257305484`, `7644921150`; Presidio `13812345678`). Also leaks `Phone:5551234567` and `tel:+15551234567`, which both report today. **Rejected** on recall and leak.
        3. Reject a bare run next to an EDIFACT or SWIFT delimiter (`+ : ' | *`). Gold loss: 0 bare, 2 grouped (nemotron 1, gretel 1). FPs removed: 239. **Rejected:** `tel:+15551234567` reports `15551234567` after a `+`, and `Phone:5551234567` after a `:`. A real phone would leak.
        4. Reject a bare run after an uppercase code and hyphen (`BIO-`, `WA-`, `CA-`). Gold loss: 0. FPs removed: 145 (143 nemotron, 2 gretel). **Rejected:** the 117 `BIO-` and 26 state-prefixed beneficiary IDs would be unredacted, and `TEL-5551234567` (a plausible phone) would leak.
        5. Reject `555` exchange numbers (`555-555-xxxx`). Gold loss: 10 gretel and 12 Presidio TPs. **Rejected.**
      - **Decision for the user (not implemented).** Whether to stop redacting the bare placeholder family `1234567890` (480 FPs, 474 in gretel). Recommendation: keep redacting. A denylist is a workaround, and the same rule would leak `0123456789` and `9876543210`. Choosing otherwise needs a labelled policy change and a golden case for each side.
      - **Result.** Nemotron PHONE stays P=0.5703 R=0.9982. Gretel PHONE stays P=0.2766 R=0.8179. Presidio phone stays exact tp 19 fp 5 fn 9. Seed stays P 1.0000 R 0.9973. No code, test, bound or doc number changed.
      - **Scope note for the gretel "explained" claim.** The bucket is mostly unlabelled digits in ID and EDI contexts, which is a corpus gap. Redacting them is the protective choice. It is not a measured detector error, and it is not a measured detector success either.
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
        figure is mostly a scope artefact. SPEC says so as of 2026-10-19.
      Option (c) moves every corpus number, so it is not applied. Until one of
      (a)-(c) is chosen, record no SSN precision target.
- [x] **Phone: gretel boundary misses (10 exact FPs against a gold PHONE). Done 2026-10-26.** Decision: keep truncation as-is — the boundary misses are corpus-side (bracketed links, UK trunk-zero) and the remaining detector-side misses (extension, IDD prefix, paren-country prefix) would each need a labelled policy change before a recognizer edit. No code change; the protective choice is to keep redacting them. Seed, corpus, and doc numbers unchanged.
- **Partial fix 2026-10-19:** extension (`x2744`), IDD prefix (`001-`) and paren-country prefix (`(+1)`) are now matched, including the extension in the span. The two UK trunk-zero cases remain out of scope. Corpus-side bracket cases unchanged.
- [x] **Phone: decision needed on the bare sequential placeholder `1234567890` (gretel). Done 2026-10-26.** Decision: keep redacting — 480 FPs are exactly `1234567890` (474 gretel, 6 nemotron). Removing them needs a denylist (a workaround) and the same rule would leak `0123456789` and `9876543210`. No code change; the protective choice is to keep redacting them. Seed, corpus, and doc numbers unchanged.
- [ ] **BITCOIN coverage.** Neither corpus labels it, so it stays unmeasured 2026-11-04: seed negatives added for a bad P2SH checksum, a bad taproot checksum and mixed-case bech32 (seed 553 to 556; `minCases` 556; recall bounds ratcheted to 1.0 at tp=385 fn=0, 203/203 negatives clean).
      on real text. The seed set now has five BITCOIN cases (batch 8). Closing
      this needs a third labelled corpus, which is data, not code.
- [x] **Per-language scoring of gretel. Measured 2026-10-14; answer: recall is lower in every other language.** The per-language `*_test` parquet files are already local, so the seven were converted with `convertParquet` (all rows, no limit), scored with `WithInSchema` and the default scrubber. Exact P / R, and the share of negatives clean, were: English (2891 cases) 0.3652 / 0.7709, 1850 of 2140; French 0.2039 / 0.4205, 295 of 338; German 0.2926 / 0.5154, 299 of 353; Italian 0.3680 / 0.6346, 298 of 346; Spanish 0.4475 / 0.6864, 285 of 324; Swedish 0.1912 / 0.4906, 276 of 345; Dutch 0.2974 / 0.5141, 299 of 339. Recall falls by 8 to 35 points against English in every other language, so the recognizers do worse on non-English text. The cause per type is not diagnosed here. Precision is low everywhere, which the corpus-label caveats above already explain. This was a one-off scratch run, not committed; the converter still reads `combined_test` only (`tools/corpusbench/convert/gretel.go`). Making it a repeatable target would mean a language parameter on the converter, which is a CLI change and is not done.
- [x] **Corpus results as a CI gate.** Satisfied by design: the 498-case hand-labelled seed set is the CI gate (`golden/accuracy_test.go`). The corpora and their converted output are gitignored, so CI cannot score them; the seed set is the committed gate. The corpora remain available locally via `make corpus` for developer measurement.
- [x] **Golden seed accuracy at 498 cases (2026-10-19).** Independently recomputed over `testdata/golden/seed.json` through the live recognizer: 316 positive, 182 negative, exact P=1.0000 R=0.9971 (tp=347 fp=0 fn=1), overlap P=1.0000 R=0.9971, 182/182 negatives clean. All doc copies (README.md, piiscrub-usage.md, SPEC.md, spec-v1.md, SOURCES.md, docs/index.md, AGENTS.md, development-conventions.md) updated to match; `minCases` ratcheted 491→518 in `golden/accuracy_test.go`.
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
- [x] **Re-syncing is now automated.** `scripts/sync-docs.py`, wired as
      `make docs-sync`, copies the body of `SPEC.md` into
      `docs/decisions/spec-v1.md` and the body of `AGENTS.md` into
      `docs/concepts/development-conventions.md`, keeping each file's
      frontmatter. **Corrected 2026-10-20:** the earlier note here claimed
      `docs/concepts/piiscrub-usage.md` was a copy of `README.md` with four
      link rewrites. It is not: it is a condensed usage document with its own
      body, and the script leaves it alone. The four link rewrites are
      therefore gone with the copy step that needed them, and the usage doc is
      maintained by hand. The script is idempotent and refuses to write a file
      that has no frontmatter.
- [x] **The usage doc is still hand-maintained.** Done 2026-11-02.
      `scripts/docs_check.py` now reads `recognizers.AllEntityTypes` and fails
      when the README entity table or its "N types" claim, or the usage doc's
      seed coverage table, drifts; it also requires the usage doc to name
      PERSON, ORGANIZATION and LOCATION. The first run caught a real drift:
      README said 17 types and omitted `PH_TIN`/`IN_GSTIN`, and the usage doc
      said 5 national recognizers. Both docs now list 19 types and 7 national
      recognizers. The recognizer counts were enforced on 2026-11-03; see the
      next item.
- [x] **Usage-doc prose is still hand-written. Done 2026-11-03.** The four
      recognizer counts in the Features list are now source-derived: regex
      (8), national (7) and secret (1) count the packages' `func New*`
      constructors, and checksum (2) counts the validators `newScrubber`
      constructs. The bases are disjoint, so IBAN and bitcoin may sit in both
      a package and an entity-type list without making a count ambiguous. The
      same numbers are checked in the usage doc's frontmatter description and
      the bundle index that copies it. Verified by changing "8 regex" to "9":
      `docs_check: usage regex count 9 != source 8`. The operator count stays
      "4" by kind (replace, mask, hash, redact); tail-mask and FPE are a
      variant and an optional operator, not separate kinds.
- [ ] Optional LLM second pass for weak candidates (first case: keyword-free bare 12-digit MyKad). Design in `DESIGN-llm-second-pass.md` (2026-11-05); no code yet. Open: model choice and licence (Apache-2.0/MIT), and review of the design decisions.
- [ ] NLP managed mode: deferred parts of `recognizers/nlp/managed.go` (2026-11-05). (1) ~~Windows: shutdown sends SIGTERM via `Process.Signal`, which Windows does not support, so `Close` returns an error and the child keeps running.~~ **Done 2026-11-05, untested on Windows:** the child starts with `CREATE_NEW_PROCESS_GROUP` and `terminate` sends `CTRL_BREAK_EVENT` to it (`recognizers/nlp/managed_windows.go`), so the sidecar drains as on `os.Interrupt`; `stop` kills it after the timeout. With no shared console the child is killed at once with no drain, and processes it started are not killed. Only `GOOS=windows go build` has been run; no Windows runtime test. (2) ~~Port race: the port is probed and released before the child binds it; if another process takes it, the child exits and `StartManaged` fails fast with no retry.~~ **Done 2026-11-05:** a child that exits with a bind error on stderr (`address already in use`, or the Windows wording) is retried on a fresh port, up to `managedStartAttempts` (3) attempts; any other early exit still fails fast. Covered by `TestStartManagedRetriesWhenPortIsTaken` and `TestStartManagedGivesUpAfterMaxAttempts`. (3) ~~Child stderr goes to this process's stderr and is not captured into errors; only the exit status is reported.~~ **Done 2026-11-05:** the last 4 KiB of child stderr is kept and appended to startup errors (`withStderr` in `recognizers/nlp/managed.go`); it still also goes to this process's stderr. (4) ~~A child that dies after start is not detected; `Recognize` surfaces it as connection errors.~~ **Done 2026-11-05:** once the child has exited, `Recognize` returns `sidecar process exited` with the stderr tail (`stateErr` in `recognizers/nlp/managed.go`). Residual: under `WithBestEffort` the error is logged and dropped, as for every sidecar error. ~~(5) No env var or CLI flag opts in; callers use `StartManaged` directly, and `PII_SCRUB_NLP_URL` still selects the external-sidecar path. Add the opt-in when a caller needs it.~~ **Done 2026-11-05:** `PII_SCRUB_NLP_BINARY` (sidecar path) opts in via `nlp.ManagedFromEnv(ctx)`, which errors if `PII_SCRUB_NLP_URL` is also set; `nlp.FromEnv()` still reads only the URL.
- [ ] NLP managed mode, code-review findings still open (2026-11-05). Fixed the same day: the child now runs in its own process group (Unix) and `/healthz` must return the sidecar's JSON body, and a child that exits right after the probe fails startup. **Items:** (1) ~~nothing ties the child to `ctx` after startup, so a parent that never calls Close leaks it~~ **Done 2026-11-05:** cancelling the `StartManaged` ctx stops the child as `Close` would; (2) ~~`Recognize` calls in flight during `Close` fail with a raw dial error~~ **Done 2026-11-05:** requests during or after `Close` return a `recognizer is closed` error; (3) ~~on Windows `terminate` sends `os.Interrupt`, which Windows does not support, so Close returns an error there.~~ **Done 2026-11-05, untested on Windows:** see the Windows item (1) in the entry above. ~~The MyKad filler group also accepts repeats such as `no no`; it is looser than needed but still needs the keyword.~~ **Done 2026-11-05:** the two filler words must differ, so `mykad no no <digits>` is rejected (`recognizers/national/national_test.go`). **Residual, open:** the keywords `ic no`, `i/c no`, `kp no` and `k/p no` already contain `no`, so `ic no no <digits>` still matches as keyword plus one filler. Dropping those keywords would also reject `ic no number is <digits>` (three fillers), so this needs a decision. **Residual, open (docs and callers):** `AGENTS.md`'s `recognizers/nlp/` line does not yet say that the `StartManaged` ctx ends the child or that startup retries on a taken port. `ManagedFromEnv` in `recognizers/nlp/nlp.go` passes its ctx through, so its doc comment needs the same ctx-lifetime note. The ctx path and the closed-state path are not yet tested on Windows.
- [ ] NLP managed mode follow-ups (2026-11-05). (1) ~~`nlp.FromEnv()` silently ignores `PII_SCRUB_NLP_BINARY`.~~ **Done 2026-11-06:** `FromEnv` now returns `(*NLPRecognizer, error)` and errors when the binary variable is set; no in-repo callers. (2) `scripts/benchmark.sh`, `integration_ner_test.go`, `ner_accuracy_test.go` and the Makefile `NER_URL` targets read only `PII_SCRUB_NLP_URL`. (3) Bundle docs still describe URL-only: `docs/concepts/piiscrub-usage.md:252`, `docs/decisions/spec-v1.md:802`, `docs/concepts/development-conventions.md:44`; add an OKF `docs/log.md` entry. (4) Windows stop falls back to an immediate kill (logged as a warning) when no shared console exists: no drain, grandchildren survive; runtime untested on Windows. (5) MyKad: `ic no no <12 digits>` still matches because the keywords `ic no`, `i/c no`, `kp no`, `k/p no` contain "no". Decision 2026-11-06: no more regex special-casing; handle such ambiguity in the LLM second pass (see the LLM second-pass item above).
