---
type: Decision
title: Finance and law redaction types — decision, overlap policy and measured state (2026-11-22)
description: 'Dated 2026-11-22 decision record: 19 finance, identity, national-ID and legal entity types added (38 total), all on by default with WithoutEntityTypes as the opt-out; the keyword-gate rule for keyword-only types, the overlap policy, the measured state (655-case seed exact P=1.0000 R=1.0000; corpus figures and per-type numbers for the new types), known false firings, unverified items and open decisions.'
resource: recognizers/finance; recognizers/identity; recognizers/legal; recognizers/terms; recognizers/national; recognizers/checksum; analyzer/analyzer.go
tags:
  - finance
  - legal
  - entity-types
  - overlap
  - redaction
  - decision
timestamp: 2026-11-22T19:26:13Z
status: active
---
# Finance and law redaction types (2026-11-22)

Date: 2026-11-22. The user decided to add finance and law redaction. Law
jurisdictions in scope: Malaysia, Singapore and US federal. The authoritative
detail is in the root `SPEC.md`, `README.md`, `AGENTS.md`, `TODO.md` and
`SOURCES.md`; this record keeps the decision, the reasons and the open items in
one place, and does not repeat those files.

## Decision

- 19 new entity types, 38 in total. All are on by default.
- Opt-out: `piiscrub.WithoutEntityTypes(types...)`, backed by
  `Analyzer.DisableEntityTypes`. A disabled type's entities are dropped right
  after the recognizers run, before boost, judge, confidence filter and overlap,
  so a disabled type cannot suppress another type's span.
- Keyword-only types: the user confirmed that a labelled corpus example satisfies
  the project's "no unverified regex shape" rule for a keyword-only type.

## The 19 types

| Group | Types | Rule | Score |
|---|---|---|---|
| Finance, keyword-gated | `SWIFT_BIC`, `US_ROUTING`, `CARD_CVV`, `BANK_ACCOUNT`, `PIN`, `ISIN`, `MONEY` | label keyword; `US_ROUTING` uses the ABA checksum with prefix ranges; `ISIN` also needs Luhn | 0.85 (`MONEY` 0.85) |
| Finance, checksum | `LEI` | ISO 7064 MOD 97-10, no keyword | 0.90 |
| Finance, checksum | `ETHEREUM` | EIP-55 mixed-case checksum; all-lower and all-upper accepted by shape | 0.95 |
| Identity, keyword-gated | `TAX_ID`, `PASSPORT`, `DRIVER_LICENSE`, `DATE_OF_BIRTH` | label keyword; `TAX_ID` uses IRS EIN prefixes | 0.85 |
| National, keyword-gated | `IN_PAN`, `SG_UEN`, `MY_SSM`, `MY_TIN` | label keyword | keyword-gated |
| Law | `CASE_NUMBER` | US federal docket shape and Singapore court/tribunal forms at 0.90, no keyword; a generic label-gated form at 0.85 covers Malaysia; neutral citations such as `[2021] SGHC 123` are never matched | 0.90 / 0.85 |
| Law, caller-supplied | `CUSTOM_TERM` | exact caller terms in `recognizers/terms`; not in the default pipeline; added with `WithRecognizer` | 1.00 |

Implementation notes:

- `ETHEREUM` uses a hand-written Keccak-256 in `recognizers/checksum`, because the
  standard library's `crypto/sha3` provides only the NIST SHA-3 variant.
- `SG_UEN`: the check letter is not published, so the rule is shape only.
- `MY_SSM`: 12 digits or the old `NNNNNNN-L` form; digit groups are not validated.
- `MY_TIN`: `IG` plus 9 to 11 digits; non-individual codes are included.
- `TAX_ID`: an SSN-format value is reported as `SSN`, because `SSN` wins the overlap.
- `PASSPORT` is the generic type; `MY_PASSPORT` and `SG_PASSPORT` keep their own rules.

Legal role words (plaintiff, defendant, borrower, guarantor, and Malay forms) are
added to `internal/context` as boost words for PERSON and ORGANIZATION. They
apply only when `WithContextBoost` is opted into.

The shared label gate lives in `internal/keyword` (`LabelGate`).

## Overlap policy

In `winsOverlap` (`analyzer/analyzer.go`):

- `CUSTOM_TERM` wins every overlap.
- `MONEY` loses every overlap.
- Every keyword-gated type beats `PHONE` and loses to `CREDIT_CARD`.

In the yields table:

- `TAX_ID` loses to `SSN`, `PH_TIN`, `IN_GSTIN` and `MY_TIN`.
- `PASSPORT` loses to `MY_PASSPORT` and `SG_PASSPORT`.
- `IN_PAN` loses to `IN_GSTIN`.
- `MY_IC` loses to `MY_SSM`.

## Measured state (2026-11-23)

Verified on 2026-11-23 by re-running the tests:

- `go test ./golden/` passes and `go test ./...` passes. `make test-tools` and
  `make test-race` were also reported passing for this change.
- Seed: 655 cases (412 positive, 243 negative), 449 gold spans. Exact and overlap
  both P=1.0000 R=1.0000, tp=449 fp=0 fn=0. All 243 negative cases are clean.
  The `minCases` gate is 655.

Corpus measurement (`make corpus`, 2000 records each, exact span). The corpus run
itself was not repeated for this record:

| Corpus | Precision | Recall | tp / fp / fn |
|---|---|---|---|
| nemotron | 0.8841 | 0.3326 | 3434 / 450 / 6892 |
| gretel | 0.4397 | 0.1192 | 922 / 1175 / 6815 |

Per-type figures for the new types (precision / recall):

| Type | nemotron | gretel |
|---|---|---|
| `BANK_ACCOUNT` | 0.9927 / 0.8042 | 0.2143 / 0.3396 |
| `CARD_CVV` | 1.0000 / 0.7955 | 0.9048 / 0.3455 |
| `PIN` | 1.0000 / 0.7213 | 1.0000 / 0.5957 |
| `SWIFT_BIC` | 1.0000 / 0.7083 | 0.7619 / 0.4211 |
| `US_ROUTING` | 1.0000 / 0.1209 | 0.2500 / 0.0179 |
| `TAX_ID` | 1.0000 / 0.1935 | not reported |
| `DATE_OF_BIRTH` | 0.9922 / 0.9442 | 0.9452 / 0.8214 |
| `PASSPORT` | not reported | 1.0000 / 0.5333 |
| `DRIVER_LICENSE` | not reported | 1.0000 / 0.4490 |

Reasons for the low numbers:

- `US_ROUTING` recall is low because most corpus routing numbers fail the ABA
  checksum (1 of 56 in gretel, 24 of 182 in nemotron pass it).
- Gretel `BANK_ACCOUNT` precision: 61 of its 66 false positives are real account
  numbers that the corpus leaves unlabelled. This is a label gap, not a detector
  claim.

Not measured on the corpora, because they have no labels: `LEI`, `ISIN`,
`IN_PAN`, `ETHEREUM`, `SG_UEN`, `MY_SSM`, `MY_TIN`, `CASE_NUMBER`, `MONEY` and
`CUSTOM_TERM`.

## Known false firings and open decisions

- Known false firing: `MONEY` on a currency code followed by digits with no space
  (for example `USD987654321` in SWIFT message fields). This gave 155 detections
  in gretel.
- Open decision: whether `TAX_ID` should beat `SSN` when a tax keyword names the value.
- Not implemented: `BAR_NUMBER`. There is no labelled example and no source.

## Unverified items

- Malaysian court case-number shape: the only examples are printed in published
  judgments. No official format page was found.
- Singapore pre-2022 court prefixes `HC`/`OS` and `SIC`/`S`: label-gated only.
- ISO 4217 codes other than AUD came from an extracted summary, not the primary source.
- The currency symbols €, £, ¥ and US$ were not fetched.
- The `SG_UEN` check letter and the `MY_SSM` digit-group rules are unverified (see above).

## Sources

All sources were fetched 2026-11-22 and are listed in `SOURCES.md`.
