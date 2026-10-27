---
type: Decision
title: piiscrub v1 Spec — Scope, Architecture & Non-Goals
description: 'Architecture decision record for piiscrub v1: why a dependency-free Go PII library instead of Python Presidio, the explicit non-goals, MVP scope versus v2+ phases, core interfaces (Recognizer, Operator, Analyzer, Anonymizer), the actual on-disk package layout, per-recognizer accuracy targets alongside measured results from the 549-case golden seed AND from a 2000-record sample of both fetched corpora (which fall well short of the >99% target, with every large gap explained), operator semantics, the CSV batch contract, pinned dependencies, and the testing/benchmark strategy.'
tags:
  - pii
  - go
  - architecture
  - spec
  - recognizers
  - anonymization
  - privacy
  - v1
timestamp: 2026-10-07T04:56:44Z
---

# piiscrub — Go PII Detection & Anonymization Library

## Spec v1

**Location:** `~/www/dataprotect`
**Language:** Go 1.24+
**License:** MIT
**Target RAM:** 100-200 MB (vs Presidio's 4 GB)

---

## Rationale & Non-Goals

**Why build a Go version when Presidio exists?**
Presidio is Python-only, requires 4 GB RAM with spaCy models loaded, and runs as a sidecar service. For Go projects that need inline PII scrubbing (before LLM calls, in data pipelines, in log sanitization), a native Go library is 100x lighter, compiles into any binary, and has zero operational overhead.

**Non-goals (v1):**
- No multi-language support (English only)
- No cloud API — everything runs in-process
- No formal privacy guarantees (k-anonymity, differential privacy) — that's ARX's domain
- No GUI — library + optional REST API only

---

## MVP Scope (v1)

Core interfaces + regex recognizers + checksum recognizers + 4 operators.

### In scope:
- Core interfaces: `Recognizer`, `Operator`, `Analyzer`, `Anonymizer`
- **Regex recognizers** — email, phone, credit card, SSN, IP, URL, IBAN, bitcoin
- **Checksum recognizers** — Luhn (credit cards), IBAN mod-97
- **National ID recognizers** — Singapore NRIC/FIN (check letter, prefixes S/T/F/G;
  the M prefix launched 1 January 2022 is NOT supported: its check-letter table has
  not been confirmed against an official ICA/PRADO spec — see TODO.md). Malaysia
  MyKad (hyphenated form, real date of birth; no checksum exists). Thailand citizen
  ID (mod-11 check digit). Malaysia passports (one letter A/H/K and eight digits,
  matched only next to a passport keyword). Singapore passports (E or K, seven
  digits, one letter, same keyword gate; shape not confirmed by PRADO).
  Thailand passports stay uncovered by decision (2026-10-15): the only sourced shape
  is the expired 2012 booklet's, which overlaps Japan's. National IDs need a word
  boundary before them, so an ID glued to a letter is not redacted (for example
  `ID3100600445635` or `xS1234567D`). The exception is an SG NRIC glued directly to
  its label (`NRICS1234567D`), which is found. This is documented scope (decided
  2026-10-15): a looser boundary would also redact order and reference numbers.
- **Operators** — replace, mask, hash, redact
- **Context-aware confidence boosting** — opt-in via `analyzer.WithContextBoost()`, applied
  before the confidence filter and before overlap resolution. A span that overlaps a URL
  which survives the confidence threshold is never boosted, so the URL keeps its whole
  span. Re-measured 2026-10-19 with the corpus schema: boost on and off give identical exact P
  and R on both corpora (nemotron P 0.8357 R 0.8916; gretel P 0.3677 R 0.8201), the entity
  list is identical for every one of the 2000 cases in each corpus, and no URL span is lost.
  Those are the 2026-10-19 corpus figures, kept as dated measurements; the corpus schema
  changed on 2026-10-20 and "Corpus measurement" below carries the current ones. What is
  claimed here is the identity of boost-on and boost-off, not the level. Still off by default.
- **Structured data** — CSV batch processing
- **Golden dataset** — 553-case hand-labelled seed set for CI, plus an
  on-demand corpus measurement over the fetched corpora (`make corpus`)

### Later (v2+):
- Format-preserving encryption (FPE)
- Image PII detection (facial recognition)
- REST API sidecar
- Multi-language support
---

## Project Structure (MVP)

```
dataprotect/
├── go.mod
├── go.sum
├── AGENTS.md
├── CLAUDE.md
├── README.md
├── TODO.md
├── .gitignore
├── .env.example
├── piiscrub.go              # Main package entry point
├── analyzer/
│   ├── analyzer.go          # Analyzer engine, overlap resolution
│   ├── analyzer_test.go
│   ├── overlap_test.go      # card-vs-phone, IBAN collisions
│   ├── context_boost_test.go
│   └── property_test.go     # FuzzAnalyzeInvariants + invariants
├── recognizers/
│   ├── recognizers.go       # Entity, EntityType, Recognizer interface
│   ├── regex/
│   │   ├── regex.go         # all eight regex recognizers
│   │   └── regex_test.go
│   └── checksum/
│       ├── checksum.go      # Luhn + mod-97 IBAN validators
│       └── checksum_test.go
├── anonymizer/
│   ├── anonymizer.go
│   ├── anonymizer_test.go
│   └── property_test.go     # FuzzAnonymizeInvariants + splice oracle
├── operators/
│   ├── operators.go         # replace, mask, hash, redact
│   ├── operators_test.go
│   └── property_test.go     # FuzzOperatorReplace + invariants
├── structured/
│   ├── batch.go             # CSV batch processing
│   └── batch_test.go
├── internal/context/
│   └── context.go           # confidence boosting (+ context_test.go)
├── recognizers/nlp/
│   ├── nlp.go               # HTTP client recognizer for the NLP sidecar
│   └── nlp_test.go
├── sidecar/                 # separate Go module (prose NER service)
│   ├── go.mod
│   ├── cmd/piiscrub-sidecar/
│   └── internal/{ner,server}/
├── golden/
│   ├── golden.go            # scoring runner for hand-labelled cases
│   ├── golden_test.go
│   └── accuracy_test.go     # CI gate over testdata/golden
├── testdata/
│   └── golden/
│       └── seed.json        # 50 hand-labelled seed cases
└── scripts/
    └── benchmark.sh         # scores testdata/golden, prints the report
```

This tree lists files that exist. The previous version described a
per-recognizer file layout (`email.go`, `phone.go`, …) plus an
`analyzer/entity.go` and `analyzer/recognizer.go` split that were never
created — all eight regex recognizers live in `recognizers/regex/regex.go`, and
the entity types are in `recognizers/recognizers.go`.

---

## Core Interfaces

```go
package recognizers

type EntityType string

const (
    EntityEmail       EntityType = "EMAIL"
    EntityPhone       EntityType = "PHONE"
    EntityCreditCard  EntityType = "CREDIT_CARD"
    EntitySSN         EntityType = "SSN"
    EntityIP          EntityType = "IP_ADDRESS"
    EntityURL         EntityType = "URL"
    EntityIBAN        EntityType = "IBAN"
    EntityBitcoin     EntityType = "BITCOIN"
)

type Entity struct {
    Type       EntityType
    Start      int
    End        int
    Text       string
    Confidence float64
}

type Recognizer interface {
    Name() string
    Recognize(ctx context.Context, text string) ([]Entity, error)
}
```

```go
package operators

// Operator transforms a single detected entity's text.
//
// Operators work one value at a time and the caller owns splicing: a
// replacement changes the text's length, which invalidates the byte offsets of
// every entity not yet rewritten. Anonymizer therefore rewrites right-to-left so
// each span is still valid when reached, which also makes the result independent
// of map iteration order.
type Operator interface {
    Name() string
    Replace(ctx context.Context, value string) (string, error)
}
```

```go
package analyzer

type Analyzer struct {
    recognizers []recognizers.Recognizer
    minConfidence float64
}

func New(opts ...Option) *Analyzer
func (a *Analyzer) AddRecognizer(r recognizers.Recognizer)
func (a *Analyzer) Analyze(ctx context.Context, text string) ([]recognizers.Entity, error)
```

```go
package anonymizer

type Anonymizer struct {
    operators map[recognizers.EntityType]operators.Operator
    defaultOp operators.Operator
}

func New(opts ...Option) *Anonymizer
func (a *Anonymizer) SetOperator(entityType recognizers.EntityType, op operators.Operator)
func (a *Anonymizer) Anonymize(ctx context.Context, text string, entities []recognizers.Entity) (string, error)
func (a *Anonymizer) Pseudonymize(text string, entities []recognizers.Entity) (string, map[string]string, error)
```

`Pseudonymize` replaces every entity with a numbered placeholder named for its type,
such as `[EMAIL_1]` or `[CREDIT_CARD_2]`, and returns the placeholder-to-original
map. Numbers count per type from 1, in text order. The same type and the same
canonical value share one placeholder (decided 2026-10-16). The canonical value is
the digits of a card, phone or SSN, the upper-case form of an IBAN without separators,
the lower-case form of an email, and the `netip` form of an IP. A URL and other types
compare exactly. Different values, and the same value under two types, get different
placeholders. Numbering is local to each call. It ignores the configured operators,
so every entity is replaced. The returned map holds raw PII: the caller must protect
it as it would the input. On any error the string is empty and the map is nil, and an
error never includes an original value.

Restoring a placeholder gives the first spelling seen for it, so the round trip is
exact up to spelling: spacing, separators, parentheses, letter case, and IPv6 notation
may differ from the input, inside a value only. `golden.PseudonymRoundTrip(original, out,
map)` is the check the seed and the corpora use. It requires text outside the placeholders
to match exactly, except that a value's stretch may take up separators beside it. The check
cannot tell those separators from the prose, so a dropped full stop after an email passes.
Each stretch is compared with its value modulo spelling.

`Scrubber.PseudonymizeAll(ctx, texts)` pseudonymizes several texts with one mapping,
so a value that recurs in a later text reuses its placeholder and numbering runs
through the texts in order. It is the path for inputs split into texts, such as table
cells. The round trip and error rules apply to each text.

A text that already holds a placeholder-shaped token for a known entity type (for
example a literal `[EMAIL_1]`) is an error, because the map could not be reversed
reliably. The check applies to a text with no entities too, because a shared mapping
(`PseudonymizeAll`) would rewrite such a token on restore. The check uses `recognizers.AllEntityTypes`. An unknown bracketed token is
ordinary text. `Scrubber.Pseudonymize(ctx, text)` runs detection first. `make corpus-anonymize`
checks the round trip over the corpora (up to spelling).

Scope: `structured/` (CSV) pseudonymizes through `PseudonymizeCSV` (see Structured Data), which is `PseudonymizeAll` over the file's cells.

Caller-join rule: each call numbers its placeholders from 1. Two texts pseudonymized in separate calls can both hold `[EMAIL_1]` for different values, and a token can be split or recreated where two texts are joined. A caller that joins them and then restores gets wrong values. Restore each text before joining, or join before pseudonymizing.

`Anonymize` requires non-overlapping, in-bounds entities whose start and end fall
on UTF-8 rune boundaries. An overlapping, out-of-bounds, or mid-rune span is
rejected with an error, and on any error the returned string is empty — never a
partially spliced document. The `Analyzer` already
resolves overlaps before returning, so the normal pipeline never triggers this;
the check guards direct callers.

An entity whose type has neither its own operator (`SetOperator`) nor a default
(`WithDefaultOperator`) is also an error, checked before any splice. It is never
returned unredacted. This is a breaking change for partial operator maps: a
caller that wants redact-all for unlisted types passes `WithDefaultOperator`.

---

## Recognizers (MVP)

| Recognizer | Entity Type | Method | Target | Measured (553-case seed) |
|------------|-------------|--------|--------|--------------------------|
| Email | `EMAIL` | Regex (RFC 5322 simplified) | >99% P&R | P=1.0000 R=1.0000 |
| Phone | `PHONE` | Regex (US/international/E.164) | >98% P, >97% R | P=1.0000 R=0.9828 (UK national format missed; out of scope) |
| Credit Card | `CREDIT_CARD` | Luhn checksum | >99% P&R | P=0.9811 R=0.9811 (tab-separated groups fixed; the `pos-card-then-year` window defect remains) |
| SSN | `SSN` | Regex (XXX-XX-XXXX) | >99% P&R | P=1.0000 R=1.0000 (shape-only, by decision) |
| IP Address | `IP_ADDRESS` | Regex (IPv4); IPv6 candidates validated by `netip` | >99% P&R | P=1.0000 R=1.0000 (version string suppressed) |
| URL | `URL` | Regex (HTTP/HTTPS) | >99% P&R | P=1.0000 R=1.0000 |
| IBAN | `IBAN` | Regex + mod-97 | >99% P&R | P=1.0000 R=1.0000 |
| Bitcoin | `BITCOIN` | Regex (P2PKH, P2SH, Bech32) + Base58Check / bech32 checksum | >99% P&R | P=1.0000 R=1.0000 |
| SG NRIC/FIN | `SG_NRIC` | Regex + check letter (S, T, F, G) | >99% P&R | P=1.0000 R=1.0000 (M prefix NOT supported — check table unconfirmed against ICA; see TODO.md) |
| Malaysia MyKad | `MY_IC` | Regex (hyphenated) + date check | >99% P&R | P=1.0000 R=1.0000 (no checksum exists) |
| Thailand citizen ID | `TH_ID` | Regex + mod-11 check digit | >99% P&R | P=1.0000 R=1.0000 (Scrubber types Luhn-valid Thai IDs as TH_ID, decision d; raw analyzers keep CREDIT_CARD) |
| Malaysia passport | `MY_PASSPORT` | Regex (A/H/K + 8 digits) next to a passport keyword | >99% P&R | P=1.0000 R=1.0000 (keyword-backed; shape from Wikipedia and Purview, not PRADO) |
| Singapore passport | `SG_PASSPORT` | Regex (E or K + 7 digits + letter) next to a passport keyword | >99% P&R | P=1.0000 R=1.0000 (keyword-backed; shape from Purview and the 2006 ICA BioPass description; K prefix and digit count not confirmed by PRADO) |
| Secret | `SECRET` | Regex (API key, token, password assignment, PEM private key header), and provider token formats with no label (`bareShape`: AWS access key id `AKIA` or `ASIA`, GitHub `ghp_`, `gho_`, `ghu_`, `ghs_`, `ghr_` and `github_pat_`, Slack `xoxb-` and `xoxp-`, Stripe `sk_live_` and `rk_live_`, Google `AIza`, npm `npm_`, SendGrid `SG.`, and a bare JWT). A bare token is reported whole, its body must pass the uniform check, and it is dropped when it overlaps a labelled span. The formats and their sources are in SOURCES.md. A label may be followed by a closing quote (JSON `"api_key": ...`) and by spaces before its separator. The value is kept unless it is uniform: every character the same, or one unit of up to four characters repeated at least twice. There is no character-class rule, so a lower-case passphrase or an all-digit token is reported once its shape's length floor is met. Known tradeoff, accepted because the scrubber prefers over-redaction: a plain word of 8 or more characters after a label, such as `Password: Required`, is reported | >99% P&R | exact P=1.0000 R=1.0000 (tp=34 fp=0 fn=0); overlap P=1.0000 R=1.0000 on 34 positives, 17 negatives clean. The 9 bare-token positives (one per format) and 3 bare-token negatives match exactly. The span covers the value only, with the key name, separator, quotes and `Bearer ` outside it. A JWT or JWE with an `eyJ` header is captured whole at any header length, including a `dir` JWE with an empty second segment. A dotted token is captured to its end at any segment count. A single trailing dotted word is dropped only when whitespace follows it, so `Bearer <value>.Next step` keeps the value, while `token=<value>.abc` at the end of a text or before a quote is captured whole. A password drops one sentence-final `.` that is followed by whitespace or the end of text (`password: Rainbow@2025.` reports `Rainbow@2025`); `!`, `?` and `,` stay in the value. A password also drops one trailing `)`, `]` or `}` that is followed by whitespace, the end of text, or a closing quote when its value holds no matching opener (`(pass=Hunter2xyz)` reports `Hunter2xyz`; `Pa(ss)w0rd1` is kept whole). A short `pass` or `pwd` label needs a non-letter before it, so `bypass=kittenmitten` and `Route compass: northeastregion` are not passwords, while `db_pass=`, `db-pwd=` and `"pass": ` are; `password`, `passwd` and `secret` have no boundary, so `dbpassword=` and `mysecret=` match. Accepted cost: a letter directly before `pass` or `pwd` is no longer matched (`dbpass=x`, `rootpwd=x`) (TODO.md) |

**"Target" is a goal for a ~500-case dataset; "Measured" is what
`scripts/benchmark.sh` reports today over the 553-case seed set.** Most measured
cells are 1.0. The cells below 1.0 are the defects listed in TODO.md. These cells
are evidence only for this hand-labelled set, not for the detector in general.
The Luhn and mod-97 validators are wired into the default pipeline.
The measured column covers all twelve types; the five national-ID and passport rows rest on synthetic cases only. BITCOIN is shape-then-checksum: a
shape match is reported only when its Base58Check (`1`/`3`) or bech32/bech32m
(`bc1`) checksum verifies. A shape match that fails its checksum is not an address
and is not reported. Taproot (`bc1p`, bech32m) is
matched by the same pattern and verified by the bech32m checksum.

An earlier version of this table presented ">99% P&R" as though it described the
recognizers. It described an aspiration for a dataset that did not exist. Only
the measured column is evidence.

Note the aggregate is not an average of this table, and the per-type cells are
coarse: `BITCOIN` rests on 13 gold spans, so one flipped case moves its recall by
about 8 points. Aggregate exact precision/recall over the whole seed set is
P=1.0000 R=0.9974 (tp=384 fp=0 fn=1), and overlap matching, which tolerates
mis-sized spans, gives the same P=1.0000 R=0.9974 (tp=384 fp=0 fn=1). Re-measured 2026-10-22,
when the seed grew to 553 cases with a Gemini auth key positive and its short-body negative. The one miss under
both is `pos-phone-trunk-zero`. Until 2026-10-19 the secret spans included the key name, so
exact matching missed 10 secrets (TODO.md). Before the parenthesised-phone
fix the exact aggregate was P=0.9231 R=0.9231 while overlap stayed at 1.0000; the
gap between the two granularities is exactly what localised the mis-sized span,
which is why both are reported.

---

## Corpus measurement (2026-10-21)

The seed set above is hand-written and small. `make corpus` scores the same
detector against the two fetched corpora, which is the measurement the ">99%"
target was always meant to describe. Both numbers below come from the **same
scorer** as the seed gate — only the dataset and the schema filter differ.

Reproduce with `make corpus` (default `LIMIT=2000` records per corpus, in file
order, recorded in each `schema.json`).

| | nemotron (2000 of 100000) | gretel (2000 of 5596) |
|---|---|---|
| **Exact** | P=0.8357 R=0.2886 F1=0.4290 (tp=2655 fp=522 fn=6546) | P=0.3816 R=0.0957 F1=0.1530 (tp=696 fp=1128 fn=6576) |
| **Overlap** | P=0.8455 R=0.2919 F1=0.4340 (tp=2686 fp=491 fn=6515) | P=0.3986 R=0.1000 F1=0.1599 (tp=727 fp=1097 fn=6545) |
| negatives clean | 52 of 71 | 231 of 280 |
| out-of-schema (not scored) | IBAN=1 | BITCOIN=21, MY_PASSPORT=1, URL=817 |

**Why recall fell from the 0.8916 / 0.8201 recorded on 2026-10-19.** It is a
change in what counts as gold, not a detector regression, and it happened twice
since that date. The converter's type maps gained `PERSON`, `ORGANIZATION` and
`LOCATION` on 2026-10-20 because the NLP sidecar made them detectable; this
table is the regex-only pipeline, which implements none of them, so those spans
are now false negatives — nemotron carries 3275 PERSON, 1988 LOCATION and 1018
ORGANIZATION, gretel 2577 / 1489 / 2374. And `api_key` / `http_cookie` became
`SECRET` on 2026-10-20, which moved SECRET out of the set-aside bucket and into
the scored schema. Both corpora moved the same way. The pre-change figures
survive in the re-measurement history further down and in the NLP section's
baseline row below.

Per type (exact):

| Type | nemotron P / R | gretel P / R |
|------|----------------|--------------|
| `EMAIL` | 0.9975 / 0.9975 | 0.3281 / 0.8060 |
| `PHONE` | 0.5679 / 0.9982 | 0.2857 / 0.8580 |
| `CREDIT_CARD` | 0.9405 / 0.9331 | 0.6875 / 0.7719 |
| `SSN` | 0.8780 / 1.0000 | 0.9787 / 0.8846 |
| `IP_ADDRESS` | 0.9828 / 0.9771 | 0.8400 / 0.9767 |
| `URL` | 0.9900 / 0.9229 | *no label in corpus* |
| `IBAN` | *no label in corpus* | 0.9556 / 0.9773 |
| `BITCOIN` | *no label in corpus* | *no label in corpus* |
| `SECRET` | 0.2540 / 0.0804 | 0.5600 / 0.3784 |
| `PERSON` | 0.0000 / 0.0000 | 0.0000 / 0.0000 |
| `ORGANIZATION` | 0.0000 / 0.0000 | 0.0000 / 0.0000 |
| `LOCATION` | 0.0000 / 0.0000 | 0.0000 / 0.0000 |

The last three rows are zero by construction on this pipeline and are listed so
the schema is visible rather than hidden. With the NLP sidecar they become
nonzero but stay weak; see the NLP section.

**Card outranks phone on overlap (decided 2026-10-21).** Overlap resolution
compares confidence, except that `CREDIT_CARD` beats `PHONE` at every score
(`winsOverlap` in `analyzer/analyzer.go`). A card window needs 13 or more digits
behind a valid network prefix, so the colliding phone match is always a sub-run
of digits the card already covers, and dropping the phone loses no coverage —
while the phone winning loses the card's leading group. This matters only for a
label-gated card: it scores 0.85 against the phone's 0.98, so before the rule
`Credit/Debit Card 3467 284599 8675` was masked as
`Credit/Debit Card 3467 ***-***-8675`, leaving four digits of a card number
visible under the phone's mask. A Luhn-valid card at 0.99 already won. Measured
by reverting only that call: nemotron `CREDIT_CARD` 0.9400 / 0.9252 to
0.9405 / 0.9331 (tp 235 to 237, fn 19 to 17, fp unchanged at 15) and `PHONE`
0.5668 / 0.9982 to 0.5679 / 0.9982 (fp 425 to 423 — those two were these cases);
seed, gretel, Presidio and `make test-ner` unchanged.

**Unlabelled spans, reported separately.** A detection that lands on a span the
corpus labels with a name outside our type map is not counted as wrong in the
headline. It is set aside into a bucket, and the report shows the precision with
it removed, labelled as an upper bound. The headline stays as it was, so the
bucket can't quietly move a number. Measured on the 2000-record samples:

- nemotron, exact: headline P=0.8357; 470 detections set aside (PHONE 407, SSN 25,
  SECRET 23, CREDIT_CARD 15); upper bound P=0.9808.
- gretel, exact: headline P=0.3816; 13 detections set aside (SECRET on `password`
  7, CREDIT_CARD on `bban` 4, IP_ADDRESS 1 on `credit_card_security_code`,
  PHONE 1 on `bank_routing_number`); upper bound P=0.3843.

An upper bound is not accuracy. The nemotron PHONE bucket is ID-field values
(`biometric_identifier`, `customer_id`, `account_number`, and others) that the
bare 10-digit phone branch matches. The cause is confirmed in code and by a label
scan, and 12 of the spans were hand-checked, all in ID contexts. They are PII
under the corpus's own labels and the scrubber redacts them today, so the PHONE
bound is not quoted as a precision figure. See TODO.md (2026-10-19).

**These numbers are far below the ">99%" target, and every large gap has been
explained rather than assumed away:**

- **SSN (gretel P=0.0929 → 0.9787; nemotron P=0.3495 → 0.8780)** — every
  separator was *optional*, so any bare nine-digit run matched: `CAS No.:
  123456789`, `Account Number: 123456789`, `Passport Number: 322996077`. Fixed:
  both separators are now required in prose, and a bare nine-digit value is
  accepted only when it is the whole input. Recall is unchanged (gretel 0.8846,
  nemotron 1.0000). Residual, hand-checked: all 25 nemotron false positives sit at spans the
  source labels with unmapped ID types (23 `tax_id`), and gretel's 1 is
  labelled by nothing. Whether unmapped-label spans should count as false
  positives is a scope decision — see TODO.md.
- **PHONE on nemotron (P=0.5679)** — 423 detections count as false positives in
  the headline, and a further 407 are set aside into the bucket above because
  they sit on spans the source labels as an ID-type field: `biometric_identifier`
  117, `account_number` 85, `customer_id` 83, `medical_record_number` 76,
  `health_plan_beneficiary_number` 34, `unique_id` 7, `certificate_license_number`
  4, `coordinate` 1. The bare 10-digit phone branch matches
  them (confirmed in code, by a full label scan and by a 12-span hand check). No
  other entity in the scrubber's output overlaps these spans, and they are PII
  under the corpus's labels, so narrowing the branch would stop redacting them. Five narrowing rules
  were measured and none was adopted (TODO.md, 2026-10-19). The precision figure
  is a label-scope artefact, not a measured detector error.
- **PHONE on gretel (P=0.2857)** — 695 false positives, with 1 more set aside on
  `bank_routing_number`. Almost all sit on digit runs the corpus leaves
  unlabelled: EDIFACT, SWIFT, XML and CSV values, placeholders such as
  `1234567890`, and ID fields. About 40 are real-looking phone numbers the corpus
  missed (`Phone:`, `call us at`, `Fax:`), so the true precision is higher than
  measured. 10 are boundary misses against a gold PHONE, and 4 sit on other labelled
  types. This is corpus-side and needs no recognizer change (TODO.md, 2026-10-19).
- **CREDIT_CARD on nemotron (R=0.0945 → 0.9331, fixed 2026-10-21)** — the 230
  false negatives all failed the Luhn check, and exactly the 24 Luhn-valid ones
  were the true positives. Of the 0.9331, 0.9252 is the keyword gate and the last
  0.0079 is the card-over-phone overlap rule. Re-verified 2026-10-19: all 254 gold card spans carry
  a valid network prefix and every span is the whole digit run, but only 24 of
  254 (9.4%) pass Luhn, the rate a random check digit gives. So the corpus cards
  are real-looking numbers with random check digits, not random digits. The
  label-style keyword gate (approved 2026-10-20, implemented 2026-10-21)
  recovers them: a Luhn-failing window is reported at 0.85 when a label-style
  keyword ("card number", "credit card", "Visa:") is within 40 bytes, the
  prefix is a network prefix, the layout is standard and the window is bounded.
  Measured: nemotron CREDIT_CARD P/R 0.6154/0.0945 → 0.9405/0.9331 (tp 24→237,
  fp unchanged at 15); gretel 0.5745/0.4737 → 0.6875/0.7719 (tp 27→44, fp
  unchanged at 20). The gate adds no corpus false positives at all. Bare "card"
  in prose does not gate, so the seed negatives stay clean. Cost: Presidio
  0016/0018 become false positives (P 1.0 → 0.875); the two Presidio misses are
  prefix-1 gold cards and predate this change. Of the nemotron total, 0.9252
  recall is the gate alone and the last 0.0079 is the card-over-phone overlap
  rule added the same day (see "Corpus measurement").
- **gretel EMAIL (P=0.3281)** — corpus-side, not detector-side. gretel labels
  the local part as `user_name` in some records (`lowe73@email.com` carries
  only `user_name 90..96 = 'lowe73'`), markdown `[addr](mailto:addr)` renders
  each address twice while the corpus labels one, and in records that carry any
  email label 106 of 647 email-shaped occurrences were labelled `email` at all.
  This is exactly the corpus-quality risk the spec anticipated: a corpus miss
  appears as our false positive. **Decomposed 2026-10-27:** all 383 exact false
  positives are corpus-side. 283 (73.9%) are the unlabelled second copy of a
  markdown `[addr](mailto:addr)` rendering; 76 (19.8%) are email-shaped spans
  the corpus never labels (CSV/JSON fields, prose, staff tables; none is a
  non-ASCII concatenation artifact); 24 (6.3%) are the bare address matched
  inside a decorated gold span that includes markdown brackets, emphasis
  underscores or the `mailto:` prefix. None is a detector false positive, so
  this number must not be read as detector precision. The 45 exact false
  negatives tell the same story: 31 are decorated golds whose inner address the
  recognizer found, and 14 are placeholder labels that are not addresses at all
  (`[Claims Email]`, `[insert customer support email]`, `[email protected]`). The
  schema's unlabelled-span upper bound does not capture any of these, because
  they carry no label at all rather than an out-of-map label.
- **nemotron URL (P=0.3053 → 0.9900, R=0.2846 → 0.9229)** — all 701 detections
  overlapped a gold URL, but only 214 matched it exactly. Most of the boundary
  disagreements were a trailing `.` (406) or `,` (64), which the pattern kept.
  The URL recognizer now trims that punctuation (see TODO.md). Overlap numbers
  were already matched and are unchanged.

**Coverage the corpora cannot give:** BITCOIN is labelled by neither corpus, so
it remains unmeasured; URL is measurable only on nemotron and IBAN only on
gretel, because each corpus labels only one of them.

A converted dataset declares its own scope in `schema.json`. `scripts/benchmark.sh`
applies it, so detections of a type a corpus never labels are reported as
out-of-schema instead of inflating false positives.

---

## Presidio test cases (2026-10-15)

Presidio (microsoft/presidio, MIT) ships a test suite with per-type PII cases. Its in-scope modules are converted by `scripts/presidio_extract.py` and scored with `make presidio`. Pin: `2523c7b74a469270c5c78bb253f140eafca21e31`. Converted data lives in `testdata/external/presidio/` (gitignored). The CI gate still runs only on `testdata/golden/`. The numbers below are a measurement, not a gate, and no recognizer changed for them.

**Scope.** The ten modules whose `entities` fixture maps to a type this library implements: EMAIL_ADDRESS, IP_ADDRESS, URL, IBAN_CODE, CRYPTO, CREDIT_CARD, PHONE_NUMBER, US_SSN, TH_TNIN, SG_NRIC_FIN. CRYPTO is bitcoin only: its pattern is `bc1` and `1`/`3` base58 addresses (`presidio_analyzer/predefined_recognizers/generic/crypto_recognizer.py`). Out of scope and not converted, of the 140 test modules: 76 country recognizer modules for countries this library does not cover, 24 other recognizer modules (including the NLP and model-based entities such as PERSON and LOCATION), and 30 infrastructure modules (engine, registry, configuration, chunkers). Also out of scope: presidio-research's evaluation framework and generator data (the generator's identities are CC BY-SA 3.0), the context-word sentence file (no spans, so no gold), and the e2e tests, which are API smoke tests with no per-type case tables.

**Method.** Only `parametrize` lists are parsed, with `ast` (never imported or run). Presidio's character offsets are converted to UTF-8 bytes, and each value is taken from the text. Rows that cannot be scored are set aside with a reason in `skipped.json`: duplicate texts (the same text twice — a leniency variant, or a bare token repeated in a second context — is one case, not two, so the repeat is set aside), and the 2 empty-text negatives (`golden.Load` rejects empty text). A count-only row (Presidio asserts how many matches, not where) converts only when the row asserts exactly one match on a single whitespace-free token: then the whole text is the only span the gold could be, which is transcription, not invention. Anything else count-only stays set aside. Each module is scored as its own dataset, so `in_schema` is that module's type.

| Module | Cases | Negatives | Exact P / R (tp, fp, fn) | Overlap P / R |
|--------|-------|-----------|--------------------------|---------------|
| email | 8 | 2 | 1.0000 / 1.0000 (7, 0, 0) | 1.0000 / 1.0000 |
| ip | 114 | 29 | 0.8095 / 0.7556 (68, 16, 22) | 1.0000 / 0.9333 (84, 0, 6) |
| url | 16 | 3 | 0.7500 / 0.6429 (9, 3, 5) | 0.9167 / 0.7857 |
| iban | 329 | 172 | 1.0000 / 1.0000 (158, 0, 0) | 1.0000 / 1.0000 |
| crypto (BITCOIN) | 9 | 3 | 1.0000 / 1.0000 (7, 0, 0) | 1.0000 / 1.0000 |
| credit_card | 18 | 5 | 0.8667 / 0.8667 (13, 2, 2) | 0.8667 / 0.8667 (13, 2, 2) |
| phone | 21 | 0 | 0.7500 / 0.7241 (21, 7, 8) | 0.9286 / 0.8966 |
| us_ssn | 28 | 12 | 0.5769 / 0.8824 (15, 11, 2) | 0.5769 / 0.8824 |
| th_tnin (TH_ID) | 39 | 26 | 0.8125 / 1.0000 (13, 3, 0) | 0.8125 / 1.0000 |
| sg_fin (SG_NRIC) | 10 | 1 | 1.0000 / 0.5000 (5, 0, 5) | 1.0000 / 0.5000 |

Totals: 592 cases from 601 rows. 9 rows set aside: 2 exact-duplicate texts (the `5555555555554444` card repeated in two lists, and the bare `2400:c401::5054:ff:fe1b:b031` repeated in a second context — each counted once), 4 leniency-variant duplicates of texts the default recognizer already covers, 1 count-only row with no determinable span, and 2 empty-text negatives. The overlap column counts a span that touches a gold span as a match, so it is an upper bound.

**Card and IBAN fail-open, fixed 2026-10-15.** A card followed by a space and digits (`4111111111111111 123`), two cards separated by a space, and an IBAN followed by an uppercase word or digit group were not redacted. The cause was candidate grouping: each run of digits joined by space, hyphen or dot was checked as one number, and a run that failed the check was dropped. The fix keeps the whole-run check. When that fails, every card-layout window of digit groups that passes Luhn is found, the search is bounded to 19 digits per start, and overlapping windows are merged so every card digit is covered (`checksum.cardSpans`). A run inside a valid IBAN is not searched. An IBAN keeps the whole candidate, an unspaced first block, or the longest space-cut prefix that has the printed shape (`checksum.canonicalIBANShape`) and passes mod-97. Guards: a single group must be bounded by a space, hyphen or dot, and a run inside a valid IBAN is not searched. Seed batch 9 (8 positives, 2 negatives) pins the cases, and the gate is 91 cases. Exact bounds were 61/62 and 1.0 and are now 60/62 and 60/61 (see the review response below). The user decided on 2026-10-15 to keep the merge over restoring exact spans, which would reintroduce the chance-window bypass.

Measured before and after, on the same inputs:

| Set | Before | After | Note |
|-----|--------|-------|------|
| seed | 81 cases, exact P 0.9811 (tp 52, fp 1), R 1.0000 | 91 cases, exact P 0.9677 (tp 60, fp 2), R 0.9836 (tp 60, fn 1); overlap R 1.0000 | gate: `minCases` 91; exact bounds 60/62 and 60/61, re-measured (see review response) |
| nemotron, 2000 cases, exact | P 0.8340 (fp 471), R 0.8699 | P 0.8329 (fp 475), R 0.8699 | CREDIT_CARD P 0.5714 (fp 18) to 0.5217 (fp 22), R 0.0945. The four new FPs are card windows on account-number spans (unlabelled gold) |
| gretel, 2000 cases, exact | P 0.3557 (fp 1105), R 0.7673 | P 0.3557 (fp 1105), R 0.7673 | CREDIT_CARD P 0.5094 (fp 26), R 0.4737; IBAN P 0.9556 (fp 2), R 0.9773 |
| nemotron and gretel, 2000 cases, exact, after the card separator and IBAN cut (2026-10-15) | nemotron P 0.8326 (fp 476), R 0.8699; CREDIT_CARD P 0.5106 (fp 23) | gretel P 0.3544 (fp 1111), R 0.7673; CREDIT_CARD P 0.4576 (fp 32); IBAN P 0.9556 (fp 2), R 0.9773 | recall unchanged; the new false positives are over-redaction from the separator and cut changes |
| nemotron and gretel, 2000 cases, exact, after IPv6 detection (2026-10-15) | nemotron P 0.8354 (fp 478), R 0.8916; IP P 0.9884 (fp 2), R 0.9771 | gretel P 0.3673 (fp 1123), R 0.8201; IP P 0.8571 (fp 14), R 0.9767 | IPv6 adds recall; the new gretel false positives are EDIFACT `::` separators and full-form addresses the corpus leaves unlabelled |
| nemotron and gretel, 2000 cases, exact, after the label and containment fixes (2026-10-15) | nemotron P 0.8345 (fp 481), R 0.8916; IP P 0.9716 (fp 5), R 0.9771 | gretel P 0.3669 (fp 1125), R 0.8201; IP P 0.8400 (fp 16), R 0.9767 | the new positives are unlabelled IPv6 addresses (nemotron), and EDIFACT `::` separators plus unlabelled addresses (gretel) |
| nemotron and gretel, 2000 cases, exact, re-measured 2026-10-19 on the current tree | nemotron P 0.8357 (fp 477), R 0.8916; IP P 0.9828 (fp 3), R 0.9771 | gretel P 0.3677 (fp 1121), R 0.8201; IP P 0.8400 (fp 16), R 0.9767 | the drop from fp 481 to 477 came after the 2026-10-15 row and is not attributed here; SECRET is out of schema for both corpora (38 and 11 detections, not scored; nemotron rose from 32 on 2026-10-19 when secret spans became value-only and admitted five 8-to-19-character passwords that the whole-match floor had dropped, and from 37 to 38 when a JWT with a 20-character header (a jwt.io example token) was matched; headline numbers did not change) |
| nemotron and gretel, 2000 cases, exact, after the secret class rule was removed and JSON-quoted and spaced labels were admitted (2026-10-19) | nemotron P 0.8357 (fp 477), R 0.8916; gretel P 0.3677 (fp 1121), R 0.8201; SECRET out of schema 38 and 11 | same headline, every per-type cell unchanged; SECRET out of schema 47 (nemotron) and 12 (gretel) | the class rule adds no corpus detection (no corpus value failed the old rule). The 9 new nemotron and 1 new gretel SECRET detections are labels with a quoted key (`"api_key": "..."`, `"Password": "..."`, `"TokenSecret": "..."`) or spaces before the separator (`password = ...`). SECRET is out of schema, so these are not scored |
| nemotron and gretel, 2000 cases, exact, after sentence-final password periods and trailing dotted tokens were handled (2026-10-19) | nemotron P 0.8357 (fp 477), R 0.8916; gretel P 0.3677 (fp 1121), R 0.8201; SECRET out of schema 47 and 12 | same headline and per-type cells; SECRET out-of-schema counts unchanged; 7 SECRET spans changed: six password values lose a sentence-final `.` (five nemotron, one gretel), and one nemotron JWT followed by a fourth dotted segment is captured whole (before `; Path=/auth`). SECRET is out of schema, so these are not scored |
| nemotron and gretel, 2000 cases, exact, after the short-label left boundary (2026-10-19) | nemotron P 0.8357 (fp 477), R 0.8916; gretel P 0.3677 (fp 1121), R 0.8201; SECRET out of schema 47 and 12 | same headline and per-type cells; SECRET out-of-schema counts unchanged at 47 and 12. Raw SECRET spans, diffed per case before and after the boundary, are identical in both corpora, so the boundary drops no corpus secret |
| nemotron and gretel, 2000 cases, exact, after a password's trailing closing bracket is dropped (2026-10-19) | nemotron P 0.8357 (fp 477), R 0.8916; gretel P 0.3677 (fp 1121), R 0.8201; SECRET out of schema 47 and 12 | same headline and per-type cells; SECRET out-of-schema counts unchanged at 47 and 12. Raw SECRET spans, diffed per case against HEAD (356756a) over both corpora, change once: gretel `Password: ufmE%C9zdGiFo!o1)` now reports `ufmE%C9zdGiFo!o1`. SECRET is out of schema, so this is not scored |
| nemotron and gretel, 2000 cases, exact, after provider tokens with no label were added (2026-10-19) | nemotron P 0.8357 (fp 477), R 0.8916; gretel P 0.3677 (fp 1121), R 0.8201; SECRET out of schema 63 and 25 | same headline and per-type cells; SECRET out-of-schema counts rose from 47 to 63 and from 12 to 25. Every new span was diffed: each one sits on a corpus span labelled `api_key` or `http_cookie` (outside the type map), except three gretel `sk_live_` cells in case 001702 that the corpus does not annotate (see TODO.md). No span was lost to overlap. Presidio output is unchanged. |
| nemotron and gretel, 2000 cases, exact, re-measured 2026-10-21 after the credit-card keyword gate, the card-over-phone overlap rule and the PERSON / ORGANIZATION / LOCATION and SECRET label mappings | nemotron P 0.8357 (fp 522), R 0.2886; gretel P 0.3816 (fp 1128), R 0.0957 | The recall drop is a change in gold, not a detector regression: PERSON, ORGANIZATION and LOCATION became in-scope on 2026-10-20 and this pipeline implements none of them (nemotron fn 3275 / 1988 / 1018, gretel 2577 / 1489 / 2374), and `api_key` / `http_cookie` became scored SECRET the same day. Per type: CREDIT_CARD nemotron P 0.9405 (fp 15), R 0.9331 — the keyword gate alone gives 0.9252 and the last 0.0079 is the overlap rule — gretel 0.6875 / 0.7719; PHONE nemotron 0.5679 / 0.9982 with fp 425 to 423, gretel 0.2857 / 0.8580 (the phone-form extension in ca81df3 had already lifted gretel from 0.2766 / 0.8179). Negatives 52 of 71 and 231 of 280; upper bound P 0.9808 and 0.3843. Seed, Presidio and the NER gate unchanged. |
| Presidio credit_card (2026-10-15) | tp 13, fp 1, fn 3 (exact R 0.8125) | tp 14, fp 2, fn 2 exact (R 0.8750); overlap 15, 1, 1 (R 0.9375) | the merged cluster covers two adjacent gold cards |
| Presidio iban | tp 155, fp 0, fn 3 | tp 158, fp 0, fn 0 | country-length table (2026-10-16): tp 158, fp 0, fn 0, P 1.0000, R 1.0000 (dash separators first gave tp 158, fp 3) |

Real-text recall did not move: the corpora have no Luhn-valid CREDIT_CARD gold that was missed (0 of the misses pass Luhn on their own), so the fix gains nothing on the corpora and costs nothing there. The differential over all sets (seed, 4000 corpus texts, 590 Presidio cases at the time) loses no detection, and no case has overlapping detections. Anonymize runs without error over the seed, the corpora and every Presidio module.

**Residuals, not fixed.** These stay as TODO items: seven card windows form inside the IBAN bodies in the Presidio IBAN module (out of schema, not scored, and none in the corpora).

**Spaced IBAN followed by uppercase blocks, fixed 2026-10-15.** `GB82 WEST 1234 5698 7654 32 ALL FEES` and `DE89 3704 0044 0532 0130 00 BIC TESTDEFF` now keep the IBAN. `checksum.ibanSpan` tries cuts from the right, not only the last space. Cost, measured: Presidio IBAN exact fp 0 to 3 (P 1.0 to 0.9812, R 0.9937 unchanged). The three are chance mod-97 passes on shorter cuts of negatives (`MC58 A122 2000 0101`, `JO94 CBJO 0010 0000 0000 0131`). Gretel IBAN unchanged (P 0.9556, R 0.9773). A country-length table would remove them (TODO).

**Scope decisions, 2026-10-15 (user).** IPv6 is in scope. `regex.IPRecognizer` reports an IPv6 candidate only when `netip` parses it, so clock times and MAC addresses stay out (commit `06b84dc`). Not in scope, documented: bare domains; national phone formats for Israel, Japan and UK (Presidio uses the `phonenumbers` library, which this dependency-free design does not use); and the Thai extra rule, so Thai stays on mod-11. SSN stays shape-only. Adopting Presidio's never-issued rules needs seed and test changes the user has not approved. Asian IDs: `ph_tin` then `in_gstin`, once their format documents are supplied. No Asian ID recognizer is added yet.

**IPv6 span conventions and residuals, 2026-10-15.** An address after a label colon is reported (`IP:2001:db8::1`, `id:fe80::1`). Two residuals remain, and both are kept (decided 2026-10-16). First, an address glued to a word that ends in digits (`text2001:db8::1`) is read as label plus address, so the leading group stays visible. The same rule is what fixes `id:fe80::1`, so a fix must keep both. Second, a candidate that is not a valid address but whose suffix is (`2a02:4d60:1031:4c3f::85e1:79f2:9122:5234`, eight groups plus `::`) reports the suffix, so its first group stays visible. The whole-run rule that would fix the first case also swallows the label in `id:fe80::1`, since `netip` parses `d:fe80::1`. Against Presidio's IP cases, the exact-span misses are span conventions, not missed addresses: CIDR suffixes (`/24`) and zone IDs (`%eth0`) are outside our span; the unspecified `::` is not redacted by design; and a URL containing an IPv6 host wins the overlap, so the address is redacted inside the URL span. The IP scan is linear, and so is the card-versus-IBAN containment check (`checksum.inside`, binary search).

**Dash- or dot-joined card, fixed 2026-10-15.** A single group is bounded by a space, hyphen or dot, so `4111111111111111-123`, `4111111111111111.123` and `123-4111111111111111` now report the card. Cost, measured: any Luhn-passing group of 13 to 19 digits joined by `-` or `.` is reported. Gretel CREDIT_CARD fp 26 to 32 (timestamps, EDI references, and a decimal such as `50.0000000000000000`). Nemotron CREDIT_CARD fp 22 to 23 (`88.01951444993813`). Card recall is unchanged on both corpora, and Presidio credit_card was exact tp 14, fp 2, fn 2 when this fix was measured (2026-10-15). The 2026-10-19 run is tp 16, fp 1, fn 0 (P 0.9412, R 1.0000), in the Presidio table above.

**Other findings.** Every mismatch class is recorded in TODO.md under "Presidio test cases: open findings".

## Operators (MVP)

| Operator | Behavior | Example |
|----------|----------|---------|
| `replace` | Substitute with fixed value | `john@example.com` → `<EMAIL>` |
| `mask` | Partial masking | `4111111111111111` → `****-****-****-1111` |
| `hash` | Deterministic SHA-256 | `john@example.com` → `a3f5c8...` |
| `redact` | Remove entirely | `john@example.com` → `[REDACTED]` |

`mask` is rune-based: one Unicode character is one slot, so a multi-byte rune is
never split — `é` with `#*` yields `é*`, not the invalid UTF-8 a byte-wise mask
produced. `#` keeps a slot, `*` masks it, any other pattern character is a
separator emitted verbatim, and an empty pattern masks every character. Input
that is not valid UTF-8 is kept byte-faithfully on a `#` slot.

`TailMaskOperator` is the same, except that a value with more characters than the
pattern has slots is aligned to the pattern's end, so `#` keeps the last characters.
The default for PHONE and CREDIT_CARD uses it (decided 2026-10-16): `MaskOperator`
filled from the front and showed middle digits for a 13-digit phone or a 19-digit
card. `MaskOperator` is unchanged.

---

## Structured Data (CSV)

```go
package structured

// ProcessCSV returns the number of rows whose field count differed from the
// header. Such rows are padded with empty trailing cells rather than rejected,
// because ragged CSV is common in real exports and failing the whole file over
// one truncated row is worse than writing an empty cell — but the count lets a
// caller detect that it happened instead of silently trusting the output.
func (bp *BatchProcessor) ProcessCSV(ctx context.Context, r io.Reader, w io.Writer) (int, error)

// PseudonymizeCSV replaces each configured-column value with a numbered
// placeholder, one mapping across the whole file. The map holds raw PII.
func (bp *BatchProcessor) PseudonymizeCSV(ctx context.Context, r io.Reader, w io.Writer) (int, map[string]string, error)
```

Column scope is set by `SetColumnType`, as in `ProcessCSV`: only registered columns are scanned. Header cells are scanned the same way as data cells — a header holding a real value is redacted like any other configured cell. Short rows are padded with empty trailing cells; long rows keep their extra cells unscanned (only configured columns are scanned). The ragged count lets a caller detect ragged input instead of silently trusting the output. A Thai citizen ID cell that passes Luhn is retyped to TH_ID in both CSV paths, matching `Scrubber.Analyze`.

---

## NLP Sidecar (v2, shipped)

The root module stays standard-library-only, so named-entity recognition
(PERSON, ORGANIZATION, LOCATION) lives in a separate module, `sidecar/`, and the
root talks to it over HTTP.

- `sidecar/` is its own Go module (`github.com/u007/dataprotect/sidecar`) with
  `replace github.com/u007/dataprotect => ..`. The root module never imports
  the sidecar, and the sidecar never imports the root's NLP client, so there is
  no recursion.
- `sidecar/cmd/piiscrub-sidecar` serves `GET /healthz`
  (`{"status":"ok","backend":"prose"}`) and `POST /v1/ner` with body
  `{"text":"..."}` and response
  `{"entities":[{"type","start","end","text","confidence"}]}`.
- The service binds `127.0.0.1:8787` by default, caps request bodies at 1 MiB,
  sets `ReadHeaderTimeout` and `WriteTimeout`, and shuts down on SIGTERM.
- The root client is `recognizers/nlp.NLPRecognizer`. An empty URL disables it
  (`Recognize` returns `nil, nil`). A configured-but-broken sidecar is an
  error, because `Analyzer.Analyze` aborts on a recognizer error; the opt-in
  best-effort mode is the only path that drops one.
- `NewScrubber()` stays env-free and deterministic. A caller wires the NLP
  recognizer explicitly with `NewScrubberWithRecognizers(nlp.New(url))` or
  `NewScrubber(WithRecognizer(...))`. The env var `PII_SCRUB_NLP_URL` is read
  by the caller (or `nlp.FromEnv()`), never by `NewScrubber()`.
- NLP entities carry confidence 0.80. Overlap resolution keeps the higher
  confidence; every built-in type scores at least 0.85 (the keyword-gated
  national ID and passport rules; regex, checksum and secret are 0.95 and up),
  so NLP is strictly outranked and fills gaps rather than overriding typed
  entities. 0.80 is also above the default 0.70 floor, so NLP entities survive
  the filter. Both boundaries are guarded by tests:
  `TestNLPConfidenceWindow` (root module) and `TestDefaultConfidenceWindow`
  (sidecar). The score was 0.85 originally, which tied the national rules;
  lowering it to 0.80 makes "never override typed" true rather than
  tie-dependent.
- The floor is public: `NewScrubber(WithMinConfidence(t))` overrides the
  default 0.70. A threshold above 0.80 filters every NLP entity out and leaves
  the built-in recognizers only, which is the supported conservative mode. The
  option changes the filter, not overlap resolution; overlap still keeps the
  higher-confidence span.
- The NER golden cases are not in `testdata/golden/seed.json`, because their
  result depends on a running sidecar and would make the offline gate
  nondeterministic. They live in `testdata/golden/ner/` and are scored by the
  `integration`-tagged gate (`ner_accuracy_test.go`, run by `make test-ner`).
- **Measured NER accuracy.** prose's own entity output is NOT used directly:
  its coalescer keeps the first token's label for a span, so a company name
  came back as a GPE and a person's surname came back as ORGANIZATION. The
  sidecar decodes prose's TOKEN labels instead (`sidecar/internal/ner/prose.go`),
  with shape rules for legal forms, institutional head nouns, business head
  nouns, place head nouns, and closed classes that are never entities
  (months, seasons, languages, greetings, courtesy titles, communication
  verbs).
- The gate measures TWO numbers, because the NER types share one operator
  (redact) and so a mistyped span is still redacted:
  - **typed** (the golden package's own, type-sensitive): P=0.8395 R=0.8718
    over 124 cases (tp=68 fp=13 fn=10).
  - **span** (type-agnostic, computed in the gate): P=0.9012 R=0.9359
    (tp=73 fp=8 fn=5). This is the number that protects the caller's data,
    because it is the one that says the right bytes were covered.
- The dataset has FOUR files, and the fourth exists because the corpora
  exposed a blind spot. `testdata/golden/ner/structured.json` (32 cases)
  covers JSON keys, HTTP headers and spreadsheet field labels. The first
  three sets were all prose, and the corpora showed the decoder reporting
  "Record Number", "Account Number", "SameSite=Lax", "Path=/auth" and
  "Medical Record Number" as people and places. The rules that fix that
  class are: a span carrying a field-path character is a field, and a run
  whose LEADING words are two or more field names is a field label.
- Measured on the two corpora, those two rules cut the NER detections on
  the nemotron corpus from 4.4 to 2.76 per text, and on gretel from 6.6 to
  5.90. The gate was mutation-verified against them: disabling the
  field-label rule drops typed precision to 0.7907 and the gate fails.
- The generalisation figure is the FIRST evaluation of the held-out set, on
  text the decoder had never seen: typed P=0.5333 R=0.6154, span P=0.8667
  R=1.0000. The gate bounds sit below the development-set baseline, and the
  honest first-run numbers are recorded in `ner_accuracy_test.go` rather than
  the flattering re-run.
- **The corpora score it too, and the numbers are much worse.** The person,
  organisation and place labels were added to the converter's type maps on
  2026-10-20, so the corpora's own `first_name`, `last_name`, `name`,
  `company_name`, `company`, `city`, `country`, `state`, `county`, `postcode`
  and `street_address` spans are now gold rather than dropped. Scored on the
  same 2000 nemotron cases, re-measured 2026-10-21, regex row from
  `make corpus` and sidecar row from `make corpus-nlp NER_LIMIT=2000`, both
  through the same scorer:

  | Pipeline | exact P | exact R | overlap P | overlap R |
  |---|---|---|---|---|
  | regex + checksum + national + secret | 0.8357 | 0.2886 | 0.8455 | 0.2919 |
  | with the NLP sidecar | 0.4376 | 0.4135 | 0.5222 | 0.4935 |

  The regex-only pipeline scores R=0.2886 because PERSON, ORGANIZATION and
  LOCATION are now in scope and it detects none of them. That is the
  baseline the NLP path has to beat, and it beats it on recall (0.2919 to
  0.4935 overlap) at the cost of precision (0.8455 to 0.5222).
- **Why the corpus score is so much worse than the hand-written gate.** Two
  reasons, both measured:
  - **Granularity.** The corpora annotate a GIVEN NAME ("Manal", "King")
    while a NER model reports a FULL NAME ("Manal King"). The span is right
    and the granularity differs, so an exact-span match under-counts. On the
    nemotron sample, 74% of the gold spans are covered by the detector at
    all, but only 35% match exactly; of the covered-but-larger spans, 166
    of 174 are PERSON.
  - **Field labels.** The corpora are full of form fields ("First Name:",
    "Street Address:", "Account Number"), and the detector reports some of
    them. The rules that suppress that class are the ones described above.
- **The per-type truth on the corpus**, with the NLP sidecar: LOCATION
  P=0.2457 R=0.3556, ORGANIZATION P=0.3095 R=0.1660, PERSON P=0.1307
  R=0.0837. The built-in types are untouched by the sidecar (EMAIL
  P=0.9975 R=0.9975, SSN P=0.8780 R=1.0000, URL P=0.9900 R=0.9229), which
  is the point: the NLP path adds coverage without disturbing what already
  worked. `CREDIT_CARD` (0.9405 / 0.9331) and `PHONE` (0.5679 / 0.9982) are
  byte-identical in both rows of the table above.
- **The honest conclusion.** On prose the decoder is usable; on form-filling
  structured records it is not. The corpora are mostly the latter, and the
  numbers above are what a caller who enables the sidecar on such data gets.
  A caller who enables it on running prose gets the gate's numbers. The
  difference between those two figures is the whole of the NER path's
  quality, and it is now measured rather than assumed.
- Known limitation: a brand name with no other signal ("The Orion laptop")
  is reported as a location. A gazetteer of brand names would fix it and
  would be wrong the moment the list is out of date, so it is recorded as a
  cost rather than papered over.

### What the NLP path costs

The NLP path is a separate process and it is the slow path. Measured on the
239-byte sample in the NER gate's benchmark, per `Analyze` call:

| Path | Time per call | vs. the regex-only path |
|---|---|---|
| regex + checksum + national + secret (no NLP) | 305 us | 1x |
| with the NLP sidecar | 19.6 ms | 64x |

The 64x is not the model. It was 1334x before the model was hoisted, and
that number was a bug: `prose.NewDocument` loads and decodes the 17 MB model
on EVERY call when it is given no model, so the sidecar was paying 340 ms of
model loading per request. The sidecar now builds the model once, on the first
request, and reuses it (`prose.UsingModel`). That is a 17x speedup on the
sidecar, and the token output is byte-identical — the model is read-only once
built. The shared model is safe for concurrent requests; a race test covers it.

After that fix the sidecar sustains about 50 requests/second sequentially and
about 130/second with 16 concurrent callers.

The remaining 19.6 ms is the tagger (161 ms of the original 352 ms was the
perceptron tagger) plus sentence segmentation (14 ms). Both are inherent to
prose. Cutting further would mean a different model, which is Phase E.

So: the NLP path costs two orders of magnitude more CPU than the regex path.
That is the price of finding names in prose, and it is why the recognizer is
opt-in and why `NewScrubber()` never reaches for it on its own.

---

## Dependencies (Pinned)

| Dependency | Purpose | Version |
|------------|---------|---------|
| [prose](https://github.com/jdkato/prose) | NLP NER, in the `sidecar/` module only (not the root module) | v2.0.0 |
| [go-face](https://github.com/Kagami/go-face) | Facial recognition (v2+) | v0.0.0-20240518085811-770585e |

---

### go-face model bundling strategy

The [go-face](https://github.com/Kagami/go-face) library binds to dlib (a C++ ML toolkit) and requires two pre-trained model files:

- `shape_predictor_5_face_landmarks.dat` (~22 MB)
- `dlib_face_recognition_resnet_model_v1.dat` (~97 MB)

**Decision: download on first use.** Model files are NOT bundled with the binary. On the first call that needs face detection, each missing model is downloaded from dlib.net, decompressed from `.bz2`, and stored in the OS user cache directory (`~/.cache/dataprotect/face-models/` on Linux). Subsequent calls reuse the cached files.

Rationale:
- Bundling ~120 MB of model files would bloat the binary for users who never use face detection.
- Download-on-first-use keeps the initial download small and only incurs the cost when face detection is actually needed.
- Models are cached persistently, so repeated use has no download overhead.
- Download failures are retried (up to 3 times) and do not prevent future attempts.

Implementation: `sidecar/internal/facedetect/` handles model cache management, download-on-first-use with thread-safe deduplication, and a `FaceDetector` interface. A `StubDetector` is provided that returns `ErrDlibUnavailable` until `libdlib-dev` is installed and go-face is imported.

### FPE library evaluation

The [capitalone/fpe](https://github.com/capitalone/fpe) library (FF1/FF3 per NIST SP 800-38G) was evaluated as a potential dependency for format-preserving encryption. The decision is to **keep the custom FF1 implementation** in `operators/fpe.go` for these reasons:

- The root module is standard-library-only; capitalone/fpe is an external (Apache-2.0) dependency.
- Our implementation supports radix 2–62; capitalone/fpe's ff1 package supports radix 2–36.
- Our implementation provides both `Encrypt` and `Decrypt`; capitalone/fpe's ff1 package provides `Encrypt`/`EncryptWithTweak` only.
- Our implementation allows 8–10 Feistel rounds; capitalone/fpe fixes 10 rounds.
- Our implementation passes all round-trip, determinism, key/tweak sensitivity, and invalid-input tests (`operators/fpe_test.go`).

## Testing Strategy

### Golden Dataset
- ~500 hand-labeled samples — **553 exist** in `testdata/golden/seed.json`
  (353 positive, 200 negative), past the ~500 target
- CI regression testing on every commit: `golden/accuracy_test.go` gates on
  precision/recall thresholds and fails if the dataset drops below 553 cases (ratcheted with each batch)
- Precision/recall/F1 per entity type, plus aggregate, under two matching
  policies (exact span and byte overlap)

Run `scripts/benchmark.sh` to print the current numbers. Thresholds in
`golden/accuracy_test.go` are set to the measured baseline and must be raised
deliberately as the dataset grows — they are not a target. The gate is
mutation-verified, re-run at 50 cases (stripping branch 3's `\b` raises exact fp from
1 to 4 and the gate fails): removing the `\b` anchors from the phone recognizer (the
historical card-vs-phone defect) fails it.

Labels are hand-written from the text, never copied from detector output.
A dataset derived from the detector's own predictions cannot fail and therefore
proves nothing.

### Benchmark Datasets (v2+)
| Dataset | Source |
|---------|--------|
| OpenNER 1.0 (English) | arXiv |
| Financial PII | Mendeley |
| Kaggle PII Detection | Kaggle |
| CoNLL 2025 NER | Hugging Face |
| ai4privacy/pii-masking-43k | Hugging Face |
| PII-Bench | arXiv |
| SecretBench | arXiv |
| secrets-patterns-db | GitHub |

---

## Implementation Phases

| Phase | Scope | Status |
|-------|-------|--------|
| **1 (MVP)** | Core interfaces + regex + checksum + 4 operators + context boosting + structured + golden dataset | **SHIPPED**, except: the CI golden set is a 553-case seed (past the ~500 target); the corpora are measured on demand instead |
| 2 | NLP recognizer (prose) | SHIPPED (sidecar module + opt-in client; Phases A/B/D of the NLP plan) |
| 3 | Secret recognizers (patterns + entropy) | SHIPPED |
| 4 | FPE operator | Later |
| 5 | Image PII detection (go-face) | Later |
| 6 | REST API sidecar | Later (the NLP sidecar in `sidecar/` exposes /healthz and /v1/ner; /v1/analyze and /v1/anonymize are the deferred part) |

---

## Success Criteria (MVP)

- [ ] All regex recognizers >99% precision/recall on golden dataset — **NOT
      MET, now measured.** The seed set reads exact P=1.0000 R=0.9974 and overlap
      P=1.0000 R=0.9974 over 553 cases, but not every per-recognizer cell is above
      0.99: PHONE recall is 0.9828 (the `pos-phone-trunk-zero` miss). The corpora are
      now measurable and do not come close: exact aggregate
      P=0.8357 R=0.2886 (nemotron) and
      P=0.3816 R=0.0957 (gretel) over 2000 records each — see "Corpus
      measurement (2026-10-19)" above for the per-type numbers and the
      explanation of every large gap. BITCOIN remains unmeasured because
      neither corpus labels it. Reproduce with `make corpus`.
- [x] Luhn + IBAN checksum validation working — **MET.** Both validators are
      wired into the default pipeline. Cards are detected by Luhn, plus a
      label-style keyword-gated window fallback at 0.85 for a Luhn-failing
      number that a label names (2026-10-21); the regex card recognizer is
      excluded because wiring both cannot work, since for a Luhn-failing number
      the validator emits nothing while the regex recognizer still matches the
      shape. CREDIT_CARD went from P=0.7500 to P=1.0000 on the seed set, and the
      corpus recall went from 0.0945 to 0.9331 (nemotron) and 0.4737 to 0.7719
      (gretel).
- [x] 4 operators (replace, mask, hash, redact) working
- [x] Structured data CSV processing working
- [x] Full test suite passes
- [x] README with usage examples
