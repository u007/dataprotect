package ner

import (
	"context"
	"strings"
	"sync"

	"github.com/jdkato/prose/v2"
	"github.com/u007/dataprotect/recognizers"
)

// DefaultConfidence is the score attached to every NLP entity.
//
// It must sit inside a window defined by the root module:
//
//   - strictly above the default scrubber floor (0.70), or the confidence
//     filter drops every NLP entity and the sidecar contributes nothing;
//   - strictly below the lowest built-in recognizer (0.85, the keyword-gated
//     national ID and passport rules), or NLP can win an overlap against a
//     typed detection and the "NLP fills gaps, never overrides typed" rule
//     stops holding.
//
// 0.80 leaves 0.10 of headroom above the floor and 0.05 below the ceiling.
// TestDefaultConfidenceWindow guards the window; the root module cannot import
// this constant (the sidecar imports the root module), so it repeats the value
// in nlp_combination_test.go with a comment pointing back here.
const DefaultConfidence = 0.80

// ProseBackend wraps prose for NER.
//
// prose is the model; this file is the decoder. prose's own entity output is
// produced by a coalescer that keeps the FIRST token's label for a span
// (except for a two-token span starting with B-PERSON, which it always calls
// PERSON). That discards information the token labels still carry, so this
// decoder works on the token labels directly and decides the type of each
// proper-name run itself.
//
// The rules below are closed-class and shape-based, not a fitted gazetteer:
//
//   - A RUN is a maximal sequence of consecutive proper-noun tokens. Single
//     words count, because a person's first name is one proper noun. An "&"
//     between two proper names continues the run.
//   - ORGANIZATION wins when the run ends in a legal form or an institutional
//     head noun ("Inc", "Ltd", "GmbH", "University", "Committee"). Those are
//     closed classes, and a name ending in one is an organisation whatever the
//     model said.
//   - PERSON wins when the run's FIRST token carries the PERSON label. prose
//     puts that label on the HEAD of a name, and its ORGANIZATION label on
//     the TAIL: in "Samuel Okoye" the head is B-PERSON and the tail is
//     I-ORGANIZATION, while in "Fernwood Logistics Inc" the head is B-GPE and
//     the tail is I-ORGANIZATION. The head label is the more reliable of the
//     two, so it is consulted first.
//   - ORGANIZATION also wins when any token in the run carries prose's
//     ORGANIZATION label and the head is not a person.
//   - LOCATION is what remains: prose labelled the run GPE, GSP or LOCATION.
//   - A month, a season or a language name is never a location. Those are
//     closed classes, so dropping them is a rule, not a guess about an open
//     vocabulary.
//
// A run that prose labelled nothing at all is NOT reported. prose's tagger
// marks plenty of ordinary words as proper nouns ("Version"), and reporting
// them would trade one false positive for a false negative.
//
// The measured result of these rules is in the NER gate (see the root module's
// ner_accuracy_test.go). The known misses are documented there rather than
// papered over with more rules.
type ProseBackend struct {
	model *prose.Model
	once  sync.Once
}

// NewProseBackend returns a ProseBackend.
//
// The model is NOT loaded here. prose.NewDocument loads the 17 MB model on
// every call when it is given no model, which cost 340 ms per request. The
// backend therefore builds the model once, on the first request, and reuses
// it. That is a 21x speedup and the token output is identical, because the
// model is read-only once built.
func NewProseBackend() *ProseBackend { return &ProseBackend{} }

// Name implements NERBackend.
func (p *ProseBackend) Name() string { return "prose" }

// model returns the shared prose model, building it on first use.
//
// sync.Once makes the build safe for concurrent requests: the model is
// immutable once built, and prose's classifiers are read-only during
// inference.
func (p *ProseBackend) getModel() *prose.Model {
	p.once.Do(func() {
		p.model = prose.ModelFromData("en-v2.0.0")
	})
	return p.model
}

// orgSuffixes are the legal forms and institutional head nouns that mark a
// capitalised run as an organisation.
//
// Each entry is matched against the LAST word of the run, case-insensitively,
// on a word boundary, so "Inc" matches "Inc" and "Incorporated" but not
// "Vince".
//
// Ordinary business words ("Systems", "Ventures", "Group") are deliberately
// NOT here. They are ambiguous common nouns, and a list of them would be
// fitted to whatever text the list was written against.
var orgSuffixes = []string{
	// Legal forms.
	"inc", "incorporated", "corp", "corporation", "ltd", "limited", "llc",
	"llp", "plc", "gmbh", "ag", "sa", "sas", "nv", "bv", "ab", "oy", "pty",
	"srl", "spa", "kk", "co", "company",
	// Institutional head nouns.
	"foundation", "institute", "university", "college", "hospital", "bank",
	"association", "authority", "agency", "bureau", "ministry", "council",
	"committee", "commission", "trust", "society", "federation", "union",
	"partnership",
}

// businessHeadNouns are the words a company name ends in.
//
// They are what separates "Cedarline Systems" (an organisation) from
// "Tobias Lindgren" (a person). prose gives both the same labels — a B-GPE
// head and a tail label — so without this list the two are
// indistinguishable.
//
// The list is a closed class of business words, not a gazetteer of company
// names. A company whose name ends in something outside the list is missed;
// that is the measured cost, and it is recorded in the NER gate.
var businessHeadNouns = map[string]bool{
	"systems": true, "ventures": true, "foods": true, "logistics": true,
	"manufacturing": true, "analytics": true, "trading": true,
	"technologies": true, "solutions": true, "industries": true,
	"enterprises": true, "laboratories": true, "pharmaceuticals": true,
	"consulting": true, "products": true, "services": true,
	"associates": true, "holdings": true, "publishing": true,
	"airlines": true, "motors": true, "textiles": true, "chemicals": true,
	"energy": true, "banking": true, "insurance": true, "properties": true,
	"robotics": true, "biotech": true, "software": true, "networks": true,
}

// placeHeadNouns are the words a PLACE name ends in.
//
// The list is deliberately DISJOINT from the business head nouns above, and
// that is the point. A word that could be either ("group", "press",
// "partners", "media", "capital") is a PLACE head noun here, because a name
// ending in one of those is more often a venue or a district than a legal
// entity, and the legal forms are caught by the legal-form branch instead.
// "Cape Town" is a place and "Cedarline Systems" is an organisation.
var placeHeadNouns = map[string]bool{
	"town": true, "city": true, "village": true, "hamlet": true, "borough": true,
	"county": true, "province": true, "region": true, "state": true,
	"country": true, "island": true, "islands": true, "sea": true, "ocean": true,
	"lake": true, "river": true, "valley": true, "hill": true, "hills": true,
	"mountain": true, "mountains": true, "desert": true, "forest": true,
	"park": true, "gardens": true, "beach": true, "bay": true, "gulf": true,
	"street": true, "road": true, "avenue": true, "lane": true, "drive": true,
	"boulevard": true, "square": true, "court": true, "place": true,
	"terrace": true, "crescent": true, "plaza": true, "heights": true,
	"springs": true, "falls": true, "creek": true, "point": true,
	"group": true, "press": true, "partners": true, "media": true, "capital": true,
	"bridge": true, "port": true, "harbour": true, "harbor": true,
	"airport": true, "station": true, "terminal": true, "centre": true,
	"center": true, "north": true, "south": true, "east": true, "west": true,
}

// months are the twelve month names. A month is a closed class, and a month
// reported as a location is always a false positive.
var months = map[string]bool{
	"january": true, "february": true, "march": true, "april": true,
	"may": true, "june": true, "july": true, "august": true,
	"september": true, "october": true, "november": true, "december": true,
}

// languages are the names of the world's languages and language families.
//
// A language is a closed class in the same way months are, and prose reports
// "Swedish" and "English" as places. The list is deliberately the major
// languages and their families; it is not a gazetteer of every language ever
// spoken, because the closed-class argument only holds for the large ones.
var languages = map[string]bool{
	"english": true, "french": true, "german": true, "spanish": true,
	"italian": true, "portuguese": true, "dutch": true, "danish": true,
	"swedish": true, "norwegian": true, "finnish": true, "icelandic": true,
	"polish": true, "czech": true, "slovak": true, "hungarian": true,
	"romanian": true, "greek": true, "turkish": true, "russian": true,
	"ukrainian": true, "serbian": true, "croatian": true, "bulgarian": true,
	"arabic": true, "hebrew": true, "persian": true, "urdu": true,
	"hindi": true, "bengali": true, "tamil": true, "telugu": true,
	"thai": true, "vietnamese": true, "chinese": true, "japanese": true,
	"korean": true, "mongolian": true, "indonesian": true, "malay": true,
	"tagalog": true, "swahili": true, "afrikaans": true, "amharic": true,
	"irish": true, "welsh": true, "gaelic": true, "latin": true,
	"sanskrit": true, "albanian": true, "slovenian": true, "macedonian": true,
	"catalan": true, "basque": true, "farsi": true,
}

// seasons are the four season names. Like months they are a closed class, and
// prose reports "Autumn" as a place.
var seasons = map[string]bool{
	"spring": true, "summer": true, "autumn": true, "fall": true, "winter": true,
}

// leadWords are the words that stand immediately before a name and are not
// part of it.
//
// prose tags these as proper nouns and then swallows the name that follows,
// so "Hello Fatima" and "Contact Amara Boateng" each came back as one span
// covering both the lead word and the name.
//
// The class is closed in the sense that matters here: a lead word is a
// greeting or a communication verb, and neither is ever a person's name.
// This is the same argument as for months and languages.
var leadWords = map[string]bool{
	// Greetings and closings.
	"hello": true, "hi": true, "hey": true, "dear": true, "greetings": true,
	"morning": true, "afternoon": true, "evening": true, "regards": true,
	"sincerely": true, "best": true, "thanks": true, "thank": true,
	"yours": true, "faithfully": true, "kind": true, "warm": true,
	// Communication verbs.
	"contact": true, "email": true, "call": true, "ask": true, "tell": true,
	"write": true, "inform": true, "notify": true, "message": true,
	"text": true, "phone": true, "fax": true, "reply": true, "reach": true,
	"cc": true, "bcc": true, "attn": true, "attention": true, "to": true,
	"from": true,
	// Courtesy titles. prose tags "Mr." as a proper noun, so "Mr. Aled Morgan"
	// came back as one span covering the title and the name.
	"mr": true, "mrs": true, "ms": true, "miss": true, "dr": true,
	"prof": true, "sir": true, "madam": true, "lord": true, "lady": true,
	"rev": true, "hon": true, "capt": true, "col": true, "gen": true,
	"sgt": true, "lt": true, "maj": true,
}

// notAnEntity is the set of closed-class words that prose reports as places
// but that never are. A month is a month and a language is a language; neither
// is a location.
func notAnEntity(word string) bool {
	w := strings.ToLower(word)
	return months[w] || languages[w]
}

// Recognize implements NERBackend.
func (p *ProseBackend) Recognize(ctx context.Context, text string) ([]recognizers.Entity, error) {
	doc, err := prose.NewDocument(text, prose.UsingModel(p.getModel()))
	if err != nil {
		return nil, err
	}
	toks := doc.Tokens()

	var out []recognizers.Entity
	for i := 0; i < len(toks); {
		run, next := collectRun(toks, i)
		i = next
		if len(run) == 0 {
			continue
		}
		typ, run, ok := classify(run)
		if !ok {
			continue
		}
		start, end, ok := locateRun(text, run)
		if !ok {
			continue
		}
		out = append(out, recognizers.Entity{
			Type:       typ,
			Start:      start,
			End:        end,
			Text:       text[start:end],
			Confidence: DefaultConfidence,
		})
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// collectRun gathers the proper-name run starting at i and returns the tokens
// and the index to continue from.
//
// An "&" between two proper names continues the run: "Harrington & Vale" is
// one name, and splitting it produces two fragments of a word. The "&" token
// is kept in the run so the located text matches the original.
func collectRun(toks []prose.Token, i int) ([]prose.Token, int) {
	if !isProperName(toks[i]) {
		return nil, i + 1
	}
	var run []prose.Token
	j := i
	for j < len(toks) {
		if isProperName(toks[j]) {
			run = append(run, toks[j])
			j++
			continue
		}
		if toks[j].Text == "&" && j+1 < len(toks) && isProperName(toks[j+1]) {
			run = append(run, toks[j])
			j++
			continue
		}
		break
	}
	return run, j
}

// classify decides the type of a proper-name run, and whether to report it.
//
// It returns the ADJUSTED run, because dropping a leading greeting changes
// which text the span covers. Returning the type alone would leave the caller
// locating the original run, and the span would still cover the greeting.
func classify(run []prose.Token) (recognizers.EntityType, []prose.Token, bool) {
	if len(run) == 0 {
		return "", nil, false
	}

	// A run of one word that is a month, a season or a language is neither a
	// person nor a place. Months, seasons and languages are closed classes, so
	// this is a rule rather than a guess about an open vocabulary.
	if len(run) == 1 {
		w := strings.ToLower(run[0].Text)
		if notAnEntity(w) || seasons[w] {
			return "", nil, false
		}
	}

	// A lead word is not part of the name. prose tags "Hello" and "Contact"
	// as proper nouns and swallows the name after them, so "Hello Fatima" and
	// "Contact Amara Boateng" each came back as one span covering both the
	// lead word and the name.
	if len(run) > 1 && leadWords[strings.ToLower(strings.Trim(run[0].Text, ".,;:"))] {
		run = run[1:]
		if len(run) == 0 {
			return "", nil, false
		}
	}

	person, place, org := 0, 0, 0
	for _, t := range run {
		switch {
		case hasLabel(t, "PERSON"):
			person++
		case hasLabel(t, "GPE"), hasLabel(t, "GSP"), hasLabel(t, "LOCATION"):
			place++
		case hasLabel(t, "ORGANIZATION"):
			org++
		}
	}

	// A structured field is not a name. A span carrying a field-path
	// character ("Path=/auth", "SameSite=Lax", "Name:**", "|") is a key in a
	// JSON object, an HTTP header or a spreadsheet column, and it is never a
	// person's name, an organisation or a place.
	//
	// This rule exists because the corpora are full of structured records.
	// On the nemotron corpus 14.0% of the NER detections carried such a
	// character, and every one of them was noise: "**HTTP Cookie:**",
	// "Expires=Mon", "Path=/auth", "Credit/Debit Card Number".
	if hasStructuredChar(run) {
		return "", nil, false
	}

	// A run whose LEADING words are all document field names is a field
	// label, not a name. The vocabulary is closed in the sense that matters:
	// these are the labels a structured record uses for its own fields, and
	// no person's name, company or place begins with two of them.
	//
	// "Record Number", "Account Number", "Medical Record Number" and
	// "Street Address" are all field labels that prose reported as places or
	// people.
	//
	// The check is on the LEADING words, not every word, because the value
	// that follows a label is often glued into the same run: prose made
	// "Claim ID 9921-A" and "Policy Number 4471-B" single runs, and the
	// value is not a field word. Checking every word would miss both.
	//
	// Two words are required, so a single field word that is also a real
	// word ("Name", "City", "Company") is not suppressed on its own.
	if hasFieldLabelPrefix(run) {
		return "", nil, false
	}

	// A run whose LAST word is a bare common noun is a field label with a
	// value glued to it, and the whole run is dropped.
	//
	// "Vendor Performance Report Q3" and "Policy Number 4471-B" are
	// labels followed by a value, and prose merged them into one run of
	// proper nouns. The label is not a name, and the value is not either, so
	// nothing in the run is reportable.
	//
	// The rule is deliberately WHOLE-RUN, not a trim. A trim was tried and
	// rejected: it cut "Halcyon Manufacturing Corp" to "Halcyon
	// Manufacturing", because "Corp" is both a legal form and a field word.
	//
	// A run that ends in a NAME-ending — a legal form, an institutional head
	// noun or a place head noun — is exempt. "Fernwood Logistics Inc" ends
	// in a legal form and is a company; "Elm Street" ends in a street type
	// and is an address. Only a bare common noun marks a label.
	if isBareCommonNoun(run[len(run)-1].Text) {
		return "", nil, false
	}

	// A street is an address, not an organisation. "14 Elm Street" ends in a
	// word prose tagged ORGANIZATION, and the organisation branch below would
	// take it. The street check comes first because it is the more specific
	// rule.
	if streetSuffixes[strings.ToLower(strings.Trim(run[len(run)-1].Text, ".,;:"))] {
		return recognizers.EntityLocation, run, true
	}

	// A legal form or an institutional head noun settles it.
	if matchesSuffix(run[len(run)-1].Text) {
		return recognizers.EntityOrganization, run, true
	}
	// A place head noun settles it as a place. It is checked before the
	// business head nouns because the two lists are disjoint and the place
	// reading is the right one.
	lastWord := strings.ToLower(strings.Trim(run[len(run)-1].Text, ".,;:"))
	if placeHeadNouns[lastWord] {
		return recognizers.EntityLocation, run, true
	}
	// A business head noun settles it too, and it is what separates
	// "Cedarline Systems" from "Tobias Lindgren": prose gives both a B-GPE
	// head and a tail label, so without this the two are
	// indistinguishable.
	if businessHeadNouns[lastWord] {
		return recognizers.EntityOrganization, run, true
	}
	// prose's PERSON label on the HEAD of a name beats its ORGANIZATION label
	// on the tail. The head label is the reliable one: in "Samuel Okoye" the
	// head is B-PERSON and the tail is I-ORGANIZATION, while in "Fernwood
	// Logistics Inc" the head is B-GPE and the tail is I-ORGANIZATION. Reading
	// the ORGANIZATION label as decisive turned "Samuel Okoye" into an
	// organisation.
	if hasLabel(run[0], "PERSON") {
		return recognizers.EntityPerson, run, true
	}
	// An ORGANIZATION label anywhere in a run whose head is not a person.
	if org > 0 {
		return recognizers.EntityOrganization, run, true
	}
	if person > 0 {
		return recognizers.EntityPerson, run, true
	}
	if place > 0 {
		return recognizers.EntityLocation, run, true
	}
	return "", nil, false
}

// hasLabel reports whether a token carries a label of the given type,
// including the I- continuation form.
func hasLabel(t prose.Token, typ string) bool {
	return t.Label == "B-"+typ || t.Label == "I-"+typ
}

// isProperName reports whether a token can be part of a name: a noun tagged
// as a proper noun, or an all-caps token with at least one letter.
//
// A token with no letter in it is not a name. "Version 3" is a version, and
// treating the digit as a proper noun made the pair look like a company.
func isProperName(t prose.Token) bool {
	switch t.Tag {
	case "NNP", "NNPS":
		return true
	}
	if t.Text == "" {
		return false
	}
	hasLetter := false
	allUpper := true
	for _, r := range t.Text {
		if r >= 'a' && r <= 'z' {
			allUpper = false
		}
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			hasLetter = true
		}
	}
	return hasLetter && allUpper
}

// matchesSuffix reports whether a word ends with a legal form or an
// institutional head noun, on a word boundary.
func matchesSuffix(word string) bool {
	w := strings.ToLower(strings.Trim(word, ".,;:"))
	for _, s := range orgSuffixes {
		if !strings.HasSuffix(w, s) {
			continue
		}
		if len(w) == len(s) {
			return true
		}
		prev := w[len(w)-len(s)-1]
		if prev >= 'a' && prev <= 'z' {
			continue
		}
		return true
	}
	return false
}

// locateRun finds the byte offsets of a run of tokens in the text.
//
// The tokens are joined with single spaces to build the candidate, because
// that is how prose renders a name. If the text used different whitespace the
// join does not match, so the search falls back to the first token.
func locateRun(text string, run []prose.Token) (int, int, bool) {
	parts := make([]string, 0, len(run))
	for _, t := range run {
		parts = append(parts, t.Text)
	}
	joined := strings.Join(parts, " ")
	if idx := strings.Index(text, joined); idx >= 0 {
		return trimTrailingPunct(text, idx, idx+len(joined))
	}
	first := run[0].Text
	if idx := strings.Index(text, first); idx >= 0 {
		return trimTrailingPunct(text, idx, idx+len(first))
	}
	return 0, 0, false
}

// trimTrailingPunct drops trailing punctuation from a span.
//
// prose attaches the sentence's full stop to the last token, so the token
// text of "Baltic Sea." ends with a period and the located span swallowed it.
// The span then covered one byte more than the name, and the exact match
// against the label failed.
//
// The trim is bounded by the span: it never eats into the following word.
func trimTrailingPunct(text string, start, end int) (int, int, bool) {
	for end > start {
		c := text[end-1]
		if c != '.' && c != ',' && c != ';' && c != ':' && c != '!' && c != '?' {
			break
		}
		end--
	}
	if end == start {
		return 0, 0, false
	}
	return start, end, true
}

// fieldNames are the labels a structured record uses for its own fields.
//
// This is a closed class in the sense that matters: a span made entirely of
// these words is a field label. On the two corpora, prose reported
// "Record Number" 21 times, "Account Number" 35 times, "Medical Record
// Number" 29 times, "Customer ID" 15 times, "Employee ID" 14 times and
// "Street Address" 30 times — all of them field labels, none of them names.
//
// A name that merely CONTAINS one of these words is not suppressed: the rule
// requires every word to be a field name.
var fieldNames = map[string]bool{
	// Record and field labels.
	"record": true, "records": true, "account": true, "number": true,
	"name": true, "first": true, "last": true, "address": true, "street": true,
	"city": true, "state": true, "zip": true, "country": true, "county": true,
	"code": true, "type": true, "value": true, "amount": true, "total": true,
	"balance": true, "date": true, "time": true, "year": true, "month": true,
	"day": true, "birth": true, "date of birth": true, "dob": true,
	"phone": true, "telephone": true, "mobile": true, "fax": true,
	"email": true, "e-mail": true, "website": true, "url": true, "uri": true,
	"ssn": true, "social": true, "security": true, "sin": true, "nric": true,
	"passport": true, "license": true, "licence": true, "id": true,
	"identifier": true, "reference": true, "ref": true, "invoice": true,
	"order": true, "payment": true, "method": true, "card": true, "cash": true,
	"bank": true, "iban": true, "swift": true, "bic": true, "currency": true,
	"description": true, "detail": true, "details": true,
	"subject": true, "note": true, "notes": true, "comment": true,
	"file": true, "page": true, "line": true, "text": true, "item": true,
	"product": true, "make": true, "model": true, "brand": true,
	"policy": true, "claim": true, "coverage": true, "premium": true,
	"deductible": true, "limit": true, "benefit": true, "member": true,
	"group": true, "employer": true, "employee": true,
	"customer": true, "client": true, "vendor": true, "supplier": true,
	"service": true, "services": true, "level": true,
	"priority": true, "category": true, "status": true, "result": true,
	"summary": true, "action": true, "task": true, "issue": true,
	"user": true, "username": true, "password": true, "token": true,
	"secret": true, "key": true, "signature": true, "authorization": true,
	"cookie": true, "agent": true, "header": true, "request": true,
	"response": true, "session": true, "expires": true, "issued": true,
	"secure": true, "httponly": true, "samesite": true, "lax": true,
	"strict": true, "path": true, "domain": true, "host": true,
	"scheme": true, "protocol": true, "port": true, "ip": true,
	"ipv4": true, "ipv6": true, "mac": true, "cvv": true, "cvc": true,
	"pin": true, "otp": true, "mfa": true, "2fa": true, "kyc": true,
	"aml": true, "tax": true, "vat": true, "payable": true, "receivable": true,
	"earnings": true, "equity": true, "equipment": true, "contents": true,
	"property": true, "player": true, "medical": true, "dental": true,
	"health": true, "biometric": true,
	"certificate": true, "jurisdiction": true, "compliance": true,
	"regulation": true, "regulations": true, "agreement": true,
	"program": true, "loyalty": true, "customer agreement": true,
	"privacy": true, "terms": true, "conditions": true, "consent": true,
	"credit": true, "debit": true, "deposit": true, "savings": true,
	"application": true, "form": true, "section": true, "paragraph": true,
	"demographics": true, "renewable": true, "modification": true, "cafe": true,
	"business": true, "reporting": true, "language": true,
	"energy": true, "conservation": true, "example": true,
	"case": true, "study": true, "termination": true,
	"approved": true, "review": true, "variance": true,

	// The words below were taken from the corpora, not guessed. Each one
	// appeared in the top of the false-positive list measured on the
	// nemotron and gretel corpora, as a field label that prose reported as a
	// place or a person.
	"report": true, "reports": true, "performance": true, "revision": true,
	"data": true, "sheet": true, "safety": true, "race": true,
	"ethnicity": true, "age": true, "identified": true, "identification": true,
	"identity": true, "variation": true, "modal": true,
	"entire": true, "substance": true, "hazard": true, "hvac": true,
	"furthermore": true, "company": true, "corporation": true,
	"loan": true, "lading": true,
	"corp": true,
}

// structuredChars are the characters that mark a span as a field path.
//
// A JSON key, an HTTP header and a spreadsheet column all use them, and no
// name does: "Path=/auth", "SameSite=Lax", "Name:**", "Credit/Debit",
// "Perkins|560=726".
const structuredChars = "=|*/{}[]<>\\`"

// hasStructuredChar reports whether any word in the run carries a
// structured-field character.
func hasStructuredChar(run []prose.Token) bool {
	for _, t := range run {
		if strings.ContainsAny(t.Text, structuredChars) {
			return true
		}
	}
	return false
}

// hasFieldLabelPrefix reports whether the run STARTS with two or more
// document field names.
//
// Two or more, because a single field word is also an ordinary word: "Name",
// "City", "Company" and "Report" are all field labels, and all four are also
// words a person or a place might be called. A PAIR is unambiguous: no
// person's name is "Record Number" and no place is "Street Address".
//
// The check stops at the first word that is not a field name, so the value
// glued to the label does not matter.
func hasFieldLabelPrefix(run []prose.Token) bool {
	seen := 0
	for _, t := range run {
		words := splitWords(t.Text)
		if len(words) == 0 {
			continue
		}
		for _, w := range words {
			if !fieldNames[strings.ToLower(w)] {
				return seen >= 2
			}
			seen++
		}
	}
	return seen >= 2
}

// isBareCommonNoun reports whether a word is a plain field label rather than
// something a name can END with.
//
// A name ends in a legal form ("Inc", "Ltd"), an institutional head noun
// ("University", "Committee") or a place head noun ("Street", "Town"). A
// name does not end in a bare common noun like "Report", "Number" or
// "Revision": those are labels.
//
// This is what separates "Halcyon Manufacturing Corp" (a name, the last
// word is a legal form) from "Policy Number 4471-B" (a label, the last word
// is a common noun).
func isBareCommonNoun(word string) bool {
	w := strings.ToLower(strings.Trim(word, ".,;:"))
	if matchesSuffix(w) {
		return false
	}
	if placeHeadNouns[w] || streetSuffixes[w] {
		return false
	}
	if businessHeadNouns[w] {
		return false
	}
	return fieldNames[w]
}

// splitWords breaks a token into lower-case words, splitting on punctuation
// that a field label may use.
func splitWords(text string) []string {
	replacer := strings.NewReplacer(
		":", " ", "=", " ", "|", " ", "/", " ", "*", " ", ".", " ", ",", " ",
		"(", " ", ")", " ", "[", " ", "]", " ", "{", " ", "}", " ", "_", " ",
		"\"", " ", "**", " ",
	)
	fields := strings.Fields(replacer.Replace(text))
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.Trim(f, ".*:,|/ ")
		if f == "" {
			continue
		}
		out = append(out, f)
	}
	return out
}

// streetSuffixes are the words that make a capitalised run an address rather
// than an organisation. "14 Elm Street" is a place, not a company, and prose
// labels it FACILITY/ORGANIZATION.
//
// A street type is a closed class, so this is a rule rather than a gazetteer
// of every road name in the world.
var streetSuffixes = map[string]bool{
	"street": true, "st": true, "road": true, "rd": true, "avenue": true,
	"ave": true, "lane": true, "ln": true, "drive": true, "dr": true,
	"boulevard": true, "blvd": true, "square": true, "sq": true, "court": true,
	"ct": true, "place": true, "pl": true, "terrace": true, "close": true,
	"crescent": true, "way": true, "walk": true, "parade": true, "highway": true,
	"hwy": true, "motorway": true, "alley": true, "gardens": true, "row": true,
}
