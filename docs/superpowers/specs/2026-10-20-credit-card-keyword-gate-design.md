---
type: Decision
title: "Credit-card keyword-gated rule — design"
description: "Hybrid keyword gate for Luhn-valid card numbers; implemented 2026-10-21 in recognizers/checksum"
resource: recognizers/checksum/checksum.go
tags: [credit-card, checksum, design]
timestamp: 2026-11-03T11:51:49Z
status: active
---

# Credit-card keyword-gated rule — design (2026-10-20)

Status: IMPLEMENTED 2026-10-21. Decision: 1.c (hybrid). Implementation:
`recognizers/checksum/checksum.go` (`cardLabelPhrases`, `hasCardLabelKeyword`,
`standardCardLayout`, `windowBoundedBySeparator`); tests in
`recognizers/checksum/keyword_gate_test.go`.

## 0. Measured result (2026-10-21)

| Dataset | CREDIT_CARD P | CREDIT_CARD R | tp / fp / fn |
|---|---|---|---|
| Nemotron, before | 0.6154 | 0.0945 | 24 / 15 / 230 |
| Nemotron, after | **0.9400** | **0.9252** | 235 / 15 / 19 |
| Gretel, before | 0.5745 | 0.4737 | 27 / 20 / 30 |
| Gretel, after | **0.6875** | **0.7719** | 44 / 20 / 13 |
| Presidio, before | 1.0000 | 0.8750 | 14 / 0 / 2 |
| Presidio, after | 0.8750 | 0.8750 | 14 / 2 / 2 |

The gate adds **no false positives on either corpus**: the fp counts are
unchanged at 15 (nemotron) and 20 (gretel), and every remaining one was already
present under Luhn-only (`account_number` / `device_identifier` / `bban`
labels, or unlabelled). The headline moves with it: nemotron exact P 0.8233 to
0.8351 and R 0.2654 to 0.2883; gretel P 0.3758 to 0.3816 and R 0.0934 to
0.0957. PHONE is untouched (nemotron 0.5668/0.9982, gretel 0.2857/0.8580 before
and after).

The seed set is unchanged (exact P=1.0000, its thresholds did not move). The
two Presidio FPs are exactly the predicted 0016/0018 negatives; the two FNs
(0002/0003) are prefix-1 gold cards and predate this change. Both seed
negatives that the whole-run prototype broke (`neg-card-luhn-two`,
`neg-card-luhn-invalid-then-digits`) stay clean, because bare "card" is not a
label. Measured by swapping this file's version of `checksum.go` against the
pre-change one over the same directories.


## 1. Problem

Corpus credit cards use random check digits, so Luhn alone detects ~9.4% of nemotron
gold cards (24/254). 219 of the 230 misses carry a card keyword (card, credit, debit,
visa, mastercard, amex, discover, jcb, diners, cvv, cvc) within 40 bytes. The
keyword-gated rule would recover most of them.

## 2. Why the naive attempt failed

A whole-run fallback (report the entire digit run at 0.85 when a keyword is nearby)
was tried and reverted: it dropped seed precision from 1.00 to 0.91 and recall from
1.00 to 0.95 because:

- `neg-card-luhn-two` ("4111111111111112" next to the word "card") became an FP.
- `neg-card-luhn-invalid-then-digits` became an FP.
- Three positive cases changed span (extra CVV/year groups included).

The whole-run fallback cannot distinguish a real card from a near-miss number that
happens to sit next to the word "card".

## 3. Proposed design (window-level, not whole-run)

Apply the keyword gate inside the existing window search, not on the whole run:

1. Keep the Luhn-passing window path unchanged (0.99).
2. For windows that FAIL Luhn but have a valid network prefix and a plausible card
   length (13-19 digits), report them at 0.85 ONLY when ALL of:
   - a card keyword sits within 40 bytes of the window (case-insensitive), AND
   - the keyword is a *label-style* token (one of: "card number", "card no", "card:",
     "credit card", "debit card", "cc", "cvv", "cvc", or a network name followed
     by a separator), not the bare word "card" in prose, AND
   - the window is bounded by a separator or candidate edge on both sides
     (boundedBySeparator), so a number glued to a neighbour is never gated.
3. Never gate a window whose digit count is outside 13-19.
4. Confidence 0.85 so phone (0.98) and other higher-confidence types still win
   overlap resolution.

This keeps the seed negatives clean (their keywords are prose "card", not
label-style) while recovering corpus cards (whose text uses "Card:", "credit card",
etc.).

## 4. Phone-vs-card precedence (B2)

Under the keyword rule at 0.85, phone (0.98) still wins overlap, so the
nemotron PHONE FPs on card tails are unchanged. No separate code change needed;
B2 is "follows B5".

## 5. Threshold impact

Implementing this will move the seed baseline. Expected: precision stays 1.00
(label-style gating avoids the two known negatives); recall may rise if any seed
positive currently misses (none expected). The gate thresholds in
golden/accuracy_test.go must be ratcheted to the new measured values and the
change documented in TODO.md.

## 6. Implementation steps

1. Add `hasCardLabelKeyword(text, start, end)` (stricter than `hasCardKeyword`).
2. In `cardSpans`, in the window loop, after the Luhn check fails, add a gated
   branch that appends the window at 0.85 when conditions hold.
3. Ensure windows are not double-reported (a gated window must not also be merged
   into a Luhn cluster).
4. Update tests + ratchet thresholds.

## 7. Risks

- A label-style keyword can still appear next to a non-card number in prose
  ("card number: 1234567890123456" where the number is an order code). Mitigate
  by requiring a valid network prefix and standard layout.
- Overlap merging: a gated window adjacent to a Luhn window could merge into one
  span; handle by tagging each window with its confidence and not merging across
  confidence levels.

## 8. Open question

Whether "card" alone (without a label) should gate. Corpus data says yes for recall,
seed data says no for precision. The label-style compromise is the recommendation.
If you want maximum corpus recall, accept the seed precision drop and ratchet.