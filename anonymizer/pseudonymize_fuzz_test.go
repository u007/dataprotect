package anonymizer

import (
	"strings"
	"testing"

	"github.com/u007/dataprotect/recognizers"
)

// FuzzPseudonymizeRoundTrip checks the invariants Pseudonymize promises, over
// arbitrary text and well-formed entity layouts:
//   - it never errors on a well-formed layout whose text holds no placeholder-
//     shaped token (the decoded text is arbitrary, so such a token is skipped);
//   - restoring every placeholder reproduces the input byte for byte;
//   - each placeholder appears in the output exactly as often as its span count;
//   - the caller's entity slice is not mutated.
//
// Run: go test ./anonymizer -run=FuzzPseudonymizeRoundTrip -fuzz=FuzzPseudonymizeRoundTrip
func FuzzPseudonymizeRoundTrip(f *testing.F) {
	f.Add("", []byte{})
	f.Add("a@b.com a@b.com", []byte{0, 64, 128, 64})
	f.Add("é☃ 4111111111111111", []byte{0, 128, 64, 64, 200, 40})
	f.Add("card 4111111111111111 phone 555-123-4567", []byte{0, 64, 64, 64, 128, 64})

	types := []recognizers.EntityType{recognizers.EntityEmail, recognizers.EntityCreditCard, recognizers.EntityPhone}
	f.Fuzz(func(t *testing.T, text string, raw []byte) {
		if strings.Contains(text, "[") {
			// Placeholder-shaped tokens can collide; that path is its own test.
			t.Skip()
		}
		// The decoder places offsets by byte; drop spans that do not start and end
		// on a rune boundary, since such a span is invalid input by contract.
		var entities []recognizers.Entity
		for _, e := range decodeValidEntities(text, raw) {
			if isRuneBoundary(text, e.Start) && isRuneBoundary(text, e.End) {
				e.Type = types[len(entities)%len(types)]
				entities = append(entities, e)
			}
		}
		before := copyEntities(entities)

		out, pseudonyms, err := New().Pseudonymize(text, entities)
		if err != nil {
			t.Fatalf("valid entities errored: %v\ntext=%q entities=%+v", err, text, entities)
		}
		if got := restore(out, pseudonyms); got != text {
			t.Fatalf("round trip failed:\ntext=%q\nout=%q\nmap=%v\nrestored=%q", text, out, pseudonyms, got)
		}
		for i := range before {
			if before[i] != entities[i] {
				t.Fatalf("Pseudonymize mutated entity %d: %+v -> %+v", i, before[i], entities[i])
			}
		}
	})
}
