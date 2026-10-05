package operators

import (
	"context"
	"testing"
)

func TestReplaceOperator(t *testing.T) {
	o := NewReplaceOperator("<EMAIL>")
	got, err := o.Replace(context.Background(), "john@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "<EMAIL>" {
		t.Errorf("got %q, want <EMAIL>", got)
	}
}

func TestReplaceOperatorKeepsReplacementLengthIndependent(t *testing.T) {
	o := NewReplaceOperator("[NAME]")
	// The replacement is a fixed string regardless of input length.
	for _, in := range []string{"a", "john", "a very long value indeed"} {
		got, err := o.Replace(context.Background(), in)
		if err != nil {
			t.Fatalf("unexpected error on %q: %v", in, err)
		}
		if got != "[NAME]" {
			t.Errorf("Replace(%q) = %q, want [NAME]", in, got)
		}
	}
}

func TestMaskOperator(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		input   string
		want    string
	}{
		{"card plain", "****-****-****-####", "4111111111111111", "****-****-****-1111"},
		{"card spaced", "****-****-****-####", "4111 1111 1111 1111", "****-****-****-1111"},
		{"card hyphened", "****-****-****-####", "4111-1111-1111-1111", "****-****-****-1111"},
		{"phone", "***-***-####", "5551234567", "***-***-4567"},
		{"phone hyphened", "***-***-####", "555-123-4567", "***-***-4567"},
		{"ssn", "***-**-####", "123456789", "***-**-6789"},
		{"ssn dotted", "***-**-####", "123.45.6789", "***-**-6789"},
		// The pattern's slots are consumed left to right; anything past the end
		// of the pattern is masked, and unfilled slots are masked too.
		{"input longer than pattern", "##", "abcdefgh", "ab******"},
		{"input shorter than pattern", "########", "abc", "abc*****"},
		{"empty pattern masks everything", "", "abcdef", "******"},
		{"empty input", "****", "", "****"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewMaskOperator(tt.pattern).Replace(context.Background(), tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("mask %q with pattern %q = %q, want %q", tt.input, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestMaskOperatorNeverLeaksUnmaskedDigits(t *testing.T) {
	// The masked output must not reveal any character the pattern marks '*'.
	o := NewMaskOperator("****-****-****-####")
	got, err := o.Replace(context.Background(), "4111111111111111")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, c := range got {
		if c == '*' {
			continue
		}
		if c >= '0' && c <= '9' {
			// A digit may only appear in the final four positions.
			if i < len(got)-4 {
				t.Errorf("digit %q leaked at position %d of %q", c, i, got)
			}
		}
	}
}

func TestHashOperator(t *testing.T) {
	o := NewHashOperator(16)
	got, err := o.Replace(context.Background(), "john@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 16 {
		t.Errorf("got length %d, want 16", len(got))
	}
	if got == "john@example.com" {
		t.Errorf("hash returned the input unchanged")
	}
}

func TestHashOperatorFullDigest(t *testing.T) {
	o := NewHashOperator(0) // 0 means no truncation
	got, err := o.Replace(context.Background(), "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 64 {
		t.Errorf("got length %d, want the full 64-char SHA-256 hex digest", len(got))
	}
}

// The same value must hash identically so joins across a masked dataset still
// match; different values must not collide.
func TestHashOperatorDeterministicAndDistinct(t *testing.T) {
	o := NewHashOperator(0)
	ctx := context.Background()

	a1, _ := o.Replace(ctx, "same")
	a2, _ := o.Replace(ctx, "same")
	if a1 != a2 {
		t.Errorf("hash is not deterministic: %q != %q", a1, a2)
	}

	b, _ := o.Replace(ctx, "different")
	if a1 == b {
		t.Errorf("distinct values collided on %q", a1)
	}
}

func TestHashOperatorEmptyValue(t *testing.T) {
	o := NewHashOperator(8)
	got, err := o.Replace(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 8 {
		t.Errorf("got length %d, want 8", len(got))
	}
}

func TestRedactOperator(t *testing.T) {
	o := NewRedactOperator()
	got, err := o.Replace(context.Background(), "123-45-6789")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "[REDACTED]" {
		t.Errorf("got %q, want [REDACTED]", got)
	}
}

func TestRedactOperatorRemovesEveryCharacter(t *testing.T) {
	o := NewRedactOperator()
	const secret = "sk-live-abcdef123456"
	got, err := o.Replace(context.Background(), secret)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == secret || got == "" {
		t.Errorf("redaction returned %q, want a non-empty marker distinct from the input", got)
	}
}

func TestOperatorNames(t *testing.T) {
	tests := map[string]Operator{
		"replace": NewReplaceOperator("<X>"),
		"mask":    NewMaskOperator("***"),
		"hash":    NewHashOperator(8),
		"redact":  NewRedactOperator(),
	}
	for want, op := range tests {
		if op.Name() != want {
			t.Errorf("Name() = %q, want %q", op.Name(), want)
		}
	}
}
