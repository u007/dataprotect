package finance

import (
	"context"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/u007/dataprotect/recognizers"
)

// MONEY is an amount with a currency symbol or ISO 4217 code: "$1,250.00",
// "S$ 40", "RM 900", "EUR 3.5m", "12,000 USD". Sources (fetched
// 2026-11-22; see SOURCES.md for the unverified items):
//
//   - https://en.wikipedia.org/wiki/ISO_4217 : "the first two letters of the alpha
//     code are the two letters of the ISO 3166-1 alpha-2 country code". Active
//     codes used here: USD, EUR, GBP, JPY, CNY, SGD, MYR, INR, THB, PHP, IDR,
//     AUD, CAD, CHF, HKD, SEK. Only AUD was confirmed against the verbatim table
//     text; the others come from an extracted summary of the same page.
//   - https://en.wikipedia.org/wiki/Malaysian_ringgit : "symbol: RM; currency
//     code: MYR".
//   - https://en.wikipedia.org/wiki/Singapore_dollar : "normally abbreviated with
//     the dollar sign $, or S$".
//
// The symbols EUR-sign, pound-sign, yen-sign and US$ are common usage and were
// not fetched from a page; they are listed as unfetched in SOURCES.md.
//
// A bare number is never MONEY. An amount needs a symbol or code next to it.
// Symbols and codes are matched exactly as written (codes uppercase, whole
// word). RM must be followed by a digit, so the word "RM" in prose is not
// matched. A symbol is not matched when a letter, digit, "$" or "_" comes right
// before it, so the "$10" inside a bcrypt hash ("$2a$10$...") is not money.
// MONEY scores 0.85 and loses every overlap to any other type (winsOverlap).

const moneyScore = 0.85

// moneyCodes are the ISO 4217 codes MONEY recognizes, uppercase, whole word.
var moneyCodes = []string{
	"USD", "EUR", "GBP", "JPY", "CNY", "SGD", "MYR", "INR", "THB", "PHP",
	"IDR", "AUD", "CAD", "CHF", "HKD", "SEK",
}

// moneySymbols are the prefix symbols. "US$" and "S$" come before "$" so the
// longest one wins in the alternation.
var moneySymbols = []string{`US\$`, `S\$`, `\$`, `RM`, `€`, `£`, `¥`}

// moneyAmount is digits with optional thousands separators, then either a dot
// decimal part or a one- or two-digit decimal comma ("4,99"; a comma followed by
// three digits is a thousands group), followed by an optional scale word. The
// amount always ends on a word boundary, so "$2a" is not an amount. Scale words are matched
// case-insensitively, so "5M" and "5 million" both count.
const moneyAmount = `(?:\d{1,3}(?:,\d{3})+(?:\.\d+)?|\d+,\d{1,2}\b|\d+(?:\.\d+)?)(?:[ \t]?(?i:thousand|million|billion|bn|juta|k|m)\b|\b)`

var (
	// moneyPrefix is a symbol or code followed by an optional space and the amount.
	moneyPrefix = regexp.MustCompile(
		`(?:(?:` + joinAlt(moneySymbols) + `)[ \t]?|\b(?:` + joinAlt(moneyCodes) + `)[ \t]?)` +
			`(?:` + moneyAmount + `)`)
	// moneySuffix is an amount followed by a space and a code.
	moneySuffix = regexp.MustCompile(
		`\b(?:` + moneyAmount + `)[ \t]?\b(?:` + joinAlt(moneyCodes) + `)\b`)
)

func joinAlt(words []string) string { return strings.Join(words, "|") }

// MoneyRecognizer finds amounts written with a currency symbol or ISO code.
type MoneyRecognizer struct{}

// NewMoneyRecognizer creates a MONEY recognizer.
func NewMoneyRecognizer() *MoneyRecognizer { return &MoneyRecognizer{} }

// Name returns the recognizer name.
func (r *MoneyRecognizer) Name() string { return "finance_money" }

// Recognize reports each symbol-or-code amount and each amount-then-code span.
// The span covers the symbol or code and the amount together.
func (r *MoneyRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	type span struct{ start, end int }
	var spans []span
	for _, m := range moneyPrefix.FindAllStringIndex(text, -1) {
		if !moneyPrefixStartsCleanly(text, m[0]) {
			continue
		}
		spans = append(spans, span{m[0], m[1]})
	}
	for _, m := range moneySuffix.FindAllStringIndex(text, -1) {
		spans = append(spans, span{m[0], m[1]})
	}
	// Drop a span that is only part of a larger number, such as "1" in "1,2500".
	kept := spans[:0]
	for _, sp := range spans {
		if moneyInsideNumber(text, sp.start, sp.end) {
			continue
		}
		kept = append(kept, sp)
	}
	spans = kept
	var entities []recognizers.Entity
	last := -1
	for _, s := range spans {
		if s.start < last {
			continue
		}
		last = s.end
		entities = append(entities, recognizers.Entity{
			Type:       recognizers.EntityMoney,
			Start:      s.start,
			End:        s.end,
			Text:       text[s.start:s.end],
			Confidence: moneyScore,
		})
	}
	return entities, nil
}

// moneyInsideNumber reports whether the amount at [start, end) is a fragment of
// a longer number: a digit or a "," or "." group continues it on either side.
func moneyInsideNumber(text string, start, end int) bool {
	if start > 0 {
		before, _ := utf8.DecodeLastRuneInString(text[:start])
		if unicode.IsDigit(before) {
			return true
		}
		if before == ',' || before == '.' {
			if start > 1 {
				bb, _ := utf8.DecodeLastRuneInString(text[:start-1])
				if unicode.IsDigit(bb) {
					return true
				}
			}
		}
	}
	if end < len(text) {
		after, _ := utf8.DecodeRuneInString(text[end:])
		if unicode.IsDigit(after) {
			return true
		}
		if after == ',' || after == '.' {
			if end+1 < len(text) {
				af, _ := utf8.DecodeRuneInString(text[end+1:])
				if unicode.IsDigit(af) {
					return true
				}
			}
		}
	}
	return false
}

// moneyPrefixStartsCleanly reports whether a symbol or code at start is not glued
// to a preceding letter, digit, "$" or underscore. A code is already whole-word
// by its \b; this check is what keeps "$" out of a hash and "ARM 5" out of RM.
func moneyPrefixStartsCleanly(text string, start int) bool {
	if start == 0 {
		return true
	}
	before, _ := utf8.DecodeLastRuneInString(text[:start])
	return !(unicode.IsLetter(before) || unicode.IsDigit(before) || before == '$' || before == '_')
}
