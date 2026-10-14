package checksum

import "testing"

func TestBitcoinValidate(t *testing.T) {
	tests := []struct {
		name    string
		address string
		want    bool
	}{
		// Base58Check, from the Bitcoin wiki examples.
		{"p2pkh-wiki", "1BoatSLRHtKNngkdXEeobR76b53LETtpyT", true},
		{"p2pkh-genesis-payout", "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa", true},
		{"p2sh-wiki", "3J98t1WpEZ73CNmQviecrnyiWrnqRhWNLy", true},
		{"p2pkh-last-char-changed", "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNb", false},
		{"p2sh-last-char-changed", "3J98t1WpEZ73CNmQviecrnyiWrnqRhWNLz", false},

		// bech32 version 0, BIP-173 vectors.
		{"bech32-p2wpkh", "bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4", true},
		{"bech32-p2wpkh-upper", "BC1QW508D6QEJXTDG4Y5R3ZARVARY0C5XW7KV8F3T4", true},
		{"bech32-p2wsh", "bc1qrp33g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpysxf3qccfmv3", true},
		{"bech32-checksum-changed", "bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t5", false},
		{"bech32-mixed-case", "BC1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4", false},
		{"bech32-wrong-hrp", "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4", false},

		// bech32m version 1, BIP-350 taproot vector.
		{"bech32m-taproot", "bc1p0xlxvlhemja6c4dqv22uapctqupfhlxm9h8z3k2e72q4k9hcz7vqzk5jj0", true},
		{"bech32m-checksum-changed", "bc1p0xlxvlhemja6c4dqv22uapctqupfhlxm9h8z3k2e72q4k9hcz7vqzk5jj1", false},

		{"empty", "", false},
		{"base58-invalid-char", "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfN0", false},
		{"random", "not a bitcoin address", false},
	}

	v := NewBitcoinValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := v.Validate(tt.address); got != tt.want {
				t.Errorf("Validate(%q) = %v, want %v", tt.address, got, tt.want)
			}
		})
	}
}
