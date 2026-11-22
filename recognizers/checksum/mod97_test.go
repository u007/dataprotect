package checksum

import "testing"

// Mod97Valid is the ISO 7064 MOD 97-10 check with no rearrangement. The LEI
// example 54930084UKLVMY22DS16 is from https://en.wikipedia.org/wiki/Legal_Entity_Identifier
// (fetched 2026-11-22). The IBAN cases exercise the same helper after IBAN's
// rearrangement; the GB82 example is the standard one from the IBAN specification.
func TestMod97Valid(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"54930084UKLVMY22DS16", true},
		{"54930084UKLVMY22DS17", false},
		{"54930084UKLVMY22DS15", false},
		{"54930084ukLVMY22DS16", false},
		{"54930084UKLVMY22DS16 ", false},
		{"", false},
		{"WEST12345698765432GB82", true}, // GB82WEST12345698765432, rearranged
		{"GB82WEST12345698765432", false},
	}
	for _, tt := range tests {
		if got := Mod97Valid(tt.s); got != tt.want {
			t.Errorf("Mod97Valid(%q) = %v, want %v", tt.s, got, tt.want)
		}
	}
}
