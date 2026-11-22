package identity

import (
	"github.com/u007/dataprotect/internal/keyword"
	"github.com/u007/dataprotect/recognizers"
)

// taxIDValue is the value shape for a tax identification number, one of three
// alternatives:
//
//   - EIN, NN-NNNNNNN, with the two-digit prefix limited to the values the IRS
//     assigns (https://www.irs.gov/businesses/small-businesses-self-employed/how-eins-are-assigned-and-valid-ein-prefixes,
//     fetched 2026-11-22): 01-06, 10-16, 20-27, 30-48, 50-68, 71-77, 80-88,
//     90-95, 98, 99. The IRS page does not state the dash layout; it is taken
//     from the labelled corpus values below.
//   - NNN-NN-NNNN, the ITIN format. The ITIN source
//     (https://en.wikipedia.org/wiki/Individual_Taxpayer_Identification_Number,
//     fetched 2026-11-22, citing IRS Publication 1915) gives a nine-digit number
//     beginning with 9, with the fourth and fifth digits in 50-65, 70-88, 90-92
//     or 94-99, formatted 9XX-XX-XXXX. The range is NOT enforced here, because
//     the corpus labels SSN-format tax ids outside it as tax_id: of the 23
//     nemotron SSN-format tax_id values, only those starting with 9 can be ITIN
//     and the rest are not (see SOURCES.md for the counts). Any NNN-NN-NNNN is
//     therefore accepted under the keyword. The SSN recognizer matches the same
//     shape at 0.99, and SSN wins the overlap, so in default output this shape
//     is reported as SSN; TAX_ID wins it only when SSN is disabled.
//   - A letter and eight digits, P12345678, the form three nemotron tax_id values
//     take.
//
// Corpus evidence (nemotron-pii tax_id, 31 values, unlabelled before this rule
// was mapped): 5 EIN-shaped (all with a valid prefix), 23 NNN-NN-NNNN, 3
// letter-plus-eight-digit. Labels seen before the value include "tax id", "Tax
// ID", "tax identification number", "TIN", "employer tax id" and "Taxpayer's Tax
// ID". The gate reaches 28 of 31; the rule reports 6 (3 EIN and 3
// letter-plus-eight-digit). The 23 NNN-NN-NNNN values lose to SSN.
const taxIDValue = `(?:(?:0[1-6]|1[0-6]|2[0-7]|3\d|4[0-8]|5\d|6[0-8]|7[1-7]|8[0-8]|9[0-5]|98|99)-\d{7}|\d{3}-\d{2}-\d{4}|[A-Z]\d{8})`

// taxIDKeyword lists the labels that name a tax identification number: the
// full phrases, the abbreviations TIN, ITIN and EIN, and "taxpayer id".
const taxIDKeyword = `(?i:tax\s*(?:id|identification\s+number|identification|payer\s+id)|taxpayer\s+id|employer\s+identification\s+number|employer\s+id(?:entification)?|ITIN|TIN|EIN)`

var taxIDPattern = keyword.LabelGate(taxIDKeyword, taxIDValue)

// NewTaxIDRecognizer creates a tax identification number recognizer.
func NewTaxIDRecognizer() recognizers.Recognizer {
	return &labelRecognizer{name: "identity_tax_id", typ: recognizers.EntityTaxID, pattern: taxIDPattern}
}
