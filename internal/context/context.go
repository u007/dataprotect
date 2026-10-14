// Package context provides context-aware confidence boosting for PII detection.
package context

import (
	"strings"

	"github.com/u007/dataprotect/recognizers"
)

// BoostContext contains words that boost confidence when found near an entity.
var BoostContext = map[recognizers.EntityType][]string{
	recognizers.EntityEmail: {
		"email", "e-mail", "mail", "contact", "reach",
	},
	recognizers.EntityPhone: {
		"phone", "call", "tel", "telephone", "mobile", "cell", "fax",
	},
	recognizers.EntityCreditCard: {
		"card", "credit", "debit", "visa", "mastercard", "amex", "cc",
	},
	recognizers.EntitySSN: {
		"ssn", "social", "security", "tax", "id",
	},
	recognizers.EntityIP: {
		"ip", "address", "host", "server",
	},
	recognizers.EntityIBAN: {
		"iban", "bank", "account", "routing",
	},
	recognizers.EntityBitcoin: {
		"bitcoin", "btc", "wallet", "address",
	},
}

// BoostConfidence adjusts entity confidence based on surrounding context.
func BoostConfidence(text string, entity recognizers.EntityType, start, end int) float64 {
	contextWords := BoostContext[entity]
	if len(contextWords) == 0 {
		return 0.0
	}

	// Get surrounding text (50 chars before and after)
	contextStart := start - 50
	if contextStart < 0 {
		contextStart = 0
	}
	contextEnd := end + 50
	if contextEnd > len(text) {
		contextEnd = len(text)
	}
	surrounding := strings.ToLower(text[contextStart:contextEnd])

	boost := 0.0
	for _, word := range contextWords {
		if strings.Contains(surrounding, word) {
			boost += 0.1
		}
	}
	if boost > 0.3 {
		boost = 0.3
	}
	return boost
}
