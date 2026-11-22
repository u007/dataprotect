package identity

import (
	"github.com/u007/dataprotect/internal/keyword"
	"github.com/u007/dataprotect/recognizers"
)

// driverLicenseValue is the driver licence number shape seen in labelled corpus
// values: an optional one or two letters, then a digit, then digits and hyphens
// ending on a digit. Lengths run from eight to twenty characters.
//
// Corpus evidence (gretel-pii-finance driver_license_number, 49 values,
// unlabelled before this rule was mapped): the shapes are 99-999999-99 (12),
// A99999999 (11), A999-9999-999-9 (10), AA99999999 (8) and A99-9999-999-99 (8),
// and every value fits this one pattern. No universal driver licence shape
// exists; states and countries differ, so the shape is a corpus observation.
const driverLicenseValue = `[A-Z]{0,2}\d[\d-]{6,18}\d`

// driverLicenseKeyword lists the labels for a driver licence: "driver's
// license" and its spellings, "driving licence" and "DL number". The label
// "policy number", which the corpus also uses for licence values, is not a
// licence label and is not listed.
const driverLicenseKeyword = `(?i:driver'?s?\s+licen[cs]e(?:\s+(?:number|no\.?))?|driving\s+licen[cs]e(?:\s+(?:number|no\.?))?|DL\s*(?:number|no\.?|#))`

var driverLicensePattern = keyword.LabelGate(driverLicenseKeyword, driverLicenseValue)

// NewDriverLicenseRecognizer creates a driver licence number recognizer.
func NewDriverLicenseRecognizer() recognizers.Recognizer {
	return &labelRecognizer{name: "identity_driver_license", typ: recognizers.EntityDriverLicense, pattern: driverLicensePattern}
}
