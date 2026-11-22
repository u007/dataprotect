package legal

import (
	"context"
	"testing"

	"github.com/u007/dataprotect/recognizers"
)

func caseValues(t *testing.T, text string) []string {
	t.Helper()
	ents, err := NewCaseNumberRecognizer().Recognize(context.Background(), text)
	if err != nil {
		t.Fatalf("Recognize(%q): %v", text, err)
	}
	var out []string
	for _, e := range ents {
		if e.Type != recognizers.EntityCaseNumber {
			t.Errorf("%q: type %s", text, e.Type)
		}
		if text[e.Start:e.End] != e.Text {
			t.Errorf("%q: span %q but Text %q", text, text[e.Start:e.End], e.Text)
		}
		out = append(out, e.Text)
	}
	return out
}

func only(t *testing.T, text, want string) {
	t.Helper()
	got := caseValues(t, text)
	if len(got) != 1 || got[0] != want {
		t.Errorf("%q: got %q, want [%q]", text, got, want)
	}
}

func TestUSFederalShapes(t *testing.T) {
	for text, want := range map[string]string{
		"Filed as 23-CV-0001 in Tulsa.":            "23-CV-0001",
		"Case 23-CV-0001-JFH pending.":             "23-CV-0001-JFH",
		"Matter 23-CV-0001-JFH-JFJ was reassigned": "23-CV-0001-JFH-JFJ",
		"See 99-CV-0001-RAW-KEW.":                  "99-CV-0001-RAW-KEW",
		"Docket 3:00CV0001-JFA-PJG":                "3:00CV0001-JFA-PJG",
		"Charge 3:00CR0001-01 unsealed":            "3:00CR0001-01",
		"Plain 3:00CV0001 here":                    "3:00CV0001",
		"Ref 22-mc-123 lower case":                 "22-mc-123",
	} {
		only(t, text, want)
	}
}

func TestSingaporeShapes(t *testing.T) {
	for text, want := range map[string]string{
		"Application CA/CA 1/2021 is heard.": "CA/CA 1/2021",
		"Appeal AD/CA 12/2020 in part":       "AD/CA 12/2020",
		"Summons CA/SUM 3/2022 filed":        "CA/SUM 3/2022",
		"Originating HC/OA 77/2019 served":   "HC/OA 77/2019",
		"Writ HC/OC 5/2022 issued":           "HC/OC 5/2022",
		"Summons HC / SUM 10 / 2022 listed":  "HC / SUM 10 / 2022",
		"State DC / OC 1 / 2022 was filed":   "DC / OC 1 / 2022",
		"Small MC / SUM 1 / 2022 claim":      "MC / SUM 1 / 2022",
		"Tribunal SCT/10001/2022 decided":    "SCT/10001/2022",
		"Tribunal ECT/10001/2022 decided":    "ECT/10001/2022",
		"Tribunal CDT/101/2022 decided":      "CDT/101/2022",
		"Tribunal PHC/10001/2022 decided":    "PHC/10001/2022",
	} {
		only(t, text, want)
	}
}

func TestLabelledGenericForms(t *testing.T) {
	for text, want := range map[string]string{
		"Case No. 2023-PI-1234 was filed":       "2023-PI-1234",
		"Suit No: 1234/2019 was filed":          "1234/2019",
		"Civil Suit No 45 of 2020 was filed":    "45 of 2020",
		"Docket no. ABC-123 is open":            "ABC-123",
		"Guaman No. 12-7/2021 dikemukakan":      "12-7/2021",
		"Guaman Sivil No: 3(A)/2020 ada":        "3(A)/2020",
		"Saman Pemula No 22-44-2019 ditetapkan": "22-44-2019",
		"Civil Appeal No: 01-1-2015 pending":    "01-1-2015",
		"Criminal Appeal No. CA12-5 heard":      "CA12-5",
	} {
		only(t, text, want)
	}
}

func TestMalaysianJudgmentFormsMatchedInFull(t *testing.T) {
	for text, want := range map[string]string{
		"Case No: WA-22NCC-591-11 OF 2022": "WA-22NCC-591-11 OF 2022",
		"Case No: WA-22NCC-459-09 OF 2020": "WA-22NCC-459-09 OF 2020",
		"Case No: WA-24NCC-131-03/2018":    "WA-24NCC-131-03/2018",
		"Case No: WA-25-560-12 OF 2019":    "WA-25-560-12 OF 2019",
	} {
		only(t, text, want)
	}
}

func TestLabelReportsValueOnly(t *testing.T) {
	got := caseValues(t, "Suit No. 9/2018")
	if len(got) != 1 || got[0] != "9/2018" {
		t.Fatalf("got %q", got)
	}
	// A space inside the value ends the token, so this is not a case number.
	if got := caseValues(t, "Suit No: S 1234/2019"); len(got) != 0 {
		t.Errorf("spaced value: got %q, want none", got)
	}
}

func TestNeutralCitationsAreNotCaseNumbers(t *testing.T) {
	for _, text := range []string{
		"See [2021] SGHC 123 for the test.",
		"Cited as [2021] SGCA 5 at [14].",
		"Reported at [2020] 1 MLJ 123 (FC).",
		"Compare [2019] SGDC 42 and [2018] 2 SLR 7.",
		"Section 23 of the Act and page 45 of 2021 report.",
		"Version 23-cv and 2023 cvs plus order 99 of 2020.",
		"Phone 2021 and total of 12 items",
	} {
		if got := caseValues(t, text); len(got) != 0 {
			t.Errorf("%q: got %q, want none", text, got)
		}
	}
}

func TestBareNumberWithoutLabelIsNotCaseNumber(t *testing.T) {
	for _, text := range []string{"reference 0001 of 2022", "ticket WA-22NCC-591-11 OF 2022", "code 1234/2020"} {
		if got := caseValues(t, text); len(got) != 0 {
			t.Errorf("%q: got %q, want none", text, got)
		}
	}
}

func TestShapeScoreAndKeywordScore(t *testing.T) {
	ents, _ := NewCaseNumberRecognizer().Recognize(context.Background(), "Case 23-CV-0001 and Suit No: 77/2020")
	if len(ents) != 2 {
		t.Fatalf("got %d", len(ents))
	}
	if ents[0].Confidence != ShapeScore || ents[1].Confidence != KeywordScore {
		t.Errorf("scores %v %v", ents[0].Confidence, ents[1].Confidence)
	}
}
