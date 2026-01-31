package models

import (
	"time"
)

type Property struct {
	ID                string    `json:"id" db:"id"`
	NormalizedAddress string    `json:"normalized_address" db:"normalized_address"`
	City              string    `json:"city" db:"city"`
	PostalCode        string    `json:"postal_code" db:"postal_code"`
	Beds              int       `json:"beds" db:"beds"`
	BedsPlus          int       `json:"beds_plus" db:"beds_plus"`
	Baths             int       `json:"baths" db:"baths"`
	SqFt              int       `json:"sqft" db:"sqft"`
	PropertyType      string    `json:"property_type" db:"property_type"`
	FirstSeenAt       time.Time `json:"first_seen_at" db:"first_seen_at"`
	LastSeenAt        time.Time `json:"last_seen_at" db:"last_seen_at"`
	TimesListed       int       `json:"times_listed" db:"times_listed"`
	Synced            bool      `json:"synced" db:"synced"`
	IsActive          bool      `json:"is_active" db:"is_active"`
}
