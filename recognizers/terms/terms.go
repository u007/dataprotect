// Package terms reports caller-supplied terms such as client names, party names,
// matter codes and deal code names. The caller knows these values, so the
// recognizer is not registered in the default pipeline: it needs input, and is
// added with piiscrub.WithRecognizer.
//
// Matching rules:
//   - case-insensitive (Unicode simple folding);
//   - whole terms only: the rune before and the rune after a match must not be a
//     letter or a digit, so "Ann" does not match inside "Anna";
//   - a run of whitespace inside a term matches any run of whitespace in the
//     text, so "Tan Ah Kow" matches across a line break;
//   - where two matches overlap, the longer one wins, then the earlier one.
//
// Every match scores 1.00 because the caller asserted it. The analyzer lets a
// CUSTOM_TERM span win every overlap (see winsOverlap in analyzer).
package terms

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/u007/dataprotect/recognizers"
)

// span is a byte range [start, end) of the scanned text.
type span struct{ start, end int }

// Score is the confidence of every caller-supplied term.
const Score = 1.00

// Recognizer reports the occurrences of a fixed set of terms. Each term is
// compiled once in New, so Recognize does no per-call compilation.
type Recognizer struct {
	entityType recognizers.EntityType
	patterns   []*regexp.Regexp
}

// New builds a recognizer that reports every occurrence of each term as
// entityType. Use recognizers.EntityCustomTerm for terms that fit no other
// type; EntityPerson and EntityOrganization are also accepted.
//
// New fails when entityType is empty or when a term is empty or only
// whitespace, so a blank entry in a caller's list is caught at construction.
// Duplicate terms are accepted and compiled once.
func New(entityType recognizers.EntityType, terms ...string) (*Recognizer, error) {
	if entityType == "" {
		return nil, fmt.Errorf("terms: entity type is empty")
	}
	seen := make(map[string]bool, len(terms))
	r := &Recognizer{entityType: entityType}
	for i, term := range terms {
		words := strings.Fields(term)
		if len(words) == 0 {
			return nil, fmt.Errorf("terms: term %d is empty or whitespace only", i)
		}
		key := strings.Join(words, " ")
		if seen[strings.ToLower(key)] {
			continue
		}
		seen[strings.ToLower(key)] = true
		quoted := make([]string, len(words))
		for j, w := range words {
			quoted[j] = regexp.QuoteMeta(w)
		}
		re, err := regexp.Compile(`(?i)` + strings.Join(quoted, `\s+`))
		if err != nil {
			return nil, fmt.Errorf("terms: term %d: %w", i, err)
		}
		r.patterns = append(r.patterns, re)
	}
	return r, nil
}

// Name returns the recognizer name.
func (r *Recognizer) Name() string { return "custom_terms" }

// Recognize reports every whole-term occurrence in text, in text order.
// Offsets are byte offsets into text and Text is the original substring.
func (r *Recognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	var found []span
	for _, re := range r.patterns {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		found = append(found, occurrences(re, text)...)
	}
	// Longest wins at a shared start, then the earlier span; a span that
	// overlaps one already kept is dropped.
	sort.Slice(found, func(i, j int) bool {
		if found[i].start != found[j].start {
			return found[i].start < found[j].start
		}
		return found[i].end > found[j].end
	})
	var entities []recognizers.Entity
	last := -1
	for _, s := range found {
		if s.start < last {
			continue
		}
		last = s.end
		entities = append(entities, recognizers.Entity{
			Type:       r.entityType,
			Start:      s.start,
			End:        s.end,
			Text:       text[s.start:s.end],
			Confidence: Score,
		})
	}
	return entities, nil
}

// occurrences returns the whole-term matches of re in text. When a match fails
// the boundary check, the search restarts one byte after its start, so a term
// that overlaps itself (such as "aba" in "ababa") is not missed.
func occurrences(re *regexp.Regexp, text string) []span {
	var out []span
	for pos := 0; pos <= len(text); {
		loc := re.FindStringIndex(text[pos:])
		if loc == nil {
			break
		}
		start, end := pos+loc[0], pos+loc[1]
		if wholeTerm(text, start, end) {
			out = append(out, span{start, end})
			pos = end
			continue
		}
		pos = start + 1
	}
	return out
}

// wholeTerm reports whether the match at [start, end) is not glued to a letter
// or a digit on either side.
func wholeTerm(text string, start, end int) bool {
	if start > 0 {
		before, _ := utf8.DecodeLastRuneInString(text[:start])
		if isWordRune(before) {
			return false
		}
	}
	if end < len(text) {
		after, _ := utf8.DecodeRuneInString(text[end:])
		if isWordRune(after) {
			return false
		}
	}
	return true
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }
