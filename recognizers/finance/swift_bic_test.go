package finance

import "testing"

// Two lengths are valid (8 and 11). A ten-character value and a twelve-character
// value after a label must not be reported, even though their first eleven
// characters look like a BIC.
func TestSWIFTBICLengths(t *testing.T) {
	r := NewSWIFTBICRecognizer()
	ok := []string{"DEUTDEFF", "CHASUS33XXX", "BKLMUS7RGT5", "DEUTDEFF500"}
	for _, v := range ok {
		ents := recognize(t, r, "SWIFT code "+v+" applies")
		checkValues(t, v, ents, []string{v})
	}
	for _, v := range []string{"DEUTDEF", "DEUTDEFF5", "DEUTDEFF1234"} {
		if ents := recognize(t, r, "SWIFT code "+v+" applies"); len(ents) != 0 {
			t.Errorf("length of %q: want no detection, got %+v", v, ents)
		}
	}
}

// The country code is any two uppercase letters, not an IBAN country: US, SG
// and MY are not IBAN countries and must still be accepted.
func TestSWIFTBICNonIBANCountries(t *testing.T) {
	r := NewSWIFTBICRecognizer()
	for _, v := range []string{"CHASUS33", "DBSSSGSG", "MBBEMYKL"} {
		ents := recognize(t, r, "BIC: "+v)
		checkValues(t, v, ents, []string{v})
	}
}
