package analyzer

import (
	"context"
	"strings"
	"testing"

	"github.com/u007/dataprotect/recognizers"
)

// The keyword-gated national and finance types added in batch 3a (ISIN, IN_PAN,
// SG_UEN, MY_SSM, MY_TIN) are in isKeywordGated, so each beats PHONE at any score
// and loses to CREDIT_CARD at any score. Each pair is tried in both recognizer
// orders, so a tie on confidence cannot hide the rule.
func TestNewKeywordGatedTypesOverlapRules(t *testing.T) {
	text := strings.Repeat("9", 12)
	for _, kw := range []recognizers.EntityType{
		recognizers.EntityISIN, recognizers.EntityINPAN, recognizers.EntitySGUEN,
		recognizers.EntityMYSSM, recognizers.EntityMYTIN,
	} {
		for _, swap := range []bool{false, true} {
			// Keyword-gated 0.85 against PHONE 0.98: the keyword wins at a lower score.
			kwEnt := recognizers.Entity{Type: kw, Start: 0, End: 12, Text: text, Confidence: 0.85}
			phone := recognizers.Entity{Type: recognizers.EntityPhone, Start: 0, End: 12, Text: text, Confidence: 0.98}
			got := resolvePair(t, text, swap, kwEnt, phone)
			if len(got) != 1 || got[0].Type != kw {
				t.Errorf("%s vs PHONE (swap=%v): want %s alone, got %+v", kw, swap, kw, got)
			}

			// CREDIT_CARD 0.80 against the keyword 0.85: the card wins at a lower score.
			card := recognizers.Entity{Type: recognizers.EntityCreditCard, Start: 0, End: 12, Text: text, Confidence: 0.80}
			got = resolvePair(t, text, swap, kwEnt, card)
			if len(got) != 1 || got[0].Type != recognizers.EntityCreditCard {
				t.Errorf("%s vs CREDIT_CARD (swap=%v): want CREDIT_CARD alone, got %+v", kw, swap, got)
			}
		}
	}
}

// IN_PAN yields to IN_GSTIN on the same span at every score. IN_PAN is given 0.95
// here, above IN_GSTIN's 0.90, so only the explicit rule keeps IN_GSTIN.
func TestINPANYieldsToINGSTINOnSameSpan(t *testing.T) {
	text := "27AAAPZ1234C1Z0"
	for _, swap := range []bool{false, true} {
		pan := recognizers.Entity{Type: recognizers.EntityINPAN, Start: 2, End: 12, Text: "AAAPZ1234C", Confidence: 0.95}
		gstin := recognizers.Entity{Type: recognizers.EntityINGSTIN, Start: 0, End: 15, Text: text, Confidence: 0.90}
		got := resolvePair(t, text, swap, pan, gstin)
		if len(got) != 1 || got[0].Type != recognizers.EntityINGSTIN {
			t.Errorf("IN_PAN/IN_GSTIN (swap=%v): want IN_GSTIN alone, got %+v", swap, got)
		}
	}
}

// MY_TIN takes the TIN label that TAX_ID also takes, so TAX_ID yields to it on
// the same span. Both score 0.85, so only the explicit rule decides.
func TestTaxIDYieldsToMYTINOnSameSpan(t *testing.T) {
	text := "E12345678901"
	for _, swap := range []bool{false, true} {
		taxID := recognizers.Entity{Type: recognizers.EntityTaxID, Start: 0, End: 12, Text: text, Confidence: 0.85}
		myTIN := recognizers.Entity{Type: recognizers.EntityMYTIN, Start: 0, End: 12, Text: text, Confidence: 0.85}
		got := resolvePair(t, text, swap, taxID, myTIN)
		if len(got) != 1 || got[0].Type != recognizers.EntityMYTIN {
			t.Errorf("TAX_ID/MY_TIN (swap=%v): want MY_TIN alone, got %+v", swap, got)
		}
	}
}

// MY_IC yields to MY_SSM on the same span at every score. A labelled 12-digit
// run is an SSM number even when a judge has promoted its bare MyKad candidate
// to the same score. Both orders and both scores are checked.
func TestMYICYieldsToMYSSMOnSameSpan(t *testing.T) {
	text := "202001234567"
	for _, icScore := range []float64{0.85, 0.99} {
		for _, swap := range []bool{false, true} {
			ic := recognizers.Entity{Type: recognizers.EntityMYIC, Start: 0, End: 12, Text: text, Confidence: icScore}
			ssm := recognizers.Entity{Type: recognizers.EntityMYSSM, Start: 0, End: 12, Text: text, Confidence: 0.85}
			got := resolvePair(t, text, swap, ic, ssm)
			if len(got) != 1 || got[0].Type != recognizers.EntityMYSSM {
				t.Errorf("MY_IC %.2f/MY_SSM (swap=%v): want MY_SSM alone, got %+v", icScore, swap, got)
			}
		}
	}
}

// resolvePair runs two fixed entities through an analyzer, the second one first
// when swap is set, and returns what survives overlap resolution.
func resolvePair(t *testing.T, text string, swap bool, first, second recognizers.Entity) []recognizers.Entity {
	t.Helper()
	if swap {
		first, second = second, first
	}
	a := New()
	a.AddRecognizer(&stubRecognizer{name: "first", entities: []recognizers.Entity{first}})
	a.AddRecognizer(&stubRecognizer{name: "second", entities: []recognizers.Entity{second}})
	got, err := a.Analyze(context.Background(), text)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	return got
}
