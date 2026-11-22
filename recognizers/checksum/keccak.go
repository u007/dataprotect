package checksum

import (
	"encoding/binary"
	"math/bits"
)

// This file is the original Keccak-256 hash, the one Ethereum uses. It is NOT
// NIST SHA3-256: the two differ only in the padding byte (0x01 here, 0x06 in
// FIPS 202), so the outputs differ for every input. The standard library's
// crypto/sha3 implements only the FIPS 202 variants, so the permutation is
// written out here. Rate 136 bytes (1088 bits), capacity 512 bits, 32 bytes out.
//
// Reference: The Keccak SHA-3 submission, Keccak-f[1600] with 24 rounds.
// Checked against the empty-input vector in keccak_test.go.

// keccakRoundConstants is the iota step's round constant for each of the 24 rounds.
var keccakRoundConstants = [24]uint64{
	0x0000000000000001, 0x0000000000008082, 0x800000000000808A, 0x8000000080008000,
	0x000000000000808B, 0x0000000080000001, 0x8000000080008081, 0x8000000000008009,
	0x000000000000008A, 0x0000000000000088, 0x0000000080008009, 0x000000008000000A,
	0x000000008000808B, 0x800000000000008B, 0x8000000000008089, 0x8000000000008003,
	0x8000000000008002, 0x8000000000000080, 0x000000000000800A, 0x800000008000000A,
	0x8000000080008081, 0x8000000000008080, 0x0000000080000001, 0x8000000080008008,
}

// keccakRotations holds the rho step's rotation offset for lane x+5y.
var keccakRotations = [25]int{
	0, 1, 62, 28, 27,
	36, 44, 6, 55, 20,
	3, 10, 43, 25, 39,
	41, 45, 15, 21, 8,
	18, 2, 61, 56, 14,
}

// keccakF1600 applies the 24-round permutation to the 25-lane state in place.
func keccakF1600(a *[25]uint64) {
	for round := 0; round < 24; round++ {
		// theta
		var c [5]uint64
		for x := 0; x < 5; x++ {
			c[x] = a[x] ^ a[x+5] ^ a[x+10] ^ a[x+15] ^ a[x+20]
		}
		for x := 0; x < 5; x++ {
			d := c[(x+4)%5] ^ bits.RotateLeft64(c[(x+1)%5], 1)
			for y := 0; y < 25; y += 5 {
				a[x+y] ^= d
			}
		}
		// rho and pi: B[y, 2x+3y] = rot(A[x, y], r[x, y])
		var b [25]uint64
		for x := 0; x < 5; x++ {
			for y := 0; y < 5; y++ {
				b[y+5*((2*x+3*y)%5)] = bits.RotateLeft64(a[x+5*y], keccakRotations[x+5*y])
			}
		}
		// chi
		for y := 0; y < 25; y += 5 {
			for x := 0; x < 5; x++ {
				a[x+y] = b[x+y] ^ (^b[(x+1)%5+y] & b[(x+2)%5+y])
			}
		}
		// iota
		a[0] ^= keccakRoundConstants[round]
	}
}

// keccak256 returns the Keccak-256 digest of data (original padding, not SHA3-256).
func keccak256(data []byte) [32]byte {
	const rate = 136
	var a [25]uint64
	for len(data) >= rate {
		keccakAbsorb(&a, data[:rate])
		keccakF1600(&a)
		data = data[rate:]
	}
	// Final block: the message, then the 0x01 domain byte, then zeros, with
	// 0x80 in the last byte of the rate. A message of 135 bytes shares that
	// byte, so the two pad bits meet as 0x81.
	var last [rate]byte
	copy(last[:], data)
	last[len(data)] ^= 0x01
	last[rate-1] ^= 0x80
	keccakAbsorb(&a, last[:])
	keccakF1600(&a)

	var out [32]byte
	for i := 0; i < 4; i++ {
		binary.LittleEndian.PutUint64(out[8*i:], a[i])
	}
	return out
}

// keccakAbsorb XORs one rate-sized block into the state, lane by lane, little-endian.
func keccakAbsorb(a *[25]uint64, block []byte) {
	for i := 0; i < len(block)/8; i++ {
		a[i] ^= binary.LittleEndian.Uint64(block[8*i:])
	}
}
