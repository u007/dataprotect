// Package context provides context-aware confidence boosting for PII detection.
package context

import (
	"strings"

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

// boostConfidence is the shared scorer: it counts matching keywords in the
// 50-byte window around the entity, 0.1 each, capped at 0.3.
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
		if strings.Contains(surrounding, word) {
			boost += 0.1
		}
	}
	if boost > 0.3 {
		boost = 0.3
	}
	return boost
}
