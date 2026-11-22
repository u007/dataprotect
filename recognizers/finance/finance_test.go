package finance

import (
	"context"
	"testing"

	"github.com/u007/dataprotect/recognizers"
)

// recognize runs r over text and returns the reported values, so each table
// row can state the exact span it expects (the value, never the label).
func recognize(t *testing.T, r recognizers.Recognizer, text string) []recognizers.Entity {
	t.Helper()
	ents, err := r.Recognize(context.Background(), text)
	if err != nil {
		t.Fatalf("Recognize(%q): %v", text, err)
	}
	for _, e := range ents {
		if text[e.Start:e.End] != e.Text {
			t.Errorf("Recognize(%q): span [%d:%d] is %q but Text is %q", text, e.Start, e.End, text[e.Start:e.End], e.Text)
		}
		if e.Confidence != keywordScore {
			t.Errorf("Recognize(%q): confidence %v, want %v", text, e.Confidence, keywordScore)
		}
	}
	return ents
}

// checkValues asserts the recognizer reports exactly want, in order.
func checkValues(t *testing.T, text string, ents []recognizers.Entity, want []string) {
	t.Helper()
	var got []string
	for _, e := range ents {
		got = append(got, e.Text)
	}
	if len(got) != len(want) {
		t.Fatalf("%q: got %q, want %q", text, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%q: got %q, want %q", text, got, want)
		}
	}
}

func TestFinanceRulesKeywordGateAndSpan(t *testing.T) {
	cases := []struct {
		name string
		r    recognizers.Recognizer
		text string
		want []string
	}{
		{"swift label", NewSWIFTBICRecognizer(), "Send to SWIFT code DEUTDEFF today.", []string{"DEUTDEFF"}},
		{"swift/bic label", NewSWIFTBICRecognizer(), "SWIFT/BIC: CHASUS33XXX is set.", []string{"CHASUS33XXX"}},
		{"routing label", NewUSRoutingRecognizer(), "Bank routing number: 123456780 here.", []string{"123456780"}},
		{"routing transit label", NewUSRoutingRecognizer(), "routing transit number is 123456780", []string{"123456780"}},
		{"RTN label", NewUSRoutingRecognizer(), "RTN 123456780", []string{"123456780"}},
		{"ABA label", NewUSRoutingRecognizer(), "ABA 123456780", []string{"123456780"}},
		{"cvv label", NewCardCVVRecognizer(), "CVV 123 was entered.", []string{"123"}},
		{"cvc2 label", NewCardCVVRecognizer(), "CVC2: 4821 for the card", []string{"4821"}},
		{"security code phrase", NewCardCVVRecognizer(), "The card security code is 582 for this order.", []string{"582"}},
		{"account number label", NewBankAccountRecognizer(), "wire to account number 0012345678 now", []string{"0012345678"}},
		{"bban label", NewBankAccountRecognizer(), "BBAN: SBPJ14329354627679 ok", []string{"SBPJ14329354627679"}},
		{"pin label", NewPINRecognizer(), "Enter your PIN 4821 at the terminal.", []string{"4821"}},
		{"account pin six", NewPINRecognizer(), "The account PIN is 482913 for phone banking.", []string{"482913"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ents := recognize(t, tc.r, tc.text)
			checkValues(t, tc.text, ents, tc.want)
		})
	}
}

// Negatives: a value with no label, a label with a value of the wrong shape,
// and a label glued to a longer word, must all be silent.
func TestFinanceRulesNegatives(t *testing.T) {
	cases := []struct {
		name string
		r    recognizers.Recognizer
		text string
	}{
		{"swift no label", NewSWIFTBICRecognizer(), "Our head office is DEUTDEFF in Frankfurt."},
		{"swift lowercase value", NewSWIFTBICRecognizer(), "The swift code: deutdeff was typed in lowercase."},
		{"swift wrong length", NewSWIFTBICRecognizer(), "SWIFT code DEUTDEFF1234 was typed."},
		{"routing bad checksum", NewUSRoutingRecognizer(), "Routing number: 123456789 was printed."},
		{"routing bad prefix", NewUSRoutingRecognizer(), "Routing number 130000033 is on the cheque."},
		{"routing no label", NewUSRoutingRecognizer(), "Reference 123456780 was logged at noon."},
		{"cvv five digits", NewCardCVVRecognizer(), "CVV 12345 was rejected by the terminal."},
		{"cvv no label", NewCardCVVRecognizer(), "Order 582 shipped on Monday."},
		{"account no label", NewBankAccountRecognizer(), "Ticket 882134651 was logged at noon."},
		{"account too short", NewBankAccountRecognizer(), "Account number: 1234567 was read out twice."},
		{"pin five digits", NewPINRecognizer(), "PIN 48291 was not accepted."},
		{"pin inside a word", NewPINRecognizer(), "pinned 4821 on the board"},
		{"pin no keyword", NewPINRecognizer(), "The shipment weighed 482913 kilograms."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ents := recognize(t, tc.r, tc.text)
			if len(ents) != 0 {
				t.Errorf("%q: want no detection, got %+v", tc.text, ents)
			}
		})
	}
}

// A label that sits after a long sentence must not reach across to a number:
// the filler between label and value is a closed list of at most three words.
func TestLabelDoesNotReachAcrossSentence(t *testing.T) {
	text := "The PIN was changed last week after the audit team asked. Then 4821 was set."
	if ents := recognize(t, NewPINRecognizer(), text); len(ents) != 0 {
		t.Errorf("PIN reached across a sentence: %+v", ents)
	}
}
