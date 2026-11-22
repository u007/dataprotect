package finance

import (
	"context"
	"regexp"

	"github.com/u007/dataprotect/recognizers"
	"github.com/u007/dataprotect/recognizers/checksum"
)

// EthereumRecognizer finds Ethereum addresses. Shape: "0x" followed by 40
// hexadecimal characters. A mixed-case address must pass the EIP-55 checksum;
// an all-lowercase or all-uppercase address is accepted by shape. Source:
// https://eips.ethereum.org/EIPS/eip-55 (fetched 2026-11-22). The checksum
// itself is in checksum.ValidEthereumAddress.
type EthereumRecognizer struct {
	pattern *regexp.Regexp
}

// ethereumScore matches BITCOIN, the closest existing checksummed address type.
// The recognizer reports the whole address, so there is no label to strip.
const ethereumScore = 0.95

// NewEthereumRecognizer creates an Ethereum address recognizer.
func NewEthereumRecognizer() *EthereumRecognizer {
	return &EthereumRecognizer{pattern: regexp.MustCompile(`\b0x[0-9a-fA-F]{40}\b`)}
}

// Name returns the recognizer name.
func (r *EthereumRecognizer) Name() string { return "finance_ethereum" }

// Recognize detects Ethereum addresses whose checksum (if any) verifies.
func (r *EthereumRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	var entities []recognizers.Entity
	for _, m := range r.pattern.FindAllStringIndex(text, -1) {
		addr := text[m[0]:m[1]]
		if !checksum.ValidEthereumAddress(addr) {
			continue
		}
		entities = append(entities, recognizers.Entity{
			Type:       recognizers.EntityEthereum,
			Start:      m[0],
			End:        m[1],
			Text:       addr,
			Confidence: ethereumScore,
		})
	}
	return entities, nil
}
