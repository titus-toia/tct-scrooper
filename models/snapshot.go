package models

import (
	"encoding/json"
	"time"
)

type Snapshot struct {
	ID          int64           `json:"id" db:"id"`
	PropertyID  string          `json:"property_id" db:"property_id"`
	ListingID   string          `json:"listing_id" db:"listing_id"`
	SiteID      string          `json:"site_id" db:"site_id"`
	URL         string          `json:"url" db:"url"`
	Price       int             `json:"price" db:"price"`
	Description string          `json:"description" db:"description"`
	Realtor     json.RawMessage `json:"realtor" db:"realtor"`
	Data        json.RawMessage `json:"data" db:"data"`
	ScrapedAt   time.Time       `json:"scraped_at" db:"scraped_at"`
	RunID       int64           `json:"run_id" db:"run_id"`
}

// Realtor contains company and agent info for a listing
type Realtor struct {
	Company RealtorCompany `json:"company"`
	Agents  []RealtorAgent `json:"agents"`
}

type RealtorCompany struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Phone   string `json:"phone"`
	Address string `json:"address"`
}

type RealtorAgent struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Phone string `json:"phone"`
	Photo string `json:"photo"`
}

type RawListing struct {
	ID           string          `json:"id"`
	MLS          string          `json:"mls"`
	Address      string          `json:"address"`
	City         string          `json:"city"`
	PostalCode   string          `json:"postal_code"`
	Price        int             `json:"price"`
	Beds         int             `json:"beds"`
	Baths        int             `json:"baths"`
	SqFt         int             `json:"sqft"`
	PropertyType string          `json:"property_type"`
	URL          string          `json:"url"`
	Photos       []string        `json:"photos"`
	Description  string          `json:"description"`
	Realtor      *Realtor        `json:"realtor"`
	Data         json.RawMessage `json:"data"`
}
