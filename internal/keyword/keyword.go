// Package keyword holds the label gate shared by the keyword-gated recognizers
// (recognizers/finance and recognizers/identity). A keyword-gated rule reports a
// value only when a label for it sits directly before it, and the reported span
// is the value alone, never the label.
package keyword

import "regexp"

// Score is the confidence for every keyword-gated rule. It matches the other
// keyword-gated national rules, so a keyword-backed value ranks alongside them
// and beats a bare shape match of a lower score.
const Score = 0.85

// filler is up to three short connecting words between a label and its value,
// such as "routing number is" or "CVV code for this card is". The word list is
// closed, so a long sentence cannot carry a label across to an unrelated number.
const filler = `(?:(?i:is|of|for|the|this|your|my|card|code|number|no|value|at|to|as|on|in)\b[^A-Za-z0-9]{1,8}){0,3}`

// LabelGate builds a pattern that matches a value only when keyword comes
// first, separated by one to eight non-alphanumeric characters with up to three
// filler words in between. The value is the only capture group, so the span
// reported by the recognizer is the value alone. The value must end on a word
// boundary, so a longer token is never cut short. The keyword part is
// case-insensitive when written with (?i:...); the value is matched exactly as
// written in value.
func LabelGate(keyword, value string) *regexp.Regexp {
	return regexp.MustCompile(`\b(?:` + keyword + `)[^A-Za-z0-9]{1,8}` + filler + `(` + value + `)\b`)
}
