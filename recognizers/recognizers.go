// Package recognizers provides the core types and interfaces for PII detection.
package recognizers

import (
	"context"
	"fmt"
)

// EntityType represents the category of detected PII.
type EntityType string

const (
	EntityEmail         EntityType = "EMAIL"
	EntityPhone         EntityType = "PHONE"
	EntityCreditCard    EntityType = "CREDIT_CARD"
	EntityPerson        EntityType = "PERSON"
	EntityOrganization  EntityType = "ORGANIZATION"
	EntityLocation      EntityType = "LOCATION"
	EntitySSN           EntityType = "SSN"
	EntityIP            EntityType = "IP_ADDRESS"
	EntityURL           EntityType = "URL"
	EntityIBAN          EntityType = "IBAN"
	EntityBitcoin       EntityType = "BITCOIN"
	EntitySGNRIC        EntityType = "SG_NRIC"
	EntityMYIC          EntityType = "MY_IC"
	EntityTHID          EntityType = "TH_ID"
	EntityMYPassport    EntityType = "MY_PASSPORT"
	EntitySGPassport    EntityType = "SG_PASSPORT"
	EntityPHTIN         EntityType = "PH_TIN"
	EntityINGSTIN       EntityType = "IN_GSTIN"
	EntitySecret        EntityType = "SECRET"
	EntitySWIFTBIC      EntityType = "SWIFT_BIC"
	EntityUSRouting     EntityType = "US_ROUTING"
	EntityCardCVV       EntityType = "CARD_CVV"
	EntityBankAccount   EntityType = "BANK_ACCOUNT"
	EntityPIN           EntityType = "PIN"
	EntityTaxID         EntityType = "TAX_ID"
	EntityPassport      EntityType = "PASSPORT"
	EntityDriverLicense EntityType = "DRIVER_LICENSE"
	EntityDateOfBirth   EntityType = "DATE_OF_BIRTH"
	EntityLEI           EntityType = "LEI"
	EntityISIN          EntityType = "ISIN"
	EntityINPAN         EntityType = "IN_PAN"
	EntityEthereum      EntityType = "ETHEREUM"
	EntitySGUEN         EntityType = "SG_UEN"
	EntityMYSSM         EntityType = "MY_SSM"
	EntityMYTIN         EntityType = "MY_TIN"
	EntityCaseNumber    EntityType = "CASE_NUMBER"
	EntityMoney         EntityType = "MONEY"
	EntityCustomTerm    EntityType = "CUSTOM_TERM"
)

// AllEntityTypes lists every EntityType constant. A test checks that each one
// has an operator in the default Scrubber, so a new type cannot ship without
// one. A new constant must be added here as well.
var AllEntityTypes = []EntityType{
	EntityEmail,
	EntityPhone,
	EntityCreditCard,
	EntityPerson,
	EntityOrganization,
	EntityLocation,
	EntitySSN,
	EntityIP,
	EntityURL,
	EntityIBAN,
	EntityBitcoin,
	EntitySGNRIC,
	EntityMYIC,
	EntityTHID,
	EntityMYPassport,
	EntitySGPassport,
	EntityPHTIN,
	EntityINGSTIN,
	EntitySecret,
	EntitySWIFTBIC,
	EntityUSRouting,
	EntityCardCVV,
	EntityBankAccount,
	EntityPIN,
	EntityTaxID,
	EntityPassport,
	EntityDriverLicense,
	EntityDateOfBirth,
	EntityLEI,
	EntityISIN,
	EntityINPAN,
	EntityEthereum,
	EntitySGUEN,
	EntityMYSSM,
	EntityMYTIN,
	EntityCaseNumber,
	EntityMoney,
	EntityCustomTerm,
}

// Entity represents a detected PII entity in text.
type Entity struct {
	Type       EntityType
	Start      int
	End        int
	Text       string
	Confidence float64
	Weak       bool `json:"-"` // true if this entity is a weak candidate needing LLM confirmation before reporting
}

// Recognizer detects PII entities in text.
type Recognizer interface {
	Name() string
	Recognize(ctx context.Context, text string) ([]Entity, error)
}

// EntityTypeFromString converts a string to EntityType.
func EntityTypeFromString(s string) (EntityType, error) {
	switch s {
	case "EMAIL":
		return EntityEmail, nil
	case "PHONE":
		return EntityPhone, nil
	case "CREDIT_CARD":
		return EntityCreditCard, nil
	case "PERSON":
		return EntityPerson, nil
	case "ORGANIZATION":
		return EntityOrganization, nil
	case "LOCATION":
		return EntityLocation, nil
	case "SSN":
		return EntitySSN, nil
	case "IP_ADDRESS":
		return EntityIP, nil
	case "URL":
		return EntityURL, nil
	case "IBAN":
		return EntityIBAN, nil
	case "BITCOIN":
		return EntityBitcoin, nil
	case "SG_NRIC":
		return EntitySGNRIC, nil
	case "MY_IC":
		return EntityMYIC, nil
	case "TH_ID":
		return EntityTHID, nil
	case "MY_PASSPORT":
		return EntityMYPassport, nil
	case "SG_PASSPORT":
		return EntitySGPassport, nil
	case "PH_TIN":
		return EntityPHTIN, nil
	case "IN_GSTIN":
		return EntityINGSTIN, nil
	case "SECRET":
		return EntitySecret, nil
	case "SWIFT_BIC":
		return EntitySWIFTBIC, nil
	case "US_ROUTING":
		return EntityUSRouting, nil
	case "CARD_CVV":
		return EntityCardCVV, nil
	case "BANK_ACCOUNT":
		return EntityBankAccount, nil
	case "PIN":
		return EntityPIN, nil
	case "TAX_ID":
		return EntityTaxID, nil
	case "PASSPORT":
		return EntityPassport, nil
	case "DRIVER_LICENSE":
		return EntityDriverLicense, nil
	case "DATE_OF_BIRTH":
		return EntityDateOfBirth, nil
	case "LEI":
		return EntityLEI, nil
	case "ISIN":
		return EntityISIN, nil
	case "IN_PAN":
		return EntityINPAN, nil
	case "ETHEREUM":
		return EntityEthereum, nil
	case "SG_UEN":
		return EntitySGUEN, nil
	case "MY_SSM":
		return EntityMYSSM, nil
	case "MY_TIN":
		return EntityMYTIN, nil
	case "CASE_NUMBER":
		return EntityCaseNumber, nil
	case "MONEY":
		return EntityMoney, nil
	case "CUSTOM_TERM":
		return EntityCustomTerm, nil
	default:
		return "", fmt.Errorf("unknown entity type: %s", s)
	}
}
