package identity

import (
	"github.com/u007/dataprotect/internal/keyword"
	"github.com/u007/dataprotect/recognizers"
)

// dobMonth is the English month name, capitalised as the corpus writes it.
const dobMonth = `(?:January|February|March|April|May|June|July|August|September|October|November|December)`

// dateOfBirthValue is the date shape, one of four alternatives, each observed in
// a labelled corpus value:
//   - ISO, 1980-02-27 (nemotron date_of_birth 269 of 269 values are this form;
//     gretel has 17 and two single-digit-day forms, 1980-02-2)
//   - day/month/year or month/day/year, 01/01/1980 (gretel 40 values)
//   - Month D, YYYY, January 1, 1950 (gretel)
//   - D Month YY or YYYY, 25th November 1990 and 23rd May 97 (gretel)
//
// Letter placeholders (MM/DD/YYYY, [Insert DOB]) are not dates and are not
// matched; the corpus labels 7 such placeholders, which are missed on purpose.
const dateOfBirthValue = `(?:\d{4}-\d{1,2}-\d{1,2}|\d{2}/\d{2}/\d{4}|` + dobMonth + `\s+\d{1,2},\s+\d{4}|\d{1,2}(?:st|nd|rd|th)?\s+` + dobMonth + `\s+\d{2}(?:\d{2})?)`

// dateOfBirthKeyword lists the birth labels: "date of birth" in its spaced,
// underscored and JSON-escaped forms, "DOB", "birth date" and "born on".
const dateOfBirthKeyword = `(?i:date[\s_\\]*of[\s_\\]*birth|DOB|birth[\s_]?date|born\s+on)`

var dateOfBirthPattern = keyword.LabelGate(dateOfBirthKeyword, dateOfBirthValue)

// NewDateOfBirthRecognizer creates a date-of-birth recognizer. Only the date is
// reported, never the label.
func NewDateOfBirthRecognizer() recognizers.Recognizer {
	return &labelRecognizer{name: "identity_date_of_birth", typ: recognizers.EntityDateOfBirth, pattern: dateOfBirthPattern}
}
