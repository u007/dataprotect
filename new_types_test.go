package piiscrub

import (
	"context"
	"testing"
)

// spansOf runs the default scrubber over text and returns each reported entity
// as type and value, in text order.
func spansOf(t *testing.T, text string) []string {
	t.Helper()
	ents, err := NewScrubber().Analyze(context.Background(), text)
	if err != nil {
		t.Fatalf("Analyze(%q): %v", text, err)
	}
	var out []string
	for _, e := range ents {
		out = append(out, string(e.Type)+" "+e.Text)
	}
	return out
}

func wantSpans(t *testing.T, text string, want ...string) {
	t.Helper()
	got := spansOf(t, text)
	if len(got) != len(want) {
		t.Errorf("%q: got %q, want %q", text, got, want)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%q: got %q, want %q", text, got, want)
			return
		}
	}
}

// The seven batch 3a types are on by default, and each reports its value alone.
func TestBatch3aTypesOnByDefault(t *testing.T) {
	wantSpans(t, "Entity LEI 54930084UKLVMY22DS16 is on file.", "LEI 54930084UKLVMY22DS16")
	wantSpans(t, "ISIN US0378331005 is listed.", "ISIN US0378331005")
	wantSpans(t, "PAN AAAPZ1234C on the form.", "IN_PAN AAAPZ1234C")
	wantSpans(t, "wallet 0x1a2b3c4d5e6f70819a2b3c4d5e6f70819a2b3c4d", "ETHEREUM 0x1a2b3c4d5e6f70819a2b3c4d5e6f70819a2b3c4d")
	wantSpans(t, "UEN 53412378W for the supplier.", "SG_UEN 53412378W")
	wantSpans(t, "company no 202001234567 registered.", "MY_SSM 202001234567")
	wantSpans(t, "TIN IG4455667788 for the return.", "MY_TIN IG4455667788")
}

// The Ethereum address is one value. SECRET does not claim it in the contexts
// below, where no secret label is present. A "token" or "password" label would
// make SECRET a legitimate match, so those are not used here.
func TestEthereumNotClaimedBySecret(t *testing.T) {
	const addr = "0x1a2B3C4d5e6F70819a2B3C4d5E6F70819a2B3C4D"
	for _, text := range []string{
		"Send funds to " + addr + " today.",
		"address=" + addr,
		"wallet: " + addr,
		"(" + addr + ")",
	} {
		wantSpans(t, text, "ETHEREUM "+addr)
	}
}

// A labelled 12-digit run is an SSM number, not a MyKad. The bare run is a
// weak MyKad candidate that is dropped without a judge.
func TestMYSSMNotLostToMYIC(t *testing.T) {
	wantSpans(t, "company no 202001234567", "MY_SSM 202001234567")
	wantSpans(t, "company registration no 201901000005 is on file", "MY_SSM 201901000005")
	wantSpans(t, "order 202001234567 shipped")
}

// UEN and NRIC do not collide. An NRIC is S, T, F or G then seven digits and a
// check letter; a UEN ends in a letter after eight digits, or has the other
// forms. Both are tested in both directions.
func TestSGUENAndNRICDoNotCollide(t *testing.T) {
	wantSpans(t, "UEN 53412378W is ours", "SG_UEN 53412378W")
	wantSpans(t, "NRIC S1234567D is valid", "SG_NRIC S1234567D")
	// A valid NRIC under a UEN label is still an NRIC, not a UEN.
	wantSpans(t, "UEN S1234567D is ours", "SG_NRIC S1234567D")
	// A company registration label is the UEN's, and the MY_SSM label is not
	// taken by an SG UEN value.
	wantSpans(t, "company registration no 201912345M is ours", "SG_UEN 201912345M")
	wantSpans(t, "company registration no 201901000005 is ours", "MY_SSM 201901000005")
}

// A GSTIN carries a PAN at characters 3 to 12. The PAN does not match inside it:
// there is no word boundary between the state code and the PAN, and no PAN label
// comes before it. A GSTIN with a valid check digit is IN_GSTIN alone.
func TestINPANNotInsideINGSTIN(t *testing.T) {
	wantSpans(t, "GSTIN 27AAAPZ1234C1Z0 for the vendor", "IN_GSTIN 27AAAPZ1234C1Z0")
	wantSpans(t, "PAN AAAPZ1234C and GSTIN 27AAAPZ1234C1Z0 are on file",
		"IN_PAN AAAPZ1234C", "IN_GSTIN 27AAAPZ1234C1Z0")
}

// MY_TIN takes the TIN label, so TAX_ID does not report the same value.
func TestMYTINNotLostToTaxID(t *testing.T) {
	wantSpans(t, "TIN E12345678901 for the company", "MY_TIN E12345678901")
	wantSpans(t, "TIN: IG 115002000 on file", "MY_TIN IG 115002000")
}

// The rules outside the batch still hold: a labelled SSN-shaped tax id is SSN.
func TestBatch3aDoesNotDisturbTaxIDAndSSN(t *testing.T) {
	wantSpans(t, "tax id 123-45-6789", "SSN 123-45-6789")
}
