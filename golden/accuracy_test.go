package golden_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	piiscrub "github.com/u007/dataprotect"
	"github.com/u007/dataprotect/golden"
)

// datasetDir is the checked-in hand-labelled seed set. It is relative to this
// package, so the gate runs from any working directory.
const datasetDir = "../testdata/golden"

// minCases guards against a green gate produced by an empty or truncated
// dataset. With few cases every metric is coarse, and a single flipped case
// moves recall by a large fraction — so a shrinking dataset would silently
// weaken the gate rather than fail it. Ratcheted with each batch: it tracks the
// current seed count, so any drop below it fails.
const minCases = 491

// Gate thresholds.
//
// These are the MEASURED baseline, not a target. The seed set is 491 cases, close to
// short of the ~500 SPEC.md calls for; claiming ">99% precision/recall" here would
// make the gate a rubber stamp. Each constant is the EXACT measured value, so
// any regression fails immediately and any improvement is a deliberate
// threshold bump in this file rather than a silent drift.
//
// Defects remain and are recorded, not relabelled:
//   - neg-ip-version-four-part: "3.10.2.7" has valid IPv4 octets and is read as
//     an IP address (one negative case not clean). See TODO.md.
//   - pos-card-then-year: the card window takes the trailing year, so the exact
//     span is "4111 1111 1111 1111 2024" (one exact FP, one exact FN). Overlap
//     recall is unaffected. See the card-window entry in TODO.md.
//   - pos-bitcoin-bech32-upper: an all-caps bech32 address is valid under BIP-173
//     and is missed. See TODO.md (bitcoin coverage).
//   - pos-phone-trunk-zero: a UK national number (`020 5550 1290`) is missed. The
//     national formats are out of scope by decision (TODO.md, phone).
//   - pos-url-upper-scheme: `HTTPS://DOCS.EXAMPLE.COM/START` is missed. RFC 3986
//     makes the scheme case-insensitive. See TODO.md (URL).
//   - pos-card-tab-groups: a card in tab-separated groups is missed. The card
//     separator set is space, dash and dot. See TODO.md (card separators).
//
// Batch 9 added 81 hand-written cases over the mainstream types: email, phone,
// SSN, IPv4 and IPv6, URL, card, IBAN and bitcoin, each with a negative
// neighbour. Labels were typed from the text. No case was dropped for missing the
// detector, except two SSN negatives (area 000 and 666) that assumed a validity
// rule the project rejects: SSN is shape-only by decision (TODO.md). Batch 10
// added 57 more, mixing types in one text and adding negatives that look like PII
// shapes (order codes, digests, section numbers). Batch 11 added 60 more: a
// paragraph-length case per type, and negatives built from counts, times and
// config values. Its one new miss is the upper-case URL scheme listed above.
// Batch 12 added 62 more: Japanese and Thai sentences, JSON, CSV, Markdown and
// code-string placements, and negatives built from counters, versions and
// multipliers. Its one new miss is the tab-separated card listed above. Batch 13
// added 59 more: multi-address lines, toll-free and international numbers, and
// negatives built from schedules, ratings and codes. It added no new miss. Batch
// 14 added 83 more across all types, with non-Latin negatives, and added no new
// miss.
//
// Batch 3 added the national IDs (SG NRIC/FIN, MY MyKad, TH citizen ID). All
// eight positives were detected and the five invalid-checksum negatives stayed
// clean. Batch 4 added Malaysia passports (keyword-backed): three positives were
// detected and the two negatives stayed clean. Batch 6 added two Luhn-valid Thai IDs,
// typed TH_ID by decision (d). Batch 5 added a label-glued SG NRIC and
// a keyword-gated bare MyKad: two positives were detected and the two negatives
// stayed clean. Batch 7 added Singapore passports (keyword-backed, E or K prefix):
// two positives were detected and the one negative stayed clean. Batch 8 added
// bitcoin (taproot and P2SH positives, two bad-checksum negatives): all four
// behaved as expected. The one defect above is still the only false positive.
//
// The measured baseline is exact P=0.9941 R=0.9855 (tp=339 fp=2 fn=5), overlap
// P=0.9971 R=0.9884 (tp=340 fp=1 fn=4), and 176 of 177 negative cases clean. These
// numbers describe this hand-labelled seed set, NOT the detector in general —
// SPEC.md's ">99%" needs a ~500-case hand-labelled set. The corpora in
// testdata/external are scored by `make corpus` but are not that set (see
// TODO.md). With 341 predictions one flipped case moves precision by under 1%,
// so the number is kept exact so a regression fails immediately.
//
// An earlier baseline (exact P=0.8571 R=0.9231, tp=12 fp=2 fn=1) preceded the
// checksum-validator wiring. CREDIT_CARD went from P=0.7500 to P=1.0000 and all
// negative cases became clean, because cards are now validated rather than
// matched on shape alone.
//
// While a threshold sits below 1.0, write it as the measured tp/(tp+fp) and
// tp/(tp+fn) RATIO rather than a rounded literal: 10/11 is 0.9090909..., so a
// literal 0.9091 is a hair ABOVE the true value and the gate would fail on a run
// that exactly met its baseline — a rounding trap, not a regression. Deriving
// the constant from the counts makes the comparison exact and self-documenting.
// A ratio with an equal numerator and denominator (41.0/41.0) adds nothing and
// draws an "identical expressions" lint warning, so a sitting-at-1.0 constant
// stays a literal with its counts in the trailing comment.
// Exact bounds were 61/62 and 1.0 until 2026-10-15. The card-window search now
// merges overlapping windows so every card digit is covered (PII-safe). A chance-
// passing window can extend a card by a neighbouring group: seed
// pos-card-then-year now reports "4111 1111 1111 1111 2024" against gold
// "4111 1111 1111 1111". Overlap recall stays 1.0; exact bounds are re-measured.
const (
	minExactPrecision = 339.0 / 341.0 // tp=339 fp=2 -> 339/341 (see note)
	minExactRecall    = 339.0 / 344.0 // tp=339 fn=5 -> 339/344 (see note)
	minOverlapRecall  = 340.0 / 344.0 // tp=340 fn=4 -> 340/344 (see note)
)

// TestGoldenDatasetAccuracy scores the real detector against the hand-labelled
// seed set and fails on regression.
//
// Labels in testdata/golden are written from the TEXT, never copied from the
// detector's output. Deriving labels that way would make this gate circular: it
// would assert that the detector agrees with itself and could never catch a
// wrong detector, which is the entire purpose of the exercise.
func TestGoldenDatasetAccuracy(t *testing.T) {
	cases, err := golden.Load(datasetDir)
	if err != nil {
		t.Fatalf("loading golden dataset: %v", err)
	}
	if len(cases) < minCases {
		t.Fatalf("golden dataset has %d cases, want at least %d; a small dataset makes every "+
			"metric coarse and lets a regression hide in the rounding", len(cases), minCases)
	}

	scrubber := piiscrub.NewScrubber()
	rep, err := golden.Score(context.Background(), datasetDir,
		func(ctx context.Context, text string) ([]golden.Prediction, error) {
			return scrubber.Analyze(ctx, text)
		})
	if err != nil {
		t.Fatalf("scoring golden dataset: %v", err)
	}

	// A broken label or a detector error is a dataset/tooling failure, not a
	// quality regression. Reporting it separately stops a mislabelled case from
	// being read as "the detector got worse".
	if len(rep.Errors) > 0 {
		t.Errorf("dataset/predictor errors (%d) — these invalidate the run:\n%v", len(rep.Errors), rep.Errors)
	}
	if rep.Exact.TP+rep.Exact.FP+rep.Exact.FN == 0 {
		t.Fatalf("no entities were scored at all; the dataset is not exercising the detector:\n%s", rep.Summary())
	}
	// Without negative cases precision is a tautology: predicting nothing scores
	// perfectly. A dataset of only positive cases must not pass the gate.
	if rep.Exact.Negatives == 0 {
		t.Error("dataset contains no negative cases, so precision is not a real measurement")
	}
	if rep.Exact.NegativesClean != rep.Exact.Negatives {
		t.Logf("note: %d of %d negative cases produced detections (known defects, see testdata notes)",
			rep.Exact.Negatives-rep.Exact.NegativesClean, rep.Exact.Negatives)
	}

	if rep.Exact.Precision < minExactPrecision {
		t.Errorf("exact precision %.4f is below the %.2f baseline (false positives regressed)\n%s",
			rep.Exact.Precision, minExactPrecision, rep.Summary())
	}
	if rep.Exact.Recall < minExactRecall {
		t.Errorf("exact recall %.4f is below the %.2f baseline (detections regressed)\n%s",
			rep.Exact.Recall, minExactRecall, rep.Summary())
	}
	// Overlap recall is the more forgiving measure and sits higher: it credits a
	// span the detector found but mis-sized. A gap between the two is the
	// diagnostic that localises a boundary bug.
	if rep.Overlap.Recall < minOverlapRecall {
		t.Errorf("overlap recall %.4f is below the %.2f baseline\n%s",
			rep.Overlap.Recall, minOverlapRecall, rep.Summary())
	}
	if rep.Overlap.Recall < rep.Exact.Recall {
		t.Errorf("overlap recall %.4f is BELOW exact recall %.4f, which is impossible — "+
			"overlap matching is strictly more permissive, so this indicates a scoring bug",
			rep.Overlap.Recall, rep.Exact.Recall)
	}
}

// TestGoldenDatasetHasNoDuplicateLabels checks dataset hygiene independent of
// any detector: every labelled value must be unambiguous within its own text.
// resolve() already rejects these, but asserting it here means a future case
// edit that introduces one fails with a clear dataset error rather than
// silently dropping the case from the metrics.
func TestGoldenDatasetHasNoDuplicateLabels(t *testing.T) {
	cases, err := golden.Load(datasetDir)
	if err != nil {
		t.Fatalf("loading golden dataset: %v", err)
	}
	seenIDs := make(map[string]bool, len(cases))
	for _, c := range cases {
		if seenIDs[c.ID] {
			t.Errorf("duplicate case id %q", c.ID)
		}
		seenIDs[c.ID] = true

		if len(c.Entities) == 0 {
			continue
		}
		// Labels must be listed in text order, or resolve() rejects the case.
		cursor := 0
		for i, l := range c.Entities {
			rel := indexFrom(c.Text, l.Value, cursor)
			if rel < 0 {
				t.Errorf("case %q label %d (%q) is absent or out of text order", c.ID, i, l.Value)
				break
			}
			cursor = rel + len(l.Value)
		}
	}
}

// indexFrom is strings.Index over text[cursor:], or -1 when cursor is past the
// end or the value is not present at or after it.
func indexFrom(text, value string, cursor int) int {
	if cursor > len(text) {
		return -1
	}
	rel := strings.Index(text[cursor:], value)
	if rel < 0 {
		return -1
	}
	return cursor + rel
}

// TestGoldenDatasetPathExists documents where the seed set lives, so a
// missing directory is a clear failure rather than a confusing empty-metric run.
func TestGoldenDatasetPathExists(t *testing.T) {
	if _, err := golden.Load(filepath.Clean(datasetDir)); err != nil {
		t.Fatalf("golden dataset not loadable at %s: %v", datasetDir, err)
	}
}
