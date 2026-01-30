package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Property represents a physical real estate entity (permanent)
type DomainProperty struct {
	ID           uuid.UUID       `json:"id" db:"id"`
	Fingerprint  string          `json:"fingerprint" db:"fingerprint"`
	Country      string          `json:"country" db:"country"`
	Province     string          `json:"province" db:"province"`
	City         string          `json:"city" db:"city"`
	PostalCode   string          `json:"postal_code" db:"postal_code"`
	AddressFull  string          `json:"address_full" db:"address_full"`
	Lat          *float32        `json:"lat" db:"lat"`
	Lng          *float32        `json:"lng" db:"lng"`
	UnitNumber   string          `json:"unit_number" db:"unit_number"`
	Floor        int             `json:"floor" db:"floor"`
	Stories      int             `json:"stories" db:"stories"`
	PropertyType string          `json:"property_type" db:"property_type"`
	YearBuilt    *int            `json:"year_built" db:"year_built"`
	LotSqFt      *int            `json:"lot_sqft" db:"lot_sqft"`
	Beds         *int            `json:"beds" db:"beds"`
	Baths        *int            `json:"baths" db:"baths"`
	SqFt         *int            `json:"sqft" db:"sqft"`
	Details      json.RawMessage `json:"details" db:"details"`
	CreatedAt    time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at" db:"updated_at"`
}

// PropertyIdentifier links external IDs (MLS, parcel, etc.) to a property
type PropertyIdentifier struct {
	PropertyID uuid.UUID `json:"property_id" db:"property_id"`
	Type       string    `json:"type" db:"type"`       // mls, internal, parcel, tax_roll
	Identifier string    `json:"identifier" db:"identifier"`
	Source     string    `json:"source" db:"source"`
}

// Listing represents a sale or rental session for a property
type Listing struct {
	ID           uuid.UUID       `json:"id" db:"id"`
	PropertyID   uuid.UUID       `json:"property_id" db:"property_id"`
	Source       string          `json:"source" db:"source"`     // realtor_ca, zillow, etc.
	ExternalID   string          `json:"external_id" db:"external_id"` // MLS ID
	URL          string          `json:"url" db:"url"`
	Type         string          `json:"type" db:"type"`         // sale, rent, sale_and_rent
	Status       string          `json:"status" db:"status"`     // active, delisted, expired, etc.
	Price        *float64        `json:"price" db:"price"`
	Currency     string          `json:"currency" db:"currency"`
	Fees         json.RawMessage `json:"fees" db:"fees"`
	PropertyType string          `json:"property_type" db:"property_type"`
	Beds         *int            `json:"beds" db:"beds"`
	Baths        *int            `json:"baths" db:"baths"`
	SqFt         *int            `json:"sqft" db:"sqft"`
	SqFtLot      *int            `json:"sqft_lot" db:"sqft_lot"`
	Floor        int             `json:"floor" db:"floor"`
	Stories      int             `json:"stories" db:"stories"`
	Description  string          `json:"description" db:"description"`
	Features     json.RawMessage `json:"features" db:"features"`
	RawData      json.RawMessage `json:"raw_data" db:"raw_data"`
	LastSeen     time.Time       `json:"last_seen" db:"last_seen"`
	ListedAt     time.Time       `json:"listed_at" db:"listed_at"`
	DelistedAt   *time.Time      `json:"delisted_at" db:"delisted_at"`
	CreatedAt    time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at" db:"updated_at"`
}

// Media represents an image, video, or document
type Media struct {
	ID            uuid.UUID       `json:"id" db:"id"`
	S3Key         *string         `json:"s3_key" db:"s3_key"` // nullable until uploaded
	ContentHash   string          `json:"content_hash" db:"content_hash"`
	MediaType     string          `json:"media_type" db:"media_type"` // image, video, document
	MimeType      string          `json:"mime_type" db:"mime_type"`
	FileSizeBytes *int64          `json:"file_size_bytes" db:"file_size_bytes"`
	OriginalURL   string          `json:"original_url" db:"original_url"`
	Height        *int            `json:"height" db:"height"`
	Width         *int            `json:"width" db:"width"`
	Pages         *int            `json:"pages" db:"pages"`
	Duration      *int            `json:"duration" db:"duration"`
	Metadata      json.RawMessage `json:"metadata" db:"metadata"`
	Status        string          `json:"status" db:"status"`     // pending, uploading, uploaded, failed
	Attempts      int             `json:"attempts" db:"attempts"`
	CreatedAt     time.Time       `json:"created_at" db:"created_at"`
}

// ListingMedia links media to a listing
type ListingMedia struct {
	ListingID uuid.UUID `json:"listing_id" db:"listing_id"`
	MediaID   uuid.UUID `json:"media_id" db:"media_id"`
	Position  int       `json:"position" db:"position"`
}

// Brokerage represents a real estate brokerage company
type Brokerage struct {
	ID        uuid.UUID  `json:"id" db:"id"`
	Name      string     `json:"name" db:"name"`
	Brand     string     `json:"brand" db:"brand"` // RE/MAX, Century 21, etc.
	Phone     string     `json:"phone" db:"phone"`
	Email     string     `json:"email" db:"email"`
	Website   string     `json:"website" db:"website"`
	Address   string     `json:"address" db:"address"`
	Country   string     `json:"country" db:"country"`
	City      string     `json:"city" db:"city"`
	Province  string     `json:"province" db:"province"`
	LogoID    *uuid.UUID `json:"logo_id" db:"logo_id"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
}

// Agent represents a real estate agent
type Agent struct {
	ID            uuid.UUID  `json:"id" db:"id"`
	FullName      string     `json:"full_name" db:"full_name"`
	LicenseNumber string     `json:"license_number" db:"license_number"`
	Email         string     `json:"email" db:"email"`
	Phone         string     `json:"phone" db:"phone"`
	Bio           string     `json:"bio" db:"bio"`
	BrokerageID   *uuid.UUID `json:"brokerage_id" db:"brokerage_id"`
	HeadshotID    *uuid.UUID `json:"headshot_id" db:"headshot_id"`
	FirstSeenAt   time.Time  `json:"first_seen_at" db:"first_seen_at"`
	LastSeenAt    time.Time  `json:"last_seen_at" db:"last_seen_at"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
}

// ListingAgent links agents to listings
type ListingAgent struct {
	ListingID uuid.UUID `json:"listing_id" db:"listing_id"`
	AgentID   uuid.UUID `json:"agent_id" db:"agent_id"`
	Role      string    `json:"role" db:"role"` // listing, buying, co_listing, referral
}

// PropertyMatch represents a potential duplicate property
type PropertyMatch struct {
	ID           int64           `json:"id" db:"id"`
	MatchedID    uuid.UUID       `json:"matched_id" db:"matched_id"`
	IncomingID   uuid.UUID       `json:"incoming_id" db:"incoming_id"`
	Confidence   float32         `json:"confidence" db:"confidence"`
	MatchReasons json.RawMessage `json:"match_reasons" db:"match_reasons"`
	Status       string          `json:"status" db:"status"` // pending, confirmed, rejected
	ReviewedAt   *time.Time      `json:"reviewed_at" db:"reviewed_at"`
	CreatedAt    time.Time       `json:"created_at" db:"created_at"`
}

// PropertyEvent represents a timeline event for a property
type PropertyEvent struct {
	ID            int64      `json:"id" db:"id"`
	PropertyID    uuid.UUID  `json:"property_id" db:"property_id"`
	EventType     string     `json:"event_type" db:"event_type"` // listed, delisted, relisted, price_change, etc.
	EventDate     time.Time  `json:"event_date" db:"event_date"`
	Price         *float64   `json:"price" db:"price"`
	PreviousPrice *float64   `json:"previous_price" db:"previous_price"`
	Summary       string     `json:"summary" db:"summary"`
	SourceType    string     `json:"source_type" db:"source_type"` // listing, assessment, record, intel, geo_event
	Source        string     `json:"source" db:"source"`           // scraper, gov_import, manual, etc.
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
}

// PricePoint represents a price record
type PricePoint struct {
	ID          int64      `json:"id" db:"id"`
	PropertyID  uuid.UUID  `json:"property_id" db:"property_id"`
	ListingID   *uuid.UUID `json:"listing_id" db:"listing_id"`
	PriceType   string     `json:"price_type" db:"price_type"` // asking_sale, asking_rent, assessed_total, etc.
	Amount      float64    `json:"amount" db:"amount"`
	Currency    string     `json:"currency" db:"currency"`
	Period      string     `json:"period" db:"period"` // one_time, monthly, yearly, bi_weekly
	EffectiveAt time.Time  `json:"effective_at" db:"effective_at"`
	Source      string     `json:"source" db:"source"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
}

// PropertyLink represents a URL associated with a property
type PropertyLink struct {
	ID          int64      `json:"id" db:"id"`
	PropertyID  uuid.UUID  `json:"property_id" db:"property_id"`
	ListingID   *uuid.UUID `json:"listing_id" db:"listing_id"`
	URL         string     `json:"url" db:"url"`
	Site        string     `json:"site" db:"site"`           // realtor_ca, zillow, etc.
	LinkType    string     `json:"link_type" db:"link_type"` // listing, fsbo, mention, suspicious
	IsPrimary   bool       `json:"is_primary" db:"is_primary"`
	IsActive    bool       `json:"is_active" db:"is_active"`
	FirstSeenAt time.Time  `json:"first_seen_at" db:"first_seen_at"`
	LastSeenAt  time.Time  `json:"last_seen_at" db:"last_seen_at"`
	Notes       string     `json:"notes" db:"notes"`
}

// DomainScrapeRun represents a scrape execution record
type DomainScrapeRun struct {
	ID            int64           `json:"id" db:"id"`
	Source        string          `json:"source" db:"source"`
	StartedAt     time.Time       `json:"started_at" db:"started_at"`
	FinishedAt    *time.Time      `json:"finished_at" db:"finished_at"`
	Status        string          `json:"status" db:"status"` // running, completed, failed, partial, cancelled
	ListingsFound int             `json:"listings_found" db:"listings_found"`
	ListingsNew   int             `json:"listings_new" db:"listings_new"`
	PropertiesNew int             `json:"properties_new" db:"properties_new"`
	ErrorsCount   int             `json:"errors_count" db:"errors_count"`
	ErrorMessage  string          `json:"error_message" db:"error_message"`
	Metadata      json.RawMessage `json:"metadata" db:"metadata"`
}

// DomainScrapeLog represents a log entry for a scrape run
type DomainScrapeLog struct {
	ID        int64     `json:"id" db:"id"`
	RunID     *int64    `json:"run_id" db:"run_id"`
	Timestamp time.Time `json:"timestamp" db:"timestamp"`
	Level     string    `json:"level" db:"level"` // debug, info, warn, error, fatal
	Message   string    `json:"message" db:"message"`
	SourceID  string    `json:"source_id" db:"source_id"`
}

// Event types
const (
	EventTypeListed      = "listed"
	EventTypeDelisted    = "delisted"
	EventTypeRelisted    = "relisted"
	EventTypePriceChange = "price_change"
	EventTypeExpired     = "expired"
	EventTypePending     = "pending"
	EventTypeWithdrawn   = "withdrawn"
)

// Listing status
const (
	ListingStatusActive    = "active"
	ListingStatusDelisted  = "delisted"
	ListingStatusExpired   = "expired"
	ListingStatusWithdrawn = "withdrawn"
	ListingStatusPending   = "pending"
)

// Price types
const (
	PriceTypeAskingSale  = "asking_sale"
	PriceTypeAskingRent  = "asking_rent"
	PriceTypeAssessedTotal = "assessed_total"
	PriceTypeCondoFee    = "condo_fee"
)

// Media status
const (
	MediaStatusPending   = "pending"
	MediaStatusUploading = "uploading"
	MediaStatusUploaded  = "uploaded"
	MediaStatusFailed    = "failed"
)

// Identifier types
const (
	IdentifierTypeMLS     = "mls"
	IdentifierTypeParcel  = "parcel"
	IdentifierTypeTaxRoll = "tax_roll"
)
