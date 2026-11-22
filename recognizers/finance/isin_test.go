package finance

import "testing"

// ISIN examples from https://en.wikipedia.org/wiki/International_Securities_Identification_Number
// (fetched 2026-11-22): US0378331005, AU0000XVGZA3, GB0002634946. DEK4M7P2Q9X4
// is synthetic, with its check digit computed by the Luhn rule.
func TestValidISIN(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"US0378331005", true},
		{"AU0000XVGZA3", true},
		{"GB0002634946", true},
		{"DEK4M7P2Q9X4", true},
		{"US0378331006", false},
		{"AU0000XVGZA4", false},
		{"US037833100", false},
		{"US03783310055", false},
		{"us0378331005", false},
		{"US037833100X", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := ValidISIN(tt.s); got != tt.want {
			t.Errorf("ValidISIN(%q) = %v, want %v", tt.s, got, tt.want)
		}
	}
}

func TestISINRecognizeNeedsKeyword(t *testing.T) {
	r := NewISINRecognizer()
	recognized := []struct {
		text string
		want string
	}{
		{"ISIN US0378331005", "US0378331005"},
		{"isin: AU0000XVGZA3", "AU0000XVGZA3"},
		{"The ISIN is GB0002634946 for this bond.", "GB0002634946"},
		{"ISIN code DEK4M7P2Q9X4", "DEK4M7P2Q9X4"},
	}
	for _, tt := range recognized {
		checkValues(t, tt.text, recognize(t, r, tt.text), []string{tt.want})
	}
	// Valid check digits, no ISIN label: nothing is reported.
	checkValues(t, "security US0378331005 traded", recognize(t, r, "security US0378331005 traded"), nil)
	// A label with a bad check digit: nothing is reported.
	checkValues(t, "ISIN US0378331006", recognize(t, r, "ISIN US0378331006"), nil)
	// A label before an 11-character value: nothing is reported.
	checkValues(t, "ISIN US03783310", recognize(t, r, "ISIN US03783310"), nil)
}
