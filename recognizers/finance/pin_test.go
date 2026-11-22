package finance

import "testing"

// The corpus PIN lengths are four, six and eight digits. A five-digit value
// after PIN is not reported.
func TestPINLengths(t *testing.T) {
	r := NewPINRecognizer()
	for _, v := range []string{"4821", "482913", "48291384"} {
		checkValues(t, v, recognize(t, r, "PIN "+v+" ok"), []string{v})
	}
	for _, v := range []string{"48291", "4829138", "482913845"} {
		if ents := recognize(t, r, "PIN "+v+" ok"); len(ents) != 0 {
			t.Errorf("PIN %q: want no detection, got %+v", v, ents)
		}
	}
}
