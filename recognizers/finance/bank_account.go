package finance

import (
	"context"
	"regexp"

	"github.com/u007/dataprotect/recognizers"
)

// bankAccountValue is the observed account-number shape: up to four leading
// letters (a BBAN prefix or a branch letter), then a digit-led run of digits,
// letters and hyphens that ends on a digit. The total length is 8 to 34
// characters.
//
// No universal account-number shape is published: account formats differ by
// bank and country. The shape below is taken from labelled corpus values, so it
// is a keyword-gated observation, not a validity rule.
//
// Corpus evidence (testdata/external, unlabelled spans before this rule was
// mapped):
//   - nemotron account_number (337 values): lengths 8 to 32; 329 of 337 match
//     this shape; 237 are digits with optional hyphens only; the rest have one
//     or two leading letters (for example "C387265419", "AA99-9999-9999-9999").
//   - gretel bban (53 values): 48 are four letters then fourteen digits (18
//     characters), 3 are twenty digits, one is twenty-three characters; 53 of 53
//     match this shape.
//
// Labels seen before the value: "account number" (nemotron 294 of the 337
// values within 30 characters), "bank account" (gretel 5, nemotron 2), "BBAN"
// (gretel 14), "account number" (gretel 5).
var bankAccountValue = `[A-Z]{0,4}\d[A-Z\d-]{6,28}\d`

var bankAccountPattern = labelGate(`(?i:account\s+(?:number|no)|acct\.?\s+(?:number|no)|bank\s+account|bban)`, bankAccountValue)

// BankAccountRecognizer finds bank account numbers that follow an account label.
type BankAccountRecognizer struct {
	pattern *regexp.Regexp
}

// NewBankAccountRecognizer creates a bank account number recognizer.
func NewBankAccountRecognizer() *BankAccountRecognizer {
	return &BankAccountRecognizer{pattern: bankAccountPattern}
}

// Name returns the recognizer name.
func (r *BankAccountRecognizer) Name() string { return "finance_bank_account" }

// Recognize detects bank account numbers that follow an account label. Only the
// number is reported.
func (r *BankAccountRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	var entities []recognizers.Entity
	for _, m := range r.pattern.FindAllStringSubmatchIndex(text, -1) {
		entities = append(entities, recognizers.Entity{
			Type:       recognizers.EntityBankAccount,
			Start:      m[2],
			End:        m[3],
			Text:       text[m[2]:m[3]],
			Confidence: keywordScore,
		})
	}
	return entities, nil
}
