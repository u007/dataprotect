package checksum

import (
	"bytes"
	"crypto/sha256"
	"math/big"
	"strings"
)

// base58Alphabet is the Bitcoin base58 alphabet: no 0, O, I or l.
const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// bech32Charset is the bech32 data alphabet from BIP-173, indexed by value.
const bech32Charset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"

// Checksum constants from BIP-173 (bech32) and BIP-350 (bech32m).
const (
	bech32Const  uint32 = 1
	bech32mConst uint32 = 0x2bc830a3
)

// BitcoinValidator verifies bitcoin address checksums. Base58Check covers the
// legacy P2PKH ("1...") and P2SH ("3...") forms. Witness forms ("bc1...") use
// bech32 for version 0 (BIP-173) and bech32m for version 1 and later (BIP-350).
type BitcoinValidator struct{}

// NewBitcoinValidator creates a new bitcoin address validator.
func NewBitcoinValidator() *BitcoinValidator {
	return &BitcoinValidator{}
}

// Name returns the validator name.
func (v *BitcoinValidator) Name() string { return "checksum_bitcoin" }

// Validate reports whether address carries a valid checksum for its form.
func (v *BitcoinValidator) Validate(address string) bool {
	if strings.HasPrefix(address, "1") || strings.HasPrefix(address, "3") {
		return validBase58Check(address)
	}
	return validBech32(address)
}

// validBase58Check decodes a base58 string and checks its 4-byte double-SHA256
// suffix. A Base58Check payload is 1 version byte, 20 hash bytes and 4 checksum
// bytes.
func validBase58Check(s string) bool {
	raw, ok := base58Decode(s)
	if !ok || len(raw) != 25 {
		return false
	}
	first := sha256.Sum256(raw[:21])
	second := sha256.Sum256(first[:])
	return bytes.Equal(second[:4], raw[21:])
}

// base58Decode converts a base58 string to bytes. Each leading '1' is a leading
// zero byte, which big.Int alone would drop.
func base58Decode(s string) ([]byte, bool) {
	n := new(big.Int)
	base := big.NewInt(58)
	for i := 0; i < len(s); i++ {
		idx := strings.IndexByte(base58Alphabet, s[i])
		if idx < 0 {
			return nil, false
		}
		n.Mul(n, base)
		n.Add(n, big.NewInt(int64(idx)))
	}
	leading := len(s) - len(strings.TrimLeft(s, "1"))
	return append(make([]byte, leading), n.Bytes()...), true
}

// validBech32 checks a bech32 or bech32m witness address: the hrp must be "bc",
// case must not be mixed, the checksum must match the version's constant, and
// the program must have a legal length for its version.
func validBech32(s string) bool {
	if len(s) > 90 {
		return false
	}
	lower := strings.ToLower(s)
	if s != lower && s != strings.ToUpper(s) {
		return false
	}
	pos := strings.LastIndexByte(lower, '1')
	if pos < 0 || lower[:pos] != "bc" {
		return false
	}
	values := make([]byte, 0, len(lower)-pos-1)
	for i := pos + 1; i < len(lower); i++ {
		idx := strings.IndexByte(bech32Charset, lower[i])
		if idx < 0 {
			return false
		}
		values = append(values, byte(idx))
	}
	// One version character and a six-character checksum at minimum.
	if len(values) < 7 {
		return false
	}

	version := values[0]
	if version > 16 {
		return false
	}
	want := bech32Const
	if version != 0 {
		want = bech32mConst
	}
	if bech32Polymod(append(bech32HRPExpand("bc"), values...)) != want {
		return false
	}

	program, ok := convertBits(values[1:len(values)-6], 5, 8)
	if !ok || len(program) < 2 || len(program) > 40 {
		return false
	}
	if version == 0 && len(program) != 20 && len(program) != 32 {
		return false
	}
	return true
}

// bech32HRPExpand expands the human-readable part for the checksum, per BIP-173.
func bech32HRPExpand(hrp string) []byte {
	out := make([]byte, 0, 2*len(hrp)+1)
	for i := 0; i < len(hrp); i++ {
		out = append(out, hrp[i]>>5)
	}
	out = append(out, 0)
	for i := 0; i < len(hrp); i++ {
		out = append(out, hrp[i]&31)
	}
	return out
}

// bech32Polymod is the BCH checksum over 5-bit values, per BIP-173.
func bech32Polymod(values []byte) uint32 {
	gen := [5]uint32{0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3}
	chk := uint32(1)
	for _, v := range values {
		top := chk >> 25
		chk = (chk&0x1ffffff)<<5 ^ uint32(v)
		for i := range gen {
			if (top>>i)&1 == 1 {
				chk ^= gen[i]
			}
		}
	}
	return chk
}

// convertBits regroups bits from 'from'-bit values to 'to'-bit values with no
// padding, per BIP-173. It fails on out-of-range input, on a leftover group of
// a full source width, and on non-zero padding bits.
func convertBits(data []byte, from, to uint) ([]byte, bool) {
	var acc uint32
	var bits uint
	maxv := uint32(1)<<to - 1
	out := make([]byte, 0, len(data)*int(from)/int(to))
	for _, v := range data {
		if uint32(v)>>from != 0 {
			return nil, false
		}
		acc = acc<<from | uint32(v)
		bits += from
		for bits >= to {
			bits -= to
			out = append(out, byte((acc>>bits)&maxv))
		}
	}
	if bits >= from || (acc<<(to-bits))&maxv != 0 {
		return nil, false
	}
	return out, true
}
