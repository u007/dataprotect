package golden

import (
	"net/netip"
	"regexp"
	"strings"
)

// ipToken is a run of hex digits, colons and dots that holds a colon: a candidate
// IPv6 address, or a clock time, which netip rejects.
var ipToken = regexp.MustCompile(`[0-9A-Fa-f:.]*:[0-9A-Fa-f:.]*`)

// SameModuloSpelling reports whether two texts differ only in how a value is
// written: spacing, separators, parentheses, letter case, and IPv6 notation (an
// address written in full or compressed). Pseudonymize gives one placeholder to
// every spelling of a value, and restoring gives the first spelling seen, so a
// restored value is compared with this. Use PseudonymRoundTrip for a whole text,
// because this comparison ignores spacing across the whole string.
func SameModuloSpelling(a, b string) bool {
	return normalizeSpelling(a) == normalizeSpelling(b)
}

func normalizeSpelling(s string) string {
	s = ipToken.ReplaceAllStringFunc(s, func(token string) string {
		if addr, err := netip.ParseAddr(token); err == nil {
			return addr.String()
		}
		return token
	})
	return strings.ToLower(strings.NewReplacer(" ", "", "-", "", ".", "", "(", "", ")", "").Replace(s))
}
