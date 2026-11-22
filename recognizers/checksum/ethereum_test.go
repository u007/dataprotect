package checksum

import "testing"

// Test vectors are the EIP-55 examples (https://eips.ethereum.org/EIPS/eip-55,
// fetched 2026-11-22). TestValidEthereumAddressFlippedCase covers the negatives.
func TestValidEthereumAddress(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want bool
	}{
		{"all-caps-1", "0x52908400098527886E0F7030069857D2E4169EE7", true},
		{"all-caps-2", "0x8617E340B3D01FA5F11F306F4090FD50E238070D", true},
		{"all-lower-1", "0xde709f2102306220921060314715629080e2fb77", true},
		{"all-lower-2", "0x27b1fdb04752bbc536007a920d24acb045561c26", true},
		{"mixed-1", "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed", true},
		{"mixed-2", "0xfB6916095ca1df60bB79Ce92cE3Ea74c37c5d359", true},
		{"mixed-3", "0xdbF03B407c01E7cD3CBea99509d93f8DDDC8C6FB", true},
		{"mixed-4", "0xD1220A0cf47c7B9Be7A2E6BA89F429762e7b9aDb", true},
		{"no-prefix", "5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed", false},
		{"upper-prefix", "0X5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed", false},
		{"short", "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAe", false},
		{"long", "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAedd", false},
		{"non-hex", "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAeg", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidEthereumAddress(tt.addr); got != tt.want {
				t.Errorf("ValidEthereumAddress(%q) = %v, want %v", tt.addr, got, tt.want)
			}
		})
	}
}

// Every letter of a mixed-case vector is fixed by the checksum, so flipping the
// case of any single letter must fail. Each letter is tried in turn.
func TestValidEthereumAddressFlippedCase(t *testing.T) {
	for _, addr := range []string{
		"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed",
		"0xfB6916095ca1df60bB79Ce92cE3Ea74c37c5d359",
		"0xdbF03B407c01E7cD3CBea99509d93f8DDDC8C6FB",
		"0xD1220A0cf47c7B9Be7A2E6BA89F429762e7b9aDb",
	} {
		if !ValidEthereumAddress(addr) {
			t.Fatalf("vector %q should be valid before flipping", addr)
		}
		flips := 0
		for i := 2; i < len(addr); i++ {
			c := addr[i]
			var f byte
			switch {
			case c >= 'a' && c <= 'f':
				f = c - 'a' + 'A'
			case c >= 'A' && c <= 'F':
				f = c - 'A' + 'a'
			default:
				continue
			}
			flipped := addr[:i] + string(f) + addr[i+1:]
			flips++
			if ValidEthereumAddress(flipped) {
				t.Errorf("%q with index %d flipped to %q is accepted", addr, i, string(f))
			}
		}
		if flips == 0 {
			t.Errorf("%q has no letters to flip", addr)
		}
	}
}
