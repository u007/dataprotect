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
		{"unknown language falls back to english", "email: a@b.com", "a@b.com", recognizers.EntityEmail, "xx", true},
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
