package analyzer

import (
	"context"
	"testing"

	"github.com/u007/dataprotect/recognizers"
)

// overlapPair runs two stub recognizers over the same text, one span each, in
// both orders, and returns the single surviving entity type. Both orders are
// checked so the result cannot depend on which recognizer ran first.
func overlapPair(t *testing.T, a, b recognizers.Entity) recognizers.EntityType {
	t.Helper()
	var winners []recognizers.EntityType
	for _, order := range [][2]recognizers.Entity{{a, b}, {b, a}} {
		an := New()
		an.AddRecognizer(&stubRecognizer{name: "first", entities: []recognizers.Entity{order[0]}})
		an.AddRecognizer(&stubRecognizer{name: "second", entities: []recognizers.Entity{order[1]}})
		text := "0123456789012345678901234567890123456789"
		entities, err := an.Analyze(context.Background(), text)
		if err != nil {
			t.Fatalf("Analyze: %v", err)
		}
		if len(entities) != 1 {
			t.Fatalf("%s vs %s: want one survivor, got %+v", a.Type, b.Type, entities)
		}
		winners = append(winners, entities[0].Type)
	}
	if winners[0] != winners[1] {
		t.Fatalf("%s vs %s: order-dependent winner %v then %v", a.Type, b.Type, winners[0], winners[1])
	}
	return winners[0]
}

func span(t recognizers.EntityType, start, end int, score float64) recognizers.Entity {
	return recognizers.Entity{Type: t, Start: start, End: end, Text: "x", Confidence: score}
}

// A caller's CUSTOM_TERM is asserted at 1.00 and wins every overlap, including
// the pairs the type-rules would otherwise settle for a card or a keyword type.
func TestCustomTermWinsEveryOverlap(t *testing.T) {
	for _, other := range []recognizers.EntityType{
		recognizers.EntityCreditCard, recognizers.EntityPhone, recognizers.EntitySecret,
		recognizers.EntityURL, recognizers.EntityBankAccount, recognizers.EntityCardCVV,
		recognizers.EntityPIN, recognizers.EntityCaseNumber, recognizers.EntityMoney,
		recognizers.EntityPerson,
	} {
		term := span(recognizers.EntityCustomTerm, 0, 16, 1.00)
		// The other span scores above the term's score is impossible, so tie at
		// the highest score a real recognizer uses.
		got := overlapPair(t, term, span(other, 4, 20, 0.99))
		if got != recognizers.EntityCustomTerm {
			t.Errorf("CUSTOM_TERM vs %s: winner %s, want CUSTOM_TERM", other, got)
		}
	}
}

// MONEY is the least specific type: it loses every overlap, even when its own
// score is higher.
func TestMoneyLosesEveryOverlap(t *testing.T) {
	for _, other := range []recognizers.EntityType{
		recognizers.EntitySecret, recognizers.EntityURL, recognizers.EntityPhone,
		recognizers.EntityCreditCard, recognizers.EntityIBAN, recognizers.EntityBankAccount,
		recognizers.EntityCardCVV, recognizers.EntityCaseNumber, recognizers.EntityPerson,
		recognizers.EntityIP, recognizers.EntitySSN,
	} {
		money := span(recognizers.EntityMoney, 0, 16, 1.00)
		got := overlapPair(t, money, span(other, 4, 20, 0.85))
		if got != other {
			t.Errorf("MONEY vs %s: winner %s, want %s", other, got, other)
		}
	}
}

// A "$" amount inside a secret span is the case the rule exists for.
func TestMoneyYieldsToSecretOnSameSpanRegardlessOfScore(t *testing.T) {
	got := overlapPair(t, span(recognizers.EntityMoney, 0, 10, 0.85), span(recognizers.EntitySecret, 0, 10, 0.60))
	if got != recognizers.EntitySecret {
		t.Errorf("winner %s, want SECRET", got)
	}
}

// CASE_NUMBER is keyword-gated, so it beats PHONE at every score and loses to a
// CREDIT_CARD.
func TestCaseNumberBeatsPhoneAndLosesToCard(t *testing.T) {
	if got := overlapPair(t, span(recognizers.EntityCaseNumber, 0, 16, 0.85), span(recognizers.EntityPhone, 4, 14, 0.98)); got != recognizers.EntityCaseNumber {
		t.Errorf("CASE_NUMBER vs PHONE: winner %s, want CASE_NUMBER", got)
	}
	if got := overlapPair(t, span(recognizers.EntityCaseNumber, 0, 16, 0.90), span(recognizers.EntityCreditCard, 4, 20, 0.99)); got != recognizers.EntityCreditCard {
		t.Errorf("CASE_NUMBER vs CREDIT_CARD: winner %s, want CREDIT_CARD", got)
	}
}
