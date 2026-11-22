// Package legal detects court case numbers. A case number names a court
// proceeding and is personal data when it is attached to a party. It is not a
// citation of a published judgment, so the neutral citations "[2021] SGHC 123"
// and "[2020] 1 MLJ 123" are never matched here.
//
// Sources, fetched 2026-11-22; SOURCES.md records the unverified items.
//
//   - US federal district courts, two-digit year, type CV/CR/MC/MD/MJ, sequence,
//     optional judge and magistrate initials:
//     https://www.oknd.uscourts.gov/case_assignment_and_numbering ("CV (Civil),
//     CR (Criminal), or MC (Miscellaneous)"; examples "23-CV-0001",
//     "23-CV-0001-JFH", "23-CV-0001-JFH-JFJ", "99-CV-0001-RAW-KEW");
//     https://www.oked.uscourts.gov/node/214 (the same numbering);
//     https://www.scd.uscourts.gov/Filing/casenum.asp (one-digit division, colon,
//     two-digit year, type, four-digit sequence, then hyphen and defendant number
//     or judge initials; examples "3:00CV0001-JFA-PJG", "3:00CR0001-01",
//     "3:00CV0001"); https://www.flmd.uscourts.gov/understanding-case-designation
//     (types cv, cr, mc, md, mj); and
//     https://www.ord.uscourts.gov/index.php/filing-and-forms/cm-ecf/user-manual/187-section-5-electronic-filing-protocols/2082-the-court-s-case-numbering-system
//     (division code before a colon, year, hyphen, two-letter type, sequence
//     starting at 1, judge initials appended).
//   - Singapore Supreme Court Practice Directions 2021, para 95:
//     https://epd2021-supremecourt.judiciary.gov.sg/part-11-conduct-of-court-proceedings
//     ("Description of Court/ Type of Application [Case number]/ Year filed").
//     Prefixes CA/CA, AD/CA, CA/OA, AD/OA, HC/OA, HC/OC, CA/SUM, AD/SUM, HC/SUM.
//   - Singapore State Courts Practice Directions 2021, para 26:
//     https://epd2021-statecourts.judiciary.gov.sg/part-iii-originating-processes-and-documents
//     Courts DC, MC, PHC; types OC, OA, SUM, EOA, EO (examples "DC / OC 1 / 2022",
//     "MC / SUM 1 / 2022"); tribunal forms "SCT/10001/2022", "ECT/10001/2022",
//     "CDT/101/2022", "PHC/10001/2022".
//   - Pre-2022 Supreme Court prefixes HC/OS and SIC/S are accepted only behind a
//     label (the keyword gate). They are not accepted as bare shapes, because the
//     page text for them was not verified (see SOURCES.md).
//   - Malaysia has no official case-number format page that was found. The
//     Malaysian shape is taken from case numbers printed in published judgments
//     ("WA-22NCC-591-11 OF 2022", "WA-22NCC-459-09 OF 2020",
//     "WA-24NCC-131-03/2018", "WA-25-560-12 OF 2019"), and is matched only behind a
//     label, through the generic keyword form.
//
// Shape rules (no keyword needed, score 0.90) rest on a type code that makes the
// shape distinctive. The generic form (score 0.85) needs a label such as "case no"
// or "suit no" before the value.
package legal

import (
	"context"
	"regexp"
	"strings"

	"github.com/u007/dataprotect/internal/keyword"
	"github.com/u007/dataprotect/recognizers"
)

// ShapeScore is the confidence of a bare case-number shape.
const ShapeScore = 0.90

// KeywordScore is the confidence of a label-gated case number. It matches the
// other keyword-gated rules, so it is the same as keyword.Score.
const KeywordScore = keyword.Score

// usCase is a US federal district number: optional division digit and colon,
// two-digit year, optional hyphen, type, optional hyphen, a 1-5 digit sequence,
// then up to two suffix groups of 2-3 uppercase letters (judge initials) or two
// digits (defendant number).
var usCase = regexp.MustCompile(
	`\b(?:\d:)?\d{2}-?(?i:cv|cr|mc|md|mj)-?\d{1,5}\b` +
		`(?:-(?:[A-Z]{2,3}|\d{2})\b){0,2}`)

// sgSupremeCase is a Singapore Supreme Court number, "CA/CA 1/2021" form.
var sgSupremeCase = regexp.MustCompile(
	`\b(?:CA|AD)\s*/\s*(?:CA|OA|SUM)\s*\d{1,6}\s*/\s*\d{4}\b|` +
		`\bHC\s*/\s*(?:OA|OC|SUM)\s*\d{1,6}\s*/\s*\d{4}\b`)

// sgStateCase is a Singapore State Courts number, "DC / OC 1 / 2022" form.
var sgStateCase = regexp.MustCompile(
	`\b(?:DC|MC|PHC)\s*/\s*(?:OC|OA|SUM|EOA|EO)\s*\d{1,6}\s*/\s*\d{4}\b`)

// sgTribunalCase is a Singapore tribunal form, "SCT/10001/2022" form.
var sgTribunalCase = regexp.MustCompile(`\b(?:SCT|ECT|CDT|PHC)/\d{1,6}/\d{4}\b`)

// caseLabels are the labels that name a case number. Each is case-insensitive.
// An optional "." after "no" needs no entry: the label gate accepts any
// non-alphanumeric run after the label.
const caseLabels = `(?i:case\s+no|case\s+number|suit\s+no|civil\s+suit\s+no|civil\s+action\s+no|docket\s+no|matter\s+no|originating\s+summons\s+no|guaman\s+no|guaman\s+sivil\s+no|saman\s+pemula\s+no|rayuan\s+sivil\s+no|civil\s+appeal\s+no|criminal\s+appeal\s+no)`

// genericValue is one token that starts with an uppercase letter or digit and
// holds letters, digits and the separators - / ( ) :, optionally followed by
// " OF yyyy" (the "of" in either case). A space inside the value ends it, so
// "S 1234/2019" is not one token and is not matched. The digit requirement is checked in Recognize, and the token is at
// most 40 characters.
const genericValue = `[A-Z0-9][A-Za-z0-9\-/():]{0,39}(?:(?i: of) \d{4})?`

// The HC/OS and SIC/S prefixes are accepted only behind a label.
const legacySupremeValue = `(?:HC/OS|SIC/S)\s?\d{1,6}/\d{4}`

var (
	genericLabel = keyword.LabelGate(caseLabels, genericValue)
	legacyLabel  = keyword.LabelGate(caseLabels, legacySupremeValue)
)

// CaseNumberRecognizer finds court case numbers.
type CaseNumberRecognizer struct{}

// NewCaseNumberRecognizer creates a case-number recognizer.
func NewCaseNumberRecognizer() *CaseNumberRecognizer { return &CaseNumberRecognizer{} }

// Name returns the recognizer name.
func (r *CaseNumberRecognizer) Name() string { return "legal_case_number" }

// Recognize reports case numbers. Shape matches report the whole number. Label
// matches report the value alone, never the label.
func (r *CaseNumberRecognizer) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	var entities []recognizers.Entity
	add := func(start, end int, score float64) {
		entities = append(entities, recognizers.Entity{
			Type:       recognizers.EntityCaseNumber,
			Start:      start,
			End:        end,
			Text:       text[start:end],
			Confidence: score,
		})
	}
	for _, re := range []*regexp.Regexp{usCase, sgSupremeCase, sgStateCase, sgTribunalCase} {
		for _, m := range re.FindAllStringIndex(text, -1) {
			add(m[0], m[1], ShapeScore)
		}
	}
	for _, m := range genericLabel.FindAllStringSubmatchIndex(text, -1) {
		value := text[m[2]:m[3]]
		// The 40-character limit applies to the token, not to the " of yyyy" tail.
		token := strings.SplitN(strings.ToUpper(value), " OF ", 2)[0]
		if !strings.ContainsAny(value, "0123456789") || len(token) > 40 {
			continue
		}
		add(m[2], m[3], KeywordScore)
	}
	for _, m := range legacyLabel.FindAllStringSubmatchIndex(text, -1) {
		add(m[2], m[3], KeywordScore)
	}
	return entities, nil
}
