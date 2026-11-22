package finance

import "testing"

// A CVV is three or four digits with no digit next to it. A longer run is not
// cut down to four, and a card number around the value is handled by the
// analyzer (CREDIT_CARD wins on overlap), not here.
func TestCardCVVDigitBoundary(t *testing.T) {
	r := NewCardCVVRecognizer()
	ents := recognize(t, r, "CVV 1234 and CVV 12345")
	checkValues(t, "CVV 1234 and CVV 12345", ents, []string{"1234"})
}
