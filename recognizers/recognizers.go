// Package recognizers provides the core types and interfaces for PII detection.
package recognizers

import (
	"context"
	"fmt"
)

// EntityType represents the category of detected PII.
type EntityType string

const (
	EntityEmail      EntityType = "EMAIL"
	EntityPhone      EntityType = "PHONE"
	EntityCreditCard EntityType = "CREDIT_CARD"
	EntitySSN        EntityType = "SSN"
	EntityIP         EntityType = "IP_ADDRESS"
	EntityURL        EntityType = "URL"
	EntityIBAN       EntityType = "IBAN"
	EntityBitcoin    EntityType = "BITCOIN"
	EntitySGNRIC     EntityType = "SG_NRIC"
	EntityMYIC       EntityType = "MY_IC"
	EntityTHID       EntityType = "TH_ID"
	EntityMYPassport EntityType = "MY_PASSPORT"
)

// Entity represents a detected PII entity in text.
type Entity struct {
	Type       EntityType
	Start      int
	End        int
	Text       string
	Confidence float64
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
	default:
		return "", fmt.Errorf("unknown entity type: %s", s)
	}
}
