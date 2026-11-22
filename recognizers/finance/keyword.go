// Package finance detects banking and card identifiers that have no checksum
// or no published universal shape. Each rule is keyword-gated: the value is
// reported only when a label for it (for example "routing number" or "CVV")
// sits directly before it, and the reported span is the value alone, never the
// label. Every keyword-gated rule in this package scores 0.85.
//
// The keyword is what makes these rules safe. A bare nine-digit run, a bare
// three-digit run or a bare four-digit run has no shape that separates it from
// any other number, so none of them is reported without its label.
package finance

import (
	"regexp"

	"github.com/u007/dataprotect/internal/keyword"
)

// keywordScore is the confidence for every keyword-gated finance rule.
const keywordScore = keyword.Score

// labelGate builds the shared keyword gate (see internal/keyword.LabelGate).
func labelGate(kw, value string) *regexp.Regexp { return keyword.LabelGate(kw, value) }
