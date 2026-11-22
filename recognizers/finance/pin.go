package finance

import (
	"context"
	"regexp"

	"github.com/u007/dataprotect/recognizers"
)

// pinPattern matches a PIN value after the word PIN. The value is four, six or
// eight digits, with no digit next to it.
//
// Corpus evidence (testdata/external, unlabelled spans before this rule was
// mapped): gretel account_pin values are 4 digits (27 of 47) or 6 digits (20 of
// 47); nemotron pin values are 4 digits (40 of 122), 6 digits (80 of 122) or 8
// digits (2 of 122). Every value in both corpora is one of those lengths. No
// published universal PIN shape exists, so the lengths are taken from the
// corpus. Labels seen before the value: "PIN" (nemotron 103, gretel 31),
// "account PIN".
var pinPattern = labelGate(`(?i:pin)`, `\d{4}|\d{6}|\d{8}`)

// PINRecognizer finds PIN values that follow the word PIN.
type PINRecognizer struct {
	pattern *regexp.Regexp
}

// NewPINRecognizer creates a PIN recognizer.
func NewPINRecognizer() *PINRecognizer {
	return &PINRecognizer{pattern: pinPattern}
}

// Name returns the recognizer name.
func (r *PINRecognizer) Name() string { return "finance_pin" }

// Recognize detects four-, six- or eight-digit PIN values after the word PIN.
// Only the digits are reported.
func (r *PINRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	var entities []recognizers.Entity
	for _, m := range r.pattern.FindAllStringSubmatchIndex(text, -1) {
		entities = append(entities, recognizers.Entity{
			Type:       recognizers.EntityPIN,
			Start:      m[2],
			End:        m[3],
			Text:       text[m[2]:m[3]],
			Confidence: keywordScore,
		})
	}
	return entities, nil
}
