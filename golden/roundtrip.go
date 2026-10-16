package golden

import "strings"

// PseudonymRoundTrip reports whether pseudonymized is original with every value
// replaced by its placeholder, where pseudonyms maps each placeholder to a value
// as Pseudonymize returns it. The check aligns the two texts. Text outside the
// placeholders must match original byte for byte. The stretch of original that a
// placeholder stands for must be SameModuloSpelling with that placeholder's value.
//
// The edge of a value is the one place the check is loose. A stretch may take up
// separators beside it, because "(555) 867-5309" and "555-867-5309" differ only
// there. The alignment cannot tell a separator that belongs to the value from one
// that belongs to the prose, so a restore that drops the full stop after an email
// still passes. Only the detector's spans could settle that.
//
// Only the values get the spelling allowance. Comparing the whole text modulo
// spelling would also accept a corrupted sentence whose only change is spacing,
// punctuation or case.
func PseudonymRoundTrip(original, pseudonymized string, pseudonyms map[string]string) bool {
	lits, phs := splitPlaceholders(pseudonymized, pseudonyms)
	m := &roundTripMatcher{original: original, lits: lits, phs: phs, pseudonyms: pseudonyms, failed: map[[2]int]bool{}}
	return m.match(0, 0)
}

// splitPlaceholders cuts s at each placeholder that is a key of pseudonyms. It
// returns len(phs)+1 literal pieces, so lits[i] sits before phs[i] and the last
// literal sits after the last placeholder. A placeholder-shaped token that is not
// a key stays inside a literal.
func splitPlaceholders(s string, pseudonyms map[string]string) (lits, phs []string) {
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] != '[' {
			continue
		}
		end := strings.IndexByte(s[i:], ']')
		if end < 0 {
			break
		}
		token := s[i : i+end+1]
		if _, ok := pseudonyms[token]; !ok {
			continue
		}
		lits = append(lits, s[start:i])
		phs = append(phs, token)
		i += end
		start = i + 1
	}
	return append(lits, s[start:]), phs
}

// roundTripMatcher holds the state for one alignment. A placeholder's extent in
// original is not fixed by the text: a value can be spelled with trailing
// separators, so more than one end can match. The matcher tries each end and
// remembers the states that failed, so no state is searched twice.
type roundTripMatcher struct {
	original   string
	lits       []string
	phs        []string
	pseudonyms map[string]string
	failed     map[[2]int]bool
}

// match reports whether lits[i] and the rest of the pieces align with original
// from byte offset cursor onwards.
func (m *roundTripMatcher) match(i, cursor int) bool {
	key := [2]int{i, cursor}
	if m.failed[key] {
		return false
	}
	ok := m.tryMatch(i, cursor)
	if !ok {
		m.failed[key] = true
	}
	return ok
}

func (m *roundTripMatcher) tryMatch(i, cursor int) bool {
	lit := m.lits[i]
	if !strings.HasPrefix(m.original[cursor:], lit) {
		return false
	}
	cursor += len(lit)
	if i == len(m.phs) {
		return cursor == len(m.original)
	}
	value := m.pseudonyms[m.phs[i]]
	next := m.lits[i+1]
	for end := cursor; end <= len(m.original); end++ {
		if !strings.HasPrefix(m.original[end:], next) {
			continue
		}
		if SameModuloSpelling(m.original[cursor:end], value) && m.match(i+1, end) {
			return true
		}
	}
	return false
}
