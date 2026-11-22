package finance

import "testing"

func TestBankAccountShape(t *testing.T) {
	r := NewBankAccountRecognizer()
	cases := map[string]string{
		"account number 093-284359714 is open":   "093-284359714",
		"account no 9876543210 is open":          "9876543210",
		"BBAN: KALL90112673253892 for the loan":  "KALL90112673253892",
		"bank account 12345678901234567890 here": "12345678901234567890",
	}
	for text, want := range cases {
		checkValues(t, text, recognize(t, r, text), []string{want})
	}
}
