package piiscrub

import (
	"context"
	"reflect"
	"testing"

	"github.com/u007/dataprotect/recognizers"
)

// "account number 0012345678" is matched by the BANK_ACCOUNT rule (0.85) and,
// as a bare ten-digit run, by the PHONE rule (0.98). The finance rule wins the
// overlap, so PHONE only shows through when BANK_ACCOUNT is switched off.
const bankAccountText = "Please wire funds to account number 0012345678 at the branch."

func typesIn(t *testing.T, s *Scrubber, text string) map[recognizers.EntityType]int {
	t.Helper()
	ents, err := s.Analyze(context.Background(), text)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	out := map[recognizers.EntityType]int{}
	for _, e := range ents {
		out[e.Type]++
	}
	return out
}

func TestWithoutEntityTypesOmitsType(t *testing.T) {
	const text = "Mail ops@example.com or call 555-123-4567."
	if got := typesIn(t, NewScrubber(), text); got[recognizers.EntityEmail] != 1 {
		t.Fatalf("baseline: want one EMAIL, got %v", got)
	}
	got := typesIn(t, NewScrubber(WithoutEntityTypes(recognizers.EntityEmail)), text)
	if got[recognizers.EntityEmail] != 0 {
		t.Errorf("disabled EMAIL still reported: %v", got)
	}
	if got[recognizers.EntityPhone] != 1 {
		t.Errorf("other types must still be reported: %v", got)
	}
}

// Disabling BANK_ACCOUNT lets the PHONE match that it used to beat on overlap
// be reported, so the digits are still covered by a type.
func TestWithoutEntityTypesLetsOtherTypeWin(t *testing.T) {
	if got := typesIn(t, NewScrubber(), bankAccountText); got[recognizers.EntityBankAccount] != 1 || got[recognizers.EntityPhone] != 0 {
		t.Fatalf("baseline: want BANK_ACCOUNT and no PHONE, got %v", got)
	}
	got := typesIn(t, NewScrubber(WithoutEntityTypes(recognizers.EntityBankAccount)), bankAccountText)
	if got[recognizers.EntityBankAccount] != 0 {
		t.Errorf("disabled BANK_ACCOUNT still reported: %v", got)
	}
	if got[recognizers.EntityPhone] != 1 {
		t.Errorf("want PHONE on the value once BANK_ACCOUNT is off, got %v", got)
	}
}

// An empty option list changes nothing: the output is identical to a scrubber
// built with no option at all.
func TestWithoutEntityTypesDefaultUnchanged(t *testing.T) {
	text := "Card 4012-8888-8888-1881 CVV 123, PIN 4821, " + bankAccountText +
		" Routing number: 123456780. Email a@b.example, SSN 123-45-6789."
	plain, err := NewScrubber().Analyze(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	empty, err := NewScrubber(WithoutEntityTypes()).Analyze(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plain, empty) {
		t.Errorf("empty WithoutEntityTypes changed output:\nplain=%+v\nempty=%+v", plain, empty)
	}
	if len(plain) == 0 {
		t.Fatal("sample text produced no entities; the comparison is vacuous")
	}
}

// The new finance types are on by default and redact to typed markers.
func TestFinanceTypesOnByDefault(t *testing.T) {
	cases := map[string]recognizers.EntityType{
		"Routing number: 123456780 for the transfer.":   recognizers.EntityUSRouting,
		"The card security code is 582 for this order.": recognizers.EntityCardCVV,
		"Enter your PIN 4821 at the terminal.":          recognizers.EntityPIN,
		"Beneficiary BIC: CHASUS33XXX, attached.":       recognizers.EntitySWIFTBIC,
		"BBAN: SBPJ14329354627679 is the reference.":    recognizers.EntityBankAccount,
	}
	for text, want := range cases {
		if got := typesIn(t, NewScrubber(), text); got[want] != 1 {
			t.Errorf("%q: want one %s by default, got %v", text, want, got)
		}
	}
}

// An ITIN-shaped tax id (9XX-XX-XXXX) is matched by the SSN rule at 0.99, and SSN
// wins the overlap, so by default the span is reported as SSN. Switching SSN off
// lets the keyword-gated TAX_ID rule report the same span.
func TestWithoutSSNExposesTaxIDOnSSNShape(t *testing.T) {
	const text = "Tax ID: 912-70-3456 was given for the applicant."
	if got := typesIn(t, NewScrubber(), text); got[recognizers.EntitySSN] != 1 || got[recognizers.EntityTaxID] != 0 {
		t.Fatalf("baseline: want one SSN and no TAX_ID, got %v", got)
	}
	got := typesIn(t, NewScrubber(WithoutEntityTypes(recognizers.EntitySSN)), text)
	if got[recognizers.EntityTaxID] != 1 {
		t.Errorf("with SSN disabled want one TAX_ID, got %v", got)
	}
}
