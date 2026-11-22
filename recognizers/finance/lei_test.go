package finance

import (
	"context"
	"testing"
)

// LEI tests. Positives: the Wikipedia example 54930084UKLVMY22DS16 (fetched
// 2026-11-22) and a synthetic value whose check digits were computed to give
// remainder 1 mod 97. Negatives: the same examples with a wrong check digit,
// lowercase letters, and a length one character short or long.
func TestLEIRecognize(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{"wiki-example", "Entity LEI 54930084UKLVMY22DS16 registered.", []string{"54930084UKLVMY22DS16"}},
		{"synthetic", "legal entity identifier 7ZQ4R9TM2KLBNX8WD935 on file", []string{"7ZQ4R9TM2KLBNX8WD935"}},
		{"bad-check-digit", "LEI 54930084UKLVMY22DS17", nil},
		{"bad-check-digit-synthetic", "LEI 7ZQ4R9TM2KLBNX8WD936", nil},
		{"lowercase-letters", "LEI 54930084ukLVMY22DS16", nil},
		{"nineteen-chars", "LEI 54930084UKLVMY22DS1", nil},
		{"twentyone-chars", "LEI 54930084UKLVMY22DS160", nil},
		{"glued-to-letters", "xLEI54930084UKLVMY22DS16", nil},
	}
	r := NewLEIRecognizer()
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
				if e.Type != "LEI" || e.Confidence != leiScore {
					t.Errorf("entity %+v: want type LEI at %v", e, leiScore)
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
