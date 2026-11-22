package national

import (
	"github.com/u007/dataprotect/internal/keyword"
	"github.com/u007/dataprotect/recognizers"
)

// MY_SSM: Malaysia company, business and LLP registration number. Source:
// SSM announcement, https://www.ssm.com.my/Lists/Announcement/AnnouncementDetails.aspx?ID=134
// (fetched 2026-11-22).
//
// The announcement states that the registration number contains 12 digit
// characters for companies, businesses and Limited Liability Partnerships from
// 11 October 2019. Its example is "Registration No: 201901000005 (1312525-A)":
// the new number, then the old number in parentheses. The old form is
// 1 to 7 digits, a hyphen and one uppercase letter, and it is matched too
// because the announcement shows it.
//
// The digit groups are NOT validated: the announcement does not state what they
// mean. The rule is keyword-gated, because a bare 12-digit run is also the shape
// of a MyKad (MY_IC). A labelled 12-digit run is MY_SSM at 0.85, and MY_IC's
// weak bare-digit candidate (0.50) cannot take it. MY_IC yields to MY_SSM on an
// exact overlap at every score (analyzer.winsOverlap).
var mySSMValue = `\d{12}|\d{1,7}-[A-Z]`

var mySSMPattern = keyword.LabelGate(
	`(?i:company\s+registration\s+no|business\s+registration\s+no|registration\s+no|company\s+no|no\.?\s*pendaftaran|no\.?\s*syarikat|SSM|ROC)`,
	mySSMValue,
)

// NewMYSSMRecognizer creates a Malaysia SSM registration number recognizer.
func NewMYSSMRecognizer() recognizers.Recognizer {
	return &keywordRule{name: "national_my_ssm", typ: recognizers.EntityMYSSM, pattern: mySSMPattern}
}
