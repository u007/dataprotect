// Package operators provides anonymization operators for PII entities.
package operators

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
)

// FPEOperator implements format-preserving encryption using FF1 (NIST SP 800-38G).
// It encrypts values so the ciphertext has the same format as the plaintext:
// e.g. a 16-digit card number encrypts to a 16-digit string, a 10-digit phone
// number encrypts to a 10-digit string.
// The same key, tweak and value always produce the same ciphertext, which keeps
// referential integrity across a dataset.
// Decrypt reverses the encryption with the same key and tweak.
type FPEOperator struct {
	key    []byte // AES key (16, 24, or 32 bytes)
	radix  int    // radix of the input alphabet (2..62)
	tweak  []byte // tweak data that modifies the encryption
	block  cipher.Block
	rounds int // number of Feistel rounds (8..10)
}

// NewFPEOperator creates a new FPE operator.
// key must be a valid AES key length (16, 24, or 32 bytes).
// radix must be between 2 and 62 (e.g. 10 for decimal digits, 16 for hex).
// tweak is additional data bound to the encryption; changing it changes the
// ciphertext for the same input. tweak must be at most 64 bytes (512 bits,
// the FF1 limit).
// rounds is the number of Feistel rounds; values outside 8..10 are clamped.
func NewFPEOperator(key []byte, radix int, tweak []byte, rounds int) (*FPEOperator, error) {
	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		return nil, errors.New("fpe: key must be 16, 24, or 32 bytes")
	}
	if radix < 2 || radix > 62 {
		return nil, errors.New("fpe: radix must be between 2 and 62")
	}
	if len(tweak) > 64 {
		return nil, errors.New("fpe: tweak must be at most 64 bytes")
	}
	if rounds < 8 {
		rounds = 8
	}
	if rounds > 10 {
		rounds = 10
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return &FPEOperator{
		key:    key,
		radix:  radix,
		tweak:  tweak,
		block:  block,
		rounds: rounds,
	}, nil
}

// Name returns the operator name.
func (o *FPEOperator) Name() string { return "fpe" }

// Replace encrypts value using FF1 and returns the ciphertext.
// It satisfies the Operator interface.
func (o *FPEOperator) Replace(ctx context.Context, value string) (string, error) {
	return o.Encrypt(value)
}

// Encrypt applies FF1 encryption to value, preserving its format.
// Each character of value must be encodable in the configured radix
// (for radix 10, only '0'-'9'; for radix 62, '0'-'9', 'a'-'z', 'A'-'Z').
// The ciphertext has the same length as the plaintext.
func (o *FPEOperator) Encrypt(value string) (string, error) {
	n := len(value)
	if n < 2 {
		return "", errors.New("fpe: input must be at least 2 characters")
	}

	// FF1's Feistel network requires equal half sizes (lenL == lenR).
	// For odd-length inputs, pad with a leading zero so both halves are
	// n/2 digits, then drop the leading character of the ciphertext.
	// The dropped character is recovered during decryption by trying all
	// possible leading digits and finding the one whose decryption starts
	// with the padding zero.
	padded := value
	if n%2 != 0 {
		padded = "0" + value
	}

	ciphertext, err := o.feistelEncrypt(padded)
	if err != nil {
		return "", err
	}
	if n%2 != 0 {
		return ciphertext[1:], nil
	}
	return ciphertext, nil
}

// feistelEncrypt applies the FF1 Feistel network to an even-length value.
// The value must have an even number of digits encodable in the radix.
func (o *FPEOperator) feistelEncrypt(value string) (string, error) {
	n := len(value)
	if n%2 != 0 {
		return "", errors.New("fpe: feistelEncrypt requires even-length input")
	}

	// Convert each character to a digit in the configured radix.
	digits := make([]int, n)
	for i := 0; i < n; i++ {
		d, ok := charToDigit(value[i], o.radix)
		if !ok {
			return "", fmt.Errorf("fpe: byte %#x at position %d not encodable in radix %d", value[i], i, o.radix)
		}
		digits[i] = d
	}

	half := n / 2
	bigRadix := big.NewInt(int64(o.radix))
	mod := new(big.Int).Exp(bigRadix, big.NewInt(int64(half)), nil)

	// Split into two equal halves.
	L := digitsToBigInt(digits[:half], o.radix)
	R := digitsToBigInt(digits[half:], o.radix)

	// Feistel network. Each round:
	//   c = f(R, i)              // f outputs < R^half
	//   L = (L + c) mod R^half
	//   swap(L, R)
	// With equal halves, both L and R stay < R^half throughout.
	for i := 0; i < o.rounds; i++ {
		c := o.roundFunc(R, i, half, mod)
		L.Add(L, c)
		L.Mod(L, mod)
		L, R = R, L // swap halves
	}
	// Undo the swap from the final round.
	L, R = R, L

	// Convert back to characters.
	result := make([]byte, n)
	for i, d := range bigIntToDigits(L, o.radix, half) {
		result[i] = digitToChar(byte(d))
	}
	for i, d := range bigIntToDigits(R, o.radix, half) {
		result[half+i] = digitToChar(byte(d))
	}
	return string(result), nil
}

// Decrypt reverses FF1 encryption: given ciphertext produced by Encrypt with
// the same key, tweak and radix, returns the original plaintext.
func (o *FPEOperator) Decrypt(value string) (string, error) {
	n := len(value)
	if n < 2 {
		return "", errors.New("fpe: input must be at least 2 characters")
	}

	if n%2 == 0 {
		return o.feistelDecrypt(value)
	}

	// Odd-length ciphertext: the leading character was dropped during
	// Encrypt. Try every possible leading digit; the correct one decrypts
	// to a plaintext whose first character is the padding zero.
	var firstErr error
	for d := 0; d < o.radix; d++ {
		padded := string(digitToChar(byte(d))) + value
		plaintext, err := o.feistelDecrypt(padded)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if len(plaintext) > 0 && plaintext[0] == '0' {
			return plaintext[1:], nil
		}
	}
	if firstErr != nil {
		return "", firstErr
	}
	return "", errors.New("fpe: no valid decryption found for odd-length input")
}

// feistelDecrypt reverses the FF1 Feistel network for an even-length value.
func (o *FPEOperator) feistelDecrypt(value string) (string, error) {
	n := len(value)
	if n%2 != 0 {
		return "", errors.New("fpe: feistelDecrypt requires even-length input")
	}

	digits := make([]int, n)
	for i := 0; i < n; i++ {
		d, ok := charToDigit(value[i], o.radix)
		if !ok {
			return "", fmt.Errorf("fpe: byte %#x at position %d not encodable in radix %d", value[i], i, o.radix)
		}
		digits[i] = d
	}

	half := n / 2
	bigRadix := big.NewInt(int64(o.radix))
	mod := new(big.Int).Exp(bigRadix, big.NewInt(int64(half)), nil)

	L := digitsToBigInt(digits[:half], o.radix)
	R := digitsToBigInt(digits[half:], o.radix)

	// Undo the "undo final swap" from Encrypt.
	L, R = R, L

	// Reverse the Feistel network, rounds in reverse order.
	// Each inverse round:
	//   swap(L, R)                      // undo this round's swap
	//   c = f(R, i)                     // f outputs < R^half
	//   L = (L - c) mod R^half          // recover old L
	for i := o.rounds - 1; i >= 0; i-- {
		L, R = R, L // undo this round's swap
		c := o.roundFunc(R, i, half, mod)
		L.Sub(L, c)
		L.Mod(L, mod)
	}

	// Convert back to characters.
	result := make([]byte, n)
	for i, d := range bigIntToDigits(L, o.radix, half) {
		result[i] = digitToChar(byte(d))
	}
	for i, d := range bigIntToDigits(R, o.radix, half) {
		result[half+i] = digitToChar(byte(d))
	}
	return string(result), nil
}

// roundFunc is the FF1 round function: a keyed pseudo-random function that
// maps an integer X to an integer in [0, mod). X must be < radix^maxDigits.
// The output is reduced modulo mod (typically R^half).
func (o *FPEOperator) roundFunc(X *big.Int, round int, maxDigits int, mod *big.Int) *big.Int {
	// Encode X as big-endian bytes, padded to enough bytes to represent
	// values up to radix^maxDigits - 1.
	bitLen := (maxDigits*bitsPerDigit(o.radix) + 7) / 8
	xBytes := X.Bytes()
	if len(xBytes) < bitLen {
		padded := make([]byte, bitLen)
		copy(padded[bitLen-len(xBytes):], xBytes)
		xBytes = padded
	}

	// Build the AES input:
	//   0x00 || round || tweakLen || tweak || padding || numDigits (2 bytes BE) || xBytes
	// Padding fills so that everything up to and including xBytes is a
	// multiple of the AES block size.
	tweakLen := byte(len(o.tweak))
	preamble := make([]byte, 0, 4+len(o.tweak)+2+len(xBytes))
	preamble = append(preamble, 0x00)        // domain separator
	preamble = append(preamble, byte(round)) // round number
	preamble = append(preamble, tweakLen)
	preamble = append(preamble, o.tweak...)

	targetLen := ((len(preamble) + 2 + len(xBytes) + o.block.BlockSize() - 1) / o.block.BlockSize()) * o.block.BlockSize()
	preamble = append(preamble, make([]byte, targetLen-len(preamble)-2-len(xBytes))...)

	numDigits := uint16(maxDigits)
	numDigitsBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(numDigitsBytes, numDigits)
	preamble = append(preamble, numDigitsBytes...)
	preamble = append(preamble, xBytes...)

	// CBC-MAC-like construction: chain AES encryptions, output final block.
	blockSize := o.block.BlockSize()
	var state []byte
	for off := 0; off < len(preamble); off += blockSize {
		blk := preamble[off : off+blockSize]
		if state != nil {
			for i := range blk {
				blk[i] ^= state[i]
			}
		}
		enc := make([]byte, blockSize)
		o.block.Encrypt(enc, blk)
		state = enc
	}

	// Reduce the final ciphertext block modulo mod.
	out := new(big.Int).SetBytes(state)
	out.Mod(out, mod)
	return out
}

// charToDigit maps a single byte to its numeric value in the given radix,
// or reports failure if the byte is not a valid digit for that radix.
func charToDigit(b byte, radix int) (int, bool) {
	switch {
	case b >= '0' && b <= '9':
		d := int(b - '0')
		if d < radix {
			return d, true
		}
	case b >= 'a' && b <= 'z':
		d := 10 + int(b-'a')
		if d < radix {
			return d, true
		}
	case b >= 'A' && b <= 'Z':
		d := 36 + int(b-'A')
		if d < radix {
			return d, true
		}
	}
	return 0, false
}

// digitToChar maps a digit value (0..radix-1) to its character representation.
func digitToChar(d byte) byte {
	switch {
	case d < 10:
		return '0' + d
	case d < 36:
		return 'a' + (d - 10)
	default:
		return 'A' + (d - 36)
	}
}

// bitsPerDigit returns ceil(log2(radix)), the number of bits needed to
// represent one digit in the given radix.
func bitsPerDigit(radix int) int {
	bits := 0
	for v := 1; v < radix; v <<= 1 {
		bits++
	}
	return bits
}

// digitsToBigInt converts a slice of digit values (most significant first)
// to a big.Int in the given radix.
func digitsToBigInt(digits []int, radix int) *big.Int {
	r := big.NewInt(int64(radix))
	result := big.NewInt(0)
	for _, d := range digits {
		result.Mul(result, r)
		result.Add(result, big.NewInt(int64(d)))
	}
	return result
}

// bigIntToDigits converts a big.Int to a slice of exactly `count` digit
// values (most significant first) in the given radix, padding with leading
// zeros if necessary.
func bigIntToDigits(x *big.Int, radix int, count int) []byte {
	r := big.NewInt(int64(radix))
	digits := make([]byte, count)
	remaining := new(big.Int).Set(x)
	for i := count - 1; i >= 0; i-- {
		mod := new(big.Int).Mod(remaining, r)
		digits[i] = byte(mod.Int64())
		remaining.Div(remaining, r)
	}
	return digits
}

// MustFPEOperator is like NewFPEOperator but panics on error.
// It is intended for use in variable initializations where error handling
// would be awkward.
func MustFPEOperator(key []byte, radix int, tweak []byte, rounds int) *FPEOperator {
	o, err := NewFPEOperator(key, radix, tweak, rounds)
	if err != nil {
		panic(err)
	}
	return o
}

// compile-time interface check
var _ Operator = (*FPEOperator)(nil)
