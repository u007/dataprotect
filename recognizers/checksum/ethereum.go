package checksum

import "strings"

// ValidEthereumAddress reports whether addr is an Ethereum address that passes
// the EIP-55 mixed-case checksum. Source: https://eips.ethereum.org/EIPS/eip-55
// (fetched 2026-11-22).
//
// The shape is "0x" followed by 40 hexadecimal characters. The "0x" prefix is
// lowercase only. An address whose hex part is all lowercase, all uppercase, or
// has no letters carries no checksum and is accepted by shape. A mixed-case
// address must match the checksum: the Keccak-256 hash of the lowercase hex
// string, read one nibble per character, sets each letter's case. A letter whose
// hash nibble is 8 or more is uppercase, and any other letter is lowercase.
// Digits have no case and are not checked.
func ValidEthereumAddress(addr string) bool {
	if len(addr) != 42 || addr[0] != '0' || addr[1] != 'x' {
		return false
	}
	hex := addr[2:]
	lower, upper := false, false
	for i := 0; i < len(hex); i++ {
		c := hex[i]
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
			lower = true
		case c >= 'A' && c <= 'F':
			upper = true
		default:
			return false
		}
	}
	if !(lower && upper) {
		return true
	}
	hash := keccak256([]byte(strings.ToLower(hex)))
	for i := 0; i < len(hex); i++ {
		c := hex[i]
		if c < 'A' || (c > 'F' && c < 'a') || c > 'f' {
			continue // a digit has no case
		}
		nibble := hash[i/2]
		if i%2 == 0 {
			nibble >>= 4
		}
		nibble &= 0x0f
		wantUpper := nibble >= 8
		isUpper := c <= 'F'
		if wantUpper != isUpper {
			return false
		}
	}
	return true
}
