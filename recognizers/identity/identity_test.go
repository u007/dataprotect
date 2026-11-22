package identity

import (
	"context"
	"testing"

	"github.com/u007/dataprotect/recognizers"
)

// run returns the values r reports over text, checking that each span is the
// reported text and that the confidence is the keyword score.
func run(t *testing.T, r recognizers.Recognizer, text string) []string {
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
	return got
}

func checkRun(t *testing.T, r recognizers.Recognizer, text string, want []string) {
	t.Helper()
	got := run(t, r, text)
	if len(got) != len(want) {
		t.Fatalf("%q: got %q, want %q", text, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%q: got %q, want %q", text, got, want)
		}
	}
}

// Positives: each shape the corpus labels, under the keyword, reports the value.
func TestIdentityRulesKeywordGateAndSpan(t *testing.T) {
	cases := []struct {
		name string
		r    recognizers.Recognizer
		text string
		want []string
	}{
		// Tax id. EIN prefix 12 is valid; P-prefix form and SSN form come from the nemotron labels.
		{"tax id EIN", NewTaxIDRecognizer(), "My tax ID is 43-5478236 for filing.", []string{"43-5478236"}},
		{"employer identification number", NewTaxIDRecognizer(), "employer identification number 12-3456789 applies", []string{"12-3456789"}},
		{"tax id letter form", NewTaxIDRecognizer(), "The tax id is P12345678 per the registry.", []string{"P12345678"}},
		{"TIN label, SSN form", NewTaxIDRecognizer(), "our tax identification number is 937-48-2658", []string{"937-48-2658"}},
		{"taxpayer id label", NewTaxIDRecognizer(), "Taxpayer id: 45-1234567", []string{"45-1234567"}},
		// Passport. Nine digits and one letter with eight digits, from the gretel labels.
		{"passport number nine digits", NewPassportRecognizer(), "Passport Number: 322996077 is shown.", []string{"322996077"}},
		{"passport no letter form", NewPassportRecognizer(), "Passport no. Z12345678 was scanned.", []string{"Z12345678"}},
		// Driver licence. Gretel shapes: 99-999999-99, A99999999, AA99999999, A999-9999-999-9.
		{"driver's license number", NewDriverLicenseRecognizer(), "Driver's license number: D42533932 checked.", []string{"D42533932"}},
		{"driver license hyphenated", NewDriverLicenseRecognizer(), "driver license 34-438489-77 is on file", []string{"34-438489-77"}},
		{"driving licence letters", NewDriverLicenseRecognizer(), "Driving licence GH34886022 is valid.", []string{"GH34886022"}},
		{"driver license four groups", NewDriverLicenseRecognizer(), "driver's license number is X948-1557-273-3", []string{"X948-1557-273-3"}},
		{"DL number", NewDriverLicenseRecognizer(), "DL number: 75-094116-03 renewed", []string{"75-094116-03"}},
		// Date of birth. Only the date is reported.
		{"date of birth ISO", NewDateOfBirthRecognizer(), "Date of birth: 1984-03-02, next of kin.", []string{"1984-03-02"}},
		{"DOB slash date", NewDateOfBirthRecognizer(), "DOB: 01/01/1980 on the form", []string{"01/01/1980"}},
		{"born on month name", NewDateOfBirthRecognizer(), "She was born on 17 July 1990 in Leeds.", []string{"17 July 1990"}},
		{"birth date month day", NewDateOfBirthRecognizer(), "birth date January 1, 1950 recorded", []string{"January 1, 1950"}},
		{"JSON escaped key", NewDateOfBirthRecognizer(), `"date\_of\_birth": "1974-08-17"`, []string{"1974-08-17"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkRun(t, tc.r, tc.text, tc.want)
		})
	}
}

// Negatives: a value with no label, a label whose value has the wrong shape,
// an invalid EIN prefix, a non-licence label, and a non-birth date label must
// all be silent.
func TestIdentityRulesNegatives(t *testing.T) {
	cases := []struct {
		name string
		r    recognizers.Recognizer
		text string
	}{
		{"tax id invalid EIN prefix 00", NewTaxIDRecognizer(), "Tax ID: 00-1234567 was rejected."},
		{"tax id EIN prefix 07 not assigned", NewTaxIDRecognizer(), "Tax ID: 07-1234567 was rejected."},
		{"EIN-shaped with no label", NewTaxIDRecognizer(), "The reference 12-3456789 is in the appendix."},
		{"passport with no number", NewPassportRecognizer(), "Please bring your passport to the desk."},
		{"nine digits with no passport label", NewPassportRecognizer(), "Ticket 512345678 was booked."},
		{"licence shape under policy number", NewDriverLicenseRecognizer(), "Your policy number is 34-438489-77 for the claim."},
		{"licence label, value too short", NewDriverLicenseRecognizer(), "Driver's license number: AB12 is wrong."},
		{"date with no birth label", NewDateOfBirthRecognizer(), "The invoice was issued on 1984-03-02."},
		{"effective date is not a birth date", NewDateOfBirthRecognizer(), "Effective date: 1984-03-02 for the renewal."},
		{"placeholder is not a date", NewDateOfBirthRecognizer(), "Date of birth: MM/DD/YYYY"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := run(t, tc.r, tc.text); len(got) != 0 {
				t.Errorf("%q: want no detections, got %q", tc.text, got)
			}
		})
	}
}

// The corpus gives the EIN prefixes the IRS does not assign as a negative
// check: every listed prefix is accepted and each unassigned one is rejected.
func TestTaxIDEINPrefixesMatchIRSList(t *testing.T) {
	valid := []string{"01", "06", "10", "16", "20", "27", "30", "39", "40", "48", "50", "59", "60", "68", "71", "77", "80", "88", "90", "95", "98", "99"}
	invalid := []string{"00", "07", "08", "09", "17", "18", "19", "28", "29", "49", "69", "70", "78", "79", "89", "96", "97"}
	for _, p := range valid {
		text := "Tax ID: " + p + "-1234567 end"
		checkRun(t, NewTaxIDRecognizer(), text, []string{p + "-1234567"})
	}
	for _, p := range invalid {
		if got := run(t, NewTaxIDRecognizer(), "Tax ID: "+p+"-1234567 end"); len(got) != 0 {
			t.Errorf("prefix %s is not assigned by the IRS but was reported: %q", p, got)
		}
	}
}
