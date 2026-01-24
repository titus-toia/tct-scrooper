package models

import (
	"encoding/json"
	"time"
)

type Property struct {
	ID                string    `json:"id" db:"id"`
	NormalizedAddress string    `json:"normalized_address" db:"normalized_address"`
	City              string    `json:"city" db:"city"`
	PostalCode        string    `json:"postal_code" db:"postal_code"`
	Beds              int       `json:"beds" db:"beds"`
	Baths             int       `json:"baths" db:"baths"`
	SqFt              int       `json:"sqft" db:"sqft"`
	PropertyType      string    `json:"property_type" db:"property_type"`
	FirstSeenAt       time.Time `json:"first_seen_at" db:"first_seen_at"`
	LastSeenAt        time.Time `json:"last_seen_at" db:"last_seen_at"`
	TimesListed       int       `json:"times_listed" db:"times_listed"`
	Synced            bool      `json:"synced" db:"synced"`
}

type SupabaseProperty struct {
	ID               string          `json:"id"`
	Address          string          `json:"address"`
	City             string          `json:"city"`
	Beds             int             `json:"beds"`
	Baths            int             `json:"baths"`
	SqFt             int             `json:"sqft"`
	PropertyType     string          `json:"property_type"`
	CurrentPrice     int             `json:"current_price"`
	CurrentListingID string          `json:"current_listing_id"`
	CurrentURL       string          `json:"current_url"`
	Photos           json.RawMessage `json:"photos"`
	IsActive         bool            `json:"is_active"`
	History          json.RawMessage `json:"history"`
	TimesListed      int             `json:"times_listed"`
	FirstSeenAt      time.Time       `json:"first_seen_at"`
	LastSeenAt       time.Time       `json:"last_seen_at"`
	LastSyncedAt     time.Time       `json:"last_synced_at"`
	SiteID           string          `json:"site_id"`
}

type HistoryEvent struct {
	Event         string    `json:"event"`
	Date          time.Time `json:"date"`
	Price         int       `json:"price,omitempty"`
	ListingID     string    `json:"listing_id,omitempty"`
	URL           string    `json:"url,omitempty"`
	Photos        []string  `json:"photos,omitempty"`
	PreviousPrice int       `json:"previous_price,omitempty"`
	DaysOnMarket  int       `json:"days_on_market,omitempty"`
	DaysOffMarket int       `json:"days_off_market,omitempty"`
	PriceDropPct  float64   `json:"price_drop_pct,omitempty"`
}
