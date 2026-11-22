package checksum

import (
	"encoding/hex"
	"testing"
)

// Known Keccak-256 digests (original padding, as used by Ethereum). The empty
// input is the one that separates Keccak from SHA3-256, whose digest of "" is
// a7ffc6f8bf1ed76651c14756a061d662f580ff4de43b49fa82d80a4b80f8434a.
func TestKeccak256Vectors(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", "c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470"},
		{"abc", "4e03657aea45a94fc7d47ba826c8d667c0d1e6e33a64a036ec44f58fa12d6c45"},
	}
	for _, tt := range tests {
		got := keccak256([]byte(tt.in))
		if h := hex.EncodeToString(got[:]); h != tt.want {
			t.Errorf("keccak256(%q) = %s, want %s", tt.in, h, tt.want)
		}
	}
}

// Multi-block inputs cross the 136-byte rate: 135 bytes shares the padding byte
// (0x81), 136 fills a block and pads a whole new one, and 200 spans two blocks.
// No published digest for these lengths was checked, so this test only asserts
// that hashing is deterministic and that a one-byte-shorter input gives a
// different digest. The empty and "abc" vectors above are the real anchors.
func TestKeccak256BlockBoundaries(t *testing.T) {
	for _, n := range []int{135, 136, 200} {
		in := make([]byte, n)
		for i := range in {
			in[i] = byte(i)
		}
		a := keccak256(in)
		b := keccak256(in)
		if a != b {
			t.Fatalf("keccak256 of %d bytes is not deterministic", n)
		}
		if a == keccak256(in[:n-1]) {
			t.Errorf("keccak256 of %d bytes equals that of %d bytes", n, n-1)
		}
	}
}
