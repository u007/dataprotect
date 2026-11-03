package context

import (
	"strings"
	"testing"

	"github.com/u007/dataprotect/recognizers"
)

func TestBoostConfidence(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		entity    recognizers.EntityType
		start     int
		end       int
		wantAbove float64
	}{
		{
			name:      "email keyword nearby",
			text:      "your email address is a@b.com",
			entity:    recognizers.EntityEmail,
			start:     21,
			end:       27,
			wantAbove: 0.1,
		},
		{
			name:      "no keyword",
			text:      "a@b.com",
			entity:    recognizers.EntityEmail,
			start:     0,
			end:       6,
			wantAbove: 0,
		},
		{
			name:      "phone keyword nearby",
			text:      "call me at 555-123-4567 today",
			entity:    recognizers.EntityPhone,
			start:     11,
			end:       24,
			wantAbove: 0.1,
		},
		{
			name:      "card keyword nearby",
			text:      "my visa card number is 4111111111111111",
			entity:    recognizers.EntityCreditCard,
			start:     24,
			end:       40,
			wantAbove: 0.1,
		},
		{
			name:      "unknown entity type",
			text:      "key api_key here",
			entity:    "SOMETHING_ELSE",
			start:     0,
			end:       3,
			wantAbove: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BoostConfidence(tt.text, tt.entity, tt.start, tt.end)
			if got < tt.wantAbove {
				t.Errorf("boost %f, want at least %f", got, tt.wantAbove)
			}
			if got > 0.3 {
				t.Errorf("boost %f exceeds cap 0.3", got)
			}
		})
	}
}

func TestBoostConfidenceCapsAtMax(t *testing.T) {
	// Enough keywords to exceed the cap if it were uncapped.
	text := "email e-mail mail contact reach phone tel call cell fax"
	got := BoostConfidence(text, recognizers.EntityEmail, 0, 4)
	if got != 0.3 {
		t.Errorf("boost %f, want the 0.3 cap", got)
	}
}

func TestBoostConfidenceClampsToTextBounds(t *testing.T) {
	// Start/end deliberately out of range must not panic.
	got := BoostConfidence("short", recognizers.EntityEmail, -100, 1000)
	if got < 0 || got > 0.3 {
		t.Errorf("boost %f out of range", got)
	}
}

// TestBoostConfidenceForLanguage verifies the opt-in language sets. The English
// default must not fire on a German or French label, and an unknown language
// falls back to the English words.
func TestBoostConfidenceForLanguage(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		value  string
		entity recognizers.EntityType
		lang   string // "" means the default BoostConfidence
		want   bool   // whether the boost is positive
	}{
		// "Mobil" is chosen over "Telefon" because the English set's "tel"
		// is a substring of "Telefon"; the point is a word English misses.
		{"german phone boosts", "Mobil: 555-123-4567", "555-123-4567", recognizers.EntityPhone, "de", true},
		{"english default ignores german", "Mobil: 555-123-4567", "555-123-4567", recognizers.EntityPhone, "", false},
		{"french card boosts", "carte de crédit : 4111 1111 1111 1111", "4111 1111 1111 1111", recognizers.EntityCreditCard, "fr", true},
		{"italian card boosts", "carta di credito: 4111 1111 1111 1111", "4111 1111 1111 1111", recognizers.EntityCreditCard, "it", true},
		{"dutch phone boosts", "telefoon: 555-123-4567", "555-123-4567", recognizers.EntityPhone, "nl", true},
		{"spanish card boosts", "Número de Tarjeta: 4111 1111 1111 1111", "4111 1111 1111 1111", recognizers.EntityCreditCard, "es", true},
		{"swedish card boosts", "Kortnummer: 4111 1111 1111 1111", "4111 1111 1111 1111", recognizers.EntityCreditCard, "sv", true},
		{"english default ignores swedish card", "Kortnummer: 4111 1111 1111 1111", "4111 1111 1111 1111", recognizers.EntityCreditCard, "", false},
		{"swedish phone boosts", "Samtal: 555-123-4567", "555-123-4567", recognizers.EntityPhone, "sv", true},
		{"swedish email hyphen boosts", "E-post: a@b.com", "a@b.com", recognizers.EntityEmail, "sv", true},
		{"swedish bitcoin non-ascii boosts", "Plånbok: 1BoatSLRHtKNngkdXEeobR76b53LETtpyT", "1BoatSLRHtKNngkdXEeobR76b53LETtpyT", recognizers.EntityBitcoin, "sv", true},
		{"unknown language falls back to english", "email: a@b.com", "a@b.com", recognizers.EntityEmail, "xx", true},
		{"swedish ssn ignores id inside video idag sida", "video idag sida 123-45-6789", "123-45-6789", recognizers.EntitySSN, "sv", false},
		{"swedish ip hyphen compound boosts", "IP-adress: 10.0.0.1", "10.0.0.1", recognizers.EntityIP, "sv", true},
		{"english ip ignores ip inside zip ship", "zip ship 10.0.0.1", "10.0.0.1", recognizers.EntityIP, "", false},
		{"english phone colon boundary boosts", "tel: 555-123-4567", "555-123-4567", recognizers.EntityPhone, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := strings.Index(tt.text, tt.value)
			if start < 0 {
				t.Fatalf("value %q not found", tt.value)
			}
			end := start + len(tt.value)
			var got float64
			if tt.lang == "" {
				got = BoostConfidence(tt.text, tt.entity, start, end)
			} else {
				got = BoostConfidenceForLanguage(tt.text, tt.entity, start, end, tt.lang)
			}
			if (got > 0) != tt.want {
				t.Fatalf("boost = %v, want positive=%v", got, tt.want)
			}
		})
	}
}

// TestContainsWord pins the whole-word rule: a letter on either side of an
// occurrence disqualifies it, any other rune (or the edge of s) is a boundary,
// and a later standalone occurrence still counts.
func TestContainsWord(t *testing.T) {
	tests := []struct {
		name string
		s    string
		word string
		want bool
	}{
		{"standalone", "call id now", "id", true},
		{"inside longer word", "video", "id", false},
		{"followed by letter", "idag", "id", false},
		{"preceded by letter", "sida", "id", false},
		{"hyphen boundary", "steuer-id", "id", true},
		{"colon boundary", "tel:", "tel", true},
		{"digit boundary", "1ip", "ip", true},
		{"later standalone occurrence", "video id", "id", true},
		{"edge of string", "ip", "ip", true},
		{"multi-word phrase", "tarjeta de crédito:", "tarjeta de crédito", true},
		{"multi-word phrase inside word", "xtarjeta de crédito", "tarjeta de crédito", false},
		{"non-ascii phrase boundary", "sécurité sociale.", "sécurité sociale", true},
		{"non-ascii letter before phrase", "éa", "a", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsWord(tt.s, tt.word); got != tt.want {
				t.Errorf("containsWord(%q, %q) = %v, want %v", tt.s, tt.word, got, tt.want)
			}
		})
	}
}
