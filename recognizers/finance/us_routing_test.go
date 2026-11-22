package finance

import "testing"

// Checksums are worked by hand in the comment on validRoutingCases. 123456780
// gives 3(1+4+7)+7(2+5+8)+(3+6+0) = 36+105+9 = 150, which is 0 mod 10, and its
// prefix 12 is in 00-12.
func TestUSRoutingValidate(t *testing.T) {
	v := NewUSRoutingValidator()
	valid := []string{
		"123456780", // checksum 150, prefix 12
		"021000021", // checksum 30, prefix 02
		"610050000", // checksum 30, prefix 61 (in 61-72)
		"800080000", // checksum 80, prefix 80
	}
	invalid := []string{
		"123456789", // checksum 159
		"130000033", // checksum 30 but prefix 13 is unassigned
		"000000001", // checksum 3, not 0 mod 10
		"12345678",  // eight digits
		"1234567890",
		"12345678A",
		"",
	}
	for _, s := range valid {
		if !v.Validate(s) {
			t.Errorf("Validate(%q) = false, want true", s)
		}
	}
	for _, s := range invalid {
		if v.Validate(s) {
			t.Errorf("Validate(%q) = true, want false", s)
		}
	}
}
