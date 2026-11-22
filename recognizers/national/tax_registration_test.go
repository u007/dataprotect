package national

import (
	"context"
	"testing"

	"github.com/u007/dataprotect/recognizers"
)

// runKeyword runs a keyword-gated national rule over text and checks that it
// reports exactly want, in order, each as the value alone at the keyword score.
func runKeyword(t *testing.T, r recognizers.Recognizer, text string, want []string) {
	t.Helper()
	ents, err := r.Recognize(context.Background(), text)
	if err != nil {
		t.Fatalf("Recognize(%q): %v", text, err)
	}
	var got []string
	for _, e := range ents {
		if text[e.Start:e.End] != e.Text {
			t.Errorf("%q: span [%d:%d] is %q but Text is %q", text, e.Start, e.End, text[e.Start:e.End], e.Text)
		}
		if e.Confidence != keywordScore {
			t.Errorf("%q: confidence %v, want %v", text, e.Confidence, keywordScore)
		}
		got = append(got, e.Text)
	}
	if len(got) != len(want) {
		t.Fatalf("%q: got %q, want %q", text, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%q: got %q, want %q", text, got, want)
		}
	}
}

// IN_PAN: https://en.wikipedia.org/wiki/Permanent_account_number (fetched
// 2026-11-22). The example AAAPZ1234C has P as its holder type. The other values
// are synthetic.
func TestINPANRecognize(t *testing.T) {
	r := NewINPANRecognizer()
	runKeyword(t, r, "PAN AAAPZ1234C", []string{"AAAPZ1234C"})
	runKeyword(t, r, "PAN no: AAAPZ1234C on file", []string{"AAAPZ1234C"})
	runKeyword(t, r, "permanent account number is ZKLTA5432G", []string{"ZKLTA5432G"})
	runKeyword(t, r, "pan: BNRPQ2468K", []string{"BNRPQ2468K"})

	// Wrong holder type: the fourth character must be one of A B C F G H L J P T.
	runKeyword(t, r, "PAN BNRXQ2468K", nil)
	// No label.
	runKeyword(t, r, "card BNRPQ2468K printed", nil)
	// Wrong shape: the fifth character must be a letter, the sixth to ninth digits.
	runKeyword(t, r, "PAN BNRPQ246K8", nil)
	// A GSTIN carries a PAN at characters 3 to 12, but no PAN label precedes it.
	runKeyword(t, r, "GSTIN 27AAAPZ1234C1Z5", nil)
}

// SG_UEN: OECD, Singapore TIN information, Section II (fetched 2026-11-22). All
// values are synthetic.
func TestSGUENRecognize(t *testing.T) {
	r := NewSGUENRecognizer()
	runKeyword(t, r, "UEN 53412378W", []string{"53412378W"})                         // ROB, 9 characters
	runKeyword(t, r, "unique entity number 201912345M", []string{"201912345M"})      // ROC, 10 characters
	runKeyword(t, r, "business registration no. T19LP5678H", []string{"T19LP5678H"}) // Others, T form
	runKeyword(t, r, "co. reg. no 201912345M is ours", []string{"201912345M"})       // ROC
	runKeyword(t, r, "company registration no S18FC1234Z", []string{"S18FC1234Z"})   // Others, S form
	runKeyword(t, r, "UEN 53412378 without a letter", nil)                           // no check letter
	runKeyword(t, r, "53412378W printed with no label", nil)                         // no label
	runKeyword(t, r, "UEN 181212345M", nil)                                          // year must start 19 or 20
	runKeyword(t, r, "UEN S1234567A", nil)                                           // NRIC shape, not a UEN
}

// MY_SSM: SSM announcement, ID=134 (fetched 2026-11-22). The example
// 201901000005 and its old number 1312525-A are the published ones; the other
// values are synthetic.
func TestMYSSMRecognize(t *testing.T) {
	r := NewMYSSMRecognizer()
	runKeyword(t, r, "Registration No: 201901000005", []string{"201901000005"})
	// The old number in parentheses has no label of its own, so the gate does
	// not report it; only the labelled new number is reported.
	runKeyword(t, r, "Registration No: 201901000005 (1312525-A)", []string{"201901000005"})
	runKeyword(t, r, "company no 202001234567", []string{"202001234567"})
	runKeyword(t, r, "company registration no 201801045623", []string{"201801045623"})
	runKeyword(t, r, "no. syarikat 1234567-K", []string{"1234567-K"})
	runKeyword(t, r, "SSM 1312525-A", []string{"1312525-A"})
	runKeyword(t, r, "ROC 202001234567", []string{"202001234567"})

	// Eleven digits is not the 12-digit form.
	runKeyword(t, r, "registration no 20200123456", nil)
	// No label: a 12-digit run and an old-form number are shape alone.
	runKeyword(t, r, "order 202001234567 shipped", nil)
	runKeyword(t, r, "1312525-A printed alone", nil)
}

// MY_TIN: IRBM / OECD Malaysia TIN information, March 2023, Section II (fetched
// 2026-11-22). The formats are the ones the source shows; the values are
// synthetic.
func TestMYTINRecognize(t *testing.T) {
	r := NewMYTINRecognizer()
	runKeyword(t, r, "TIN IG4455667788", []string{"IG4455667788"})                     // IG + 10 digits
	runKeyword(t, r, "TIN: IG 115002000", []string{"IG 115002000"})                    // IG + space + 9 digits
	runKeyword(t, r, "nombor pengenalan cukai C30770061120", []string{"C30770061120"}) // C + 11 digits
	runKeyword(t, r, "no. cukai D4800991122", []string{"D4800991122"})                 // D + 10 digits
	runKeyword(t, r, "income tax no E12345678901", []string{"E12345678901"})           // E + 11 digits
	runKeyword(t, r, "tax identification number F1023456709", []string{"F1023456709"}) // F + 10 digits
	runKeyword(t, r, "no. cukai pendapatan LE123456789", []string{"LE123456789"})      // LE + 9 digits

	// Eight digits is below the 9-to-11 range.
	runKeyword(t, r, "TIN IG12345678", nil)
	// The old SG prefix is not accepted; the source says it was converted to IG.
	runKeyword(t, r, "TIN SG123456789", nil)
	// G is not a code the source lists.
	runKeyword(t, r, "TIN G1234567890", nil)
	// No label.
	runKeyword(t, r, "IG123456789 printed alone", nil)
}
