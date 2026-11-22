package finance

import (
	"context"
	"testing"
)

// Ethereum recognizer tests. Mixed-case addresses are synthetic, with their
// EIP-55 checksums computed from Keccak-256 (see checksum/ethereum_test.go for
// the EIP's own vectors).
func TestEthereumRecognize(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{"mixed-valid", "Send to 0x1a2B3C4d5e6F70819a2B3C4d5E6F70819a2B3C4D today.",
			[]string{"0x1a2B3C4d5e6F70819a2B3C4d5E6F70819a2B3C4D"}},
		{"all-lower-no-checksum", "wallet 0x1a2b3c4d5e6f70819a2b3c4d5e6f70819a2b3c4d",
			[]string{"0x1a2b3c4d5e6f70819a2b3c4d5e6f70819a2b3c4d"}},
		{"all-upper-hex-no-checksum", "to 0x52908400098527886E0F7030069857D2E4169EE7 ok",
			[]string{"0x52908400098527886E0F7030069857D2E4169EE7"}},
		{"mixed-one-letter-flipped", "to 0x1a2b3C4d5e6F70819a2B3C4d5E6F70819a2B3C4D ok", nil},
		{"mixed-second-address-flipped", "to 0x9C4E2F1A7b8d30e5C6a9f2B1D4e7c8a0F3B6D9E2 ok", nil},
		{"short-39-hex", "to 0x1a2B3C4d5e6F70819a2B3C4d5E6F70819a2B3C4", nil},
		{"long-41-hex", "to 0x1a2B3C4d5e6F70819a2B3C4d5E6F70819a2B3C4D1", nil},
		{"no-0x-prefix", "to 1a2B3C4d5e6F70819a2B3C4d5E6F70819a2B3C4D ok", nil},
		{"glued-to-letters", "to x0x1a2B3C4d5e6F70819a2B3C4d5E6F70819a2B3C4D", nil},
	}
	r := NewEthereumRecognizer()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ents, err := r.Recognize(context.Background(), tt.text)
			if err != nil {
				t.Fatalf("Recognize: %v", err)
			}
			var got []string
			for _, e := range ents {
				if tt.text[e.Start:e.End] != e.Text {
					t.Errorf("span [%d:%d] is %q but Text is %q", e.Start, e.End, tt.text[e.Start:e.End], e.Text)
				}
				if e.Type != "ETHEREUM" || e.Confidence != ethereumScore {
					t.Errorf("entity %+v: want ETHEREUM at %v", e, ethereumScore)
				}
				got = append(got, e.Text)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("got %q, want %q", got, tt.want)
				}
			}
		})
	}
}
