package national

import (
	"context"
	"testing"

	"dataprotect/recognizers"
)

func TestSGNRICValidate(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{"sample S prefix", "S1234567D", true},
		{"T prefix", "T0123456G", true},
		{"F prefix", "F5555555U", true},
		{"G prefix", "G7654321L", true},
		{"wrong check letter", "S1234567A", false},
		{"wrong prefix for check table", "S1234567U", false},
		{"unsupported M prefix", "M1234567K", false},
		{"too few digits", "S123456D", false},
	}
	v := NewSGNRICValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := v.Validate(tt.id); got != tt.want {
				t.Errorf("Validate(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestSGNRICRecognizeInProse(t *testing.T) {
	r := NewSGNRICRecognizer()
	text := "Applicant NRIC S1234567D is on file, and the FIN G7654321L was checked."
	got, err := r.Recognize(context.Background(), text)
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}
	want := []string{"S1234567D", "G7654321L"}
	if len(got) != len(want) {
		t.Fatalf("got %d entities, want %d: %+v", len(got), len(want), got)
	}
	for i, e := range got {
		if e.Type != recognizers.EntitySGNRIC {
			t.Errorf("entity %d type = %s, want SG_NRIC", i, e.Type)
		}
		if e.Text != want[i] {
			t.Errorf("entity %d Text = %q, want %q", i, e.Text, want[i])
		}
		if text[e.Start:e.End] != e.Text {
			t.Errorf("entity %d span [%d:%d] does not match Text", i, e.Start, e.End)
		}
	}
}

func TestSGNRICRecognizeRejectsInvalidAndEmbedded(t *testing.T) {
	r := NewSGNRICRecognizer()
	// A bad check letter is not reported, and a longer alphanumeric token that
	// merely contains a valid-looking run is not an NRIC.
	for _, text := range []string{"ref S1234567A here", "code XS1234567D99 here"} {
		got, err := r.Recognize(context.Background(), text)
		if err != nil {
			t.Fatalf("Recognize(%q): %v", text, err)
		}
		if len(got) != 0 {
			t.Errorf("Recognize(%q) = %+v, want none", text, got)
		}
	}
}

func TestMYICValidate(t *testing.T) {
	tests := []struct {
		name string
		ic   string
		want bool
	}{
		{"valid 1963 birth", "630916-12-3456", true},
		{"valid 2000 birth", "000229-10-1234", true},
		{"month 13", "631316-12-3456", false},
		{"day 31 in February", "630231-12-3456", false},
		{"day 0", "630900-12-3456", false},
		{"no hyphens", "630916123456", false},
	}
	v := NewMYICValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := v.Validate(tt.ic); got != tt.want {
				t.Errorf("Validate(%q) = %v, want %v", tt.ic, got, tt.want)
			}
		})
	}
}

func TestMYICRecognizeInProse(t *testing.T) {
	r := NewMYICRecognizer()
	text := "Her MyKad 630916-12-3456 was copied; 631316-12-3456 is not a real date."
	got, err := r.Recognize(context.Background(), text)
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}
	if len(got) != 1 || got[0].Text != "630916-12-3456" || got[0].Type != recognizers.EntityMYIC {
		t.Fatalf("got %+v, want one MY_IC 630916-12-3456", got)
	}
}

func TestTHIDValidate(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{"valid bare", "3100600445635", true},
		{"valid hyphenated", "3-1006-00445-63-5", true},
		{"valid second", "1-2345-45678-78-1", true},
		{"wrong check digit", "3100600445636", false},
		{"twelve digits", "310060044563", false},
		{"letters inside", "3-1006-00445-63-X", false},
	}
	v := NewTHIDValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := v.Validate(tt.id); got != tt.want {
				t.Errorf("Validate(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestTHIDRecognize(t *testing.T) {
	r := NewTHIDRecognizer()
	text := "Citizen ID 3-1006-00445-63-5 on the form, order 3100600445636 is not valid."
	got, err := r.Recognize(context.Background(), text)
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}
	if len(got) != 1 || got[0].Text != "3-1006-00445-63-5" || got[0].Type != recognizers.EntityTHID {
		t.Fatalf("got %+v, want one TH_ID 3-1006-00445-63-5", got)
	}
}

func TestMYPassportRecognizeWithKeyword(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{"passport number label", "Passport Number: A12345678 on file.", "A12345678"},
		{"passport no label", "passport no A12345678 renewed", "A12345678"},
		{"run-together no label", "PASSPORTNO: H87654321", "H87654321"},
		{"malay label", "Nombor pasport K11223344 disahkan.", "K11223344"},
		{"lowercase letter", "No. Pasport: a12345678", "a12345678"},
		{"hash separator", "passport #A12345678", "A12345678"},
		{"underscore from a column name", "passport_number: A12345678", "A12345678"},
		{"quoted json key", "\"passport\": \"A12345678\"", "A12345678"},
		{"line break after label", "Passport No:\nA12345678", "A12345678"},
		{"malay keyword run together", "nomborpasport A12345678", "A12345678"},
		{"dash separator", "Passport No - A12345678", "A12345678"},
		{"en dash separator", "Passport No – A12345678", "A12345678"},
		{"non-breaking space", "Passport No: A12345678", "A12345678"},
		{"fullwidth colon", "Passport No：A12345678", "A12345678"},
		{"markdown table cell", "| Passport No | A12345678 |", "A12345678"},
		{"query string equals", "passport=A12345678", "A12345678"},
		{"underscored query key", "passport_no=A12345678", "A12345678"},
		{"xml tag", "<passport>A12345678</passport>", "A12345678"},
		{"hyphen in label", "Passport-No: A12345678", "A12345678"},
		{"prefixed word", "ePassport No: A12345678", "A12345678"},
	}
	r := NewMYPassportRecognizer()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.Recognize(context.Background(), tt.text)
			if err != nil {
				t.Fatalf("Recognize: %v", err)
			}
			if len(got) != 1 || got[0].Text != tt.want || got[0].Type != recognizers.EntityMYPassport {
				t.Fatalf("got %+v, want one MY_PASSPORT %q", got, tt.want)
			}
			if tt.text[got[0].Start:got[0].End] != tt.want {
				t.Errorf("span [%d:%d] does not cover the number %q", got[0].Start, got[0].End, tt.want)
			}
		})
	}
}

func TestMYPassportRecognizeRejectsWithoutKeywordOrShape(t *testing.T) {
	// A bare number has no keyword. Wrong letter, wrong digit count, a letter run
	// into the number, and a longer word all fail the shape or the boundaries.
	texts := []string{
		"ref A12345678 on the form",
		"Passport Number: B12345678",
		"Passport Number: A1234567",
		"Passport Number: A123456789",
		"Passport Number: XA12345678",
		"passports A12345678 were checked",
		"passportA12345678",
	}
	r := NewMYPassportRecognizer()
	for _, text := range texts {
		got, err := r.Recognize(context.Background(), text)
		if err != nil {
			t.Fatalf("Recognize(%q): %v", text, err)
		}
		if len(got) != 0 {
			t.Errorf("Recognize(%q) = %+v, want none", text, got)
		}
	}
}

func TestSGPassportRecognizeWithKeyword(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{"passport number label", "Passport Number: E1234567A on file.", "E1234567A"},
		{"passport no label", "passport no K7654321B renewed", "K7654321B"},
		{"run-together no label", "PASSPORTNO: E1234567Z", "E1234567Z"},
		{"lowercase letters", "No. Pasport: e1234567a", "e1234567a"},
		{"line break after label", "Passport No:\nE1234567A", "E1234567A"},
		{"prefixed word", "ePassport No: E1234567A", "E1234567A"},
		{"markdown table cell", "| Passport No | E1234567A |", "E1234567A"},
	}
	r := NewSGPassportRecognizer()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.Recognize(context.Background(), tt.text)
			if err != nil {
				t.Fatalf("Recognize: %v", err)
			}
			if len(got) != 1 || got[0].Text != tt.want || got[0].Type != recognizers.EntitySGPassport {
				t.Fatalf("got %+v, want one SG_PASSPORT %q", got, tt.want)
			}
			if tt.text[got[0].Start:got[0].End] != tt.want {
				t.Errorf("span [%d:%d] does not cover the number %q", got[0].Start, got[0].End, tt.want)
			}
		})
	}
}

func TestSGPassportRecognizeRejectsWithoutKeywordOrShape(t *testing.T) {
	// A bare number has no keyword. Wrong first letter, wrong digit count, a
	// missing final letter, a trailing letter run, and a letter run into the
	// number all fail the shape or the boundaries.
	texts := []string{
		"ref E1234567A on the form",
		"Passport Number: B1234567A",
		"Passport Number: E123456A",
		"Passport Number: E12345678",
		"Passport Number: E1234567AB",
		"Passport Number: XE1234567A",
		"passports E1234567A were checked",
		"passportE1234567A",
	}
	r := NewSGPassportRecognizer()
	for _, text := range texts {
		got, err := r.Recognize(context.Background(), text)
		if err != nil {
			t.Fatalf("Recognize(%q): %v", text, err)
		}
		if len(got) != 0 {
			t.Errorf("Recognize(%q) = %+v, want none", text, got)
		}
	}
}

func TestSGNRICRecognizeGluedPrefix(t *testing.T) {
	r := NewSGNRICRecognizer()
	got, err := r.Recognize(context.Background(), "NRICS1234567D on file")
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}
	if len(got) != 1 || got[0].Text != "S1234567D" {
		t.Fatalf("got %+v, want one SG_NRIC S1234567D", got)
	}
}

func TestMYICRecognizeBareWithKeyword(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{"mykad label", "MyKad 900515103421 on file", "900515103421"},
		{"ic no label", "IC no: 630916123456", "630916123456"},
		{"kp label", "KP 000229081234 checked", "000229081234"},
	}
	r := NewMYICRecognizer()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.Recognize(context.Background(), tt.text)
			if err != nil {
				t.Fatalf("Recognize: %v", err)
			}
			if len(got) != 1 || got[0].Text != tt.want || got[0].Type != recognizers.EntityMYIC {
				t.Fatalf("got %+v, want one MY_IC %q", got, tt.want)
			}
			if tt.text[got[0].Start:got[0].End] != tt.want {
				t.Errorf("span [%d:%d] does not cover the digits %q", got[0].Start, got[0].End, tt.want)
			}
		})
	}
}

func TestMYICRecognizeBareRejected(t *testing.T) {
	// No keyword, a keyword inside a word, a bad date under a keyword, and a
	// 13-digit run are all out.
	texts := []string{
		"ref 900515103421 on the form",
		"tic 900515103421 on the form",
		"MyKad 900231103421 on file",
		"MyKad 9005151034211 on file",
	}
	r := NewMYICRecognizer()
	for _, text := range texts {
		got, err := r.Recognize(context.Background(), text)
		if err != nil {
			t.Fatalf("Recognize(%q): %v", text, err)
		}
		if len(got) != 0 {
			t.Errorf("Recognize(%q) = %+v, want none", text, got)
		}
	}
}

func TestSGNRICRecognizeNotInsideLongerToken(t *testing.T) {
	// A check-valid NRIC at the end of an alphanumeric run, with no label before
	// it, must not be reported. Reporting it would outrank a shape-only bitcoin
	// span and leave the rest of the token visible.
	r := NewSGNRICRecognizer()
	got, err := r.Recognize(context.Background(), "send 1Ab3CdEf5Gh7Jk9Mn2Pq4RsS1234567D now")
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %+v, want none", got)
	}
}

func TestSGNRICRecognizeLabelledGlued(t *testing.T) {
	r := NewSGNRICRecognizer()
	got, err := r.Recognize(context.Background(), "FINF1234567N and NRICS1234567D")
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}
	want := []string{"F1234567N", "S1234567D"}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %v", got, want)
	}
	for i, e := range got {
		if e.Text != want[i] {
			t.Errorf("entity %d Text = %q, want %q", i, e.Text, want[i])
		}
	}
}
