// Package context provides context-aware confidence boosting for PII detection.
package context

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/u007/dataprotect/recognizers"
)

// BoostContext contains the English words that boost confidence when found
// near an entity. It is the default set.
var BoostContext = map[recognizers.EntityType][]string{
	recognizers.EntityEmail: {
		"email", "e-mail", "mail", "contact", "reach",
	},
	recognizers.EntityPhone: {
		"phone", "call", "tel", "telephone", "mobile", "cell", "fax",
	},
	recognizers.EntityCreditCard: {
		"card", "credit", "debit", "visa", "mastercard", "amex", "cc",
	},
	recognizers.EntitySSN: {
		"ssn", "social", "security", "tax", "id",
	},
	recognizers.EntityIP: {
		"ip", "address", "host", "server",
	},
	recognizers.EntityIBAN: {
		"iban", "bank", "account", "routing",
	},
	recognizers.EntityBitcoin: {
		"bitcoin", "btc", "wallet", "address",
	},
	// Legal role words. A party name next to one of these is a party to a
	// proceeding or a contract. They are boost words only and never report a span.
	recognizers.EntityPerson:       legalRoleWords,
	recognizers.EntityOrganization: legalRoleWords,
}

// legalRoleWords are the English and Malay role words that mark a PERSON or
// ORGANIZATION as a party: litigants, contracting parties and fiduciaries. The
// Malay forms are the court-form spellings (plaintif, defendan, perayu,
// responden, pemohon).
var legalRoleWords = []string{
	"plaintiff", "defendant", "claimant", "respondent", "appellant", "applicant",
	"petitioner", "borrower", "lender", "guarantor", "lessor", "lessee",
	"licensor", "licensee", "vendor", "purchaser", "trustee", "beneficiary",
	"testator", "executor", "counsel", "solicitor",
	"plaintif", "defendan", "perayu", "responden", "pemohon",
}

// BoostContextByLanguage holds the non-English keyword sets. A language that
// does not list a type falls back to the English words for that type, so an
// e-mail address labelled "E-Mail" is still boosted in every language. These
// sets are opt-in: a Spanish "tarjeta" in English text must not boost an
// English entity unless Spanish was requested (see analyzer.WithContextBoostLanguage).
var BoostContextByLanguage = map[string]map[recognizers.EntityType][]string{
	"de": {
		recognizers.EntityEmail:      {"e-mail", "mail", "kontakt"},
		recognizers.EntityPhone:      {"telefon", "tel", "mobil", "fax", "anruf"},
		recognizers.EntityCreditCard: {"kreditkarte", "kartennummer", "ec-karte", "debitkarte"},
		recognizers.EntitySSN:        {"sozialversicherung", "steuer-id", "steueridentifikation", "id"},
		recognizers.EntityIP:         {"ip", "adresse", "server", "host"},
		recognizers.EntityIBAN:       {"iban", "bank", "konto"},
		recognizers.EntityBitcoin:    {"bitcoin", "btc", "wallet", "brieftasche"},
	},
	"nl": {
		recognizers.EntityEmail:      {"e-mail", "mail", "contact"},
		recognizers.EntityPhone:      {"telefoon", "tel", "mobiel", "fax"},
		recognizers.EntityCreditCard: {"creditcard", "betaalkaart", "kaartnummer", "pinpas"},
		recognizers.EntitySSN:        {"bsn", "burgerservicenummer", "sofinummer", "id"},
		recognizers.EntityIP:         {"ip", "adres", "server", "host"},
		recognizers.EntityIBAN:       {"iban", "bank", "rekening"},
		recognizers.EntityBitcoin:    {"bitcoin", "btc", "wallet"},
	},
	"fr": {
		recognizers.EntityEmail:      {"e-mail", "mail", "courriel", "contact"},
		recognizers.EntityPhone:      {"téléphone", "telephone", "tel", "mobile", "fax"},
		recognizers.EntityCreditCard: {"carte bancaire", "carte de crédit", "carte de credit", "numéro de carte"},
		recognizers.EntitySSN:        {"sécurité sociale", "securite sociale", "numéro fiscal", "id"},
		recognizers.EntityIP:         {"ip", "adresse", "serveur", "hôte"},
		recognizers.EntityIBAN:       {"iban", "banque", "compte"},
		recognizers.EntityBitcoin:    {"bitcoin", "btc", "portefeuille"},
	},
	"it": {
		recognizers.EntityEmail:      {"e-mail", "mail", "contatto"},
		recognizers.EntityPhone:      {"telefono", "tel", "cellulare", "fax"},
		recognizers.EntityCreditCard: {"carta di credito", "carta di debito", "numero carta", "carta"},
		recognizers.EntitySSN:        {"codice fiscale", "previdenza sociale", "id"},
		recognizers.EntityIP:         {"ip", "indirizzo", "server", "host"},
		recognizers.EntityIBAN:       {"iban", "banca", "conto"},
		recognizers.EntityBitcoin:    {"bitcoin", "btc", "portafoglio"},
	},
	"es": {
		recognizers.EntityEmail:      {"correo", "e-mail", "mail", "contacto"},
		recognizers.EntityPhone:      {"teléfono", "telefono", "tel", "móvil", "fax"},
		recognizers.EntityCreditCard: {"tarjeta de crédito", "tarjeta de debito", "número de tarjeta", "tarjeta"},
		recognizers.EntitySSN:        {"seguridad social", "número fiscal", "id"},
		recognizers.EntityIP:         {"ip", "dirección", "servidor", "host"},
		recognizers.EntityIBAN:       {"iban", "banco", "cuenta"},
		recognizers.EntityBitcoin:    {"bitcoin", "btc", "cartera"},
	},
	"sv": {
		recognizers.EntityEmail:      {"e-post", "mail", "kontakt"},
		recognizers.EntityPhone:      {"telefon", "tel", "mobil", "samtal", "fax"},
		recognizers.EntityCreditCard: {"kreditkort", "kortnummer", "betalkort", "kontokort"},
		recognizers.EntitySSN:        {"personnummer", "samordningsnummer", "id"},
		recognizers.EntityIP:         {"ip", "adress", "server", "host"},
		recognizers.EntityIBAN:       {"iban", "bank", "konto"},
		recognizers.EntityBitcoin:    {"bitcoin", "btc", "plånbok"},
	},
}

// BoostConfidence adjusts entity confidence based on the English context words.
func BoostConfidence(text string, entity recognizers.EntityType, start, end int) float64 {
	return boostConfidence(text, start, end, BoostContext[entity])
}

// BoostConfidenceForLanguage adjusts entity confidence using the keyword set
// for lang, falling back to the English words when lang is unknown or does not
// list the entity type.
func BoostConfidenceForLanguage(text string, entity recognizers.EntityType, start, end int, lang string) float64 {
	words := BoostContext[entity]
	if m, ok := BoostContextByLanguage[lang]; ok {
		if lw := m[entity]; len(lw) > 0 {
			words = lw
		}
	}
	return boostConfidence(text, start, end, words)
}

// boostConfidence is the shared scorer: it counts matching keywords as whole
// words in the 50-byte window around the entity, 0.1 each, capped at 0.3.
func boostConfidence(text string, start, end int, contextWords []string) float64 {
	if len(contextWords) == 0 {
		return 0.0
	}

	// Get surrounding text (50 chars before and after)
	contextStart := start - 50
	if contextStart < 0 {
		contextStart = 0
	}
	contextEnd := end + 50
	if contextEnd > len(text) {
		contextEnd = len(text)
	}
	surrounding := strings.ToLower(text[contextStart:contextEnd])

	boost := 0.0
	for _, word := range contextWords {
		if containsWord(surrounding, word) {
			boost += 0.1
		}
	}
	if boost > 0.3 {
		boost = 0.3
	}
	return boost
}

// containsWord reports whether word occurs in s with a word boundary on both
// sides: the rune immediately before and the rune immediately after the
// occurrence must not be a letter (a missing rune at either end of s counts as
// a boundary). Digits and punctuation such as '-', ':' and '.' are boundaries,
// so "steuer-id" and "tel:" match. Every occurrence is checked, so a word that
// first appears inside a longer word ("video" for "id") does not hide a later
// standalone one. word may contain spaces; the boundary applies to the outer
// ends of the phrase only. word must be non-empty.
func containsWord(s, word string) bool {
	for i := 0; ; {
		j := strings.Index(s[i:], word)
		if j < 0 {
			return false
		}
		start := i + j
		end := start + len(word)
		before, _ := utf8.DecodeLastRuneInString(s[:start])
		after, _ := utf8.DecodeRuneInString(s[end:])
		if !unicode.IsLetter(before) && !unicode.IsLetter(after) {
			return true
		}
		i = start + 1
	}
}
