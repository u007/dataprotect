package context

import (
	"testing"

	"dataprotect/recognizers"
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
