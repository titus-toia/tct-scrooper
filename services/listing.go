package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"tct_scrooper/identity"
	"tct_scrooper/models"
	"tct_scrooper/storage"
)

// ListingService handles the fan-out logic for processing raw listings
type ListingService struct {
	store   *storage.PostgresStore
	match   *MatchService
	media   *MediaService
}

// NewListingService creates a new ListingService
func NewListingService(store *storage.PostgresStore, match *MatchService, media *MediaService) *ListingService {
	return &ListingService{
		store: store,
		match: match,
		media: media,
	}
}

// ProcessResult contains the outcome of processing a listing
type ProcessResult struct {
	PropertyID    uuid.UUID
	ListingID     uuid.UUID
	IsNewProperty bool
	IsNewListing  bool
	IsRelisted    bool
	PriceChanged  bool
	EventsCreated int
	MediaQueued   int
}

// ProcessListing processes a raw listing and fans out to all related tables.
// This is idempotent - safe to call multiple times for the same listing.
func (s *ListingService) ProcessListing(ctx context.Context, raw *models.RawListing, source string, runID *int64) (*ProcessResult, error) {
	result := &ProcessResult{}
	now := time.Now()

	// 1. Compute fingerprint and find/create property
	fingerprint := identity.Fingerprint(raw)

	existingProp, err := s.store.GetPropertyByFingerprint(ctx, fingerprint)
	if err != nil {
		return nil, fmt.Errorf("get property: %w", err)
	}

	var property *models.DomainProperty
	if existingProp == nil {
		// Create new property
		property = &models.DomainProperty{
			ID:           uuid.New(),
			Fingerprint:  fingerprint,
			Country:      "CA",
			Province:     provinceFromPostalCode(raw.PostalCode),
			City:         raw.City,
			PostalCode:   raw.PostalCode,
			AddressFull:  raw.Address,
			PropertyType: raw.PropertyType,
			Beds:         intPtr(raw.Beds),
			Baths:        intPtr(raw.Baths),
			SqFt:         intPtr(raw.SqFt),
			Floor:        1,
			Stories:      1,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if err := s.store.UpsertProperty(ctx, property); err != nil {
			return nil, fmt.Errorf("create property: %w", err)
		}
		result.IsNewProperty = true
		result.PropertyID = property.ID

		// Insert potential matches for new properties
		if s.match != nil {
			if _, err := s.match.InsertPotentialMatches(ctx, property); err != nil {
				log.Printf("Warning: failed to insert property matches: %v", err)
			}
		}
	} else {
		property = existingProp
		result.PropertyID = property.ID

		// Update property fields that may have changed
		property.PostalCode = raw.PostalCode
		property.UpdatedAt = now
		if err := s.store.UpsertProperty(ctx, property); err != nil {
			return nil, fmt.Errorf("update property: %w", err)
		}
	}

	// 2. Add MLS as property identifier
	if raw.MLS != "" {
		identifier := &models.PropertyIdentifier{
			PropertyID: property.ID,
			Type:       models.IdentifierTypeMLS,
			Identifier: raw.MLS,
			Source:     source,
		}
		if err := s.store.UpsertPropertyIdentifier(ctx, identifier); err != nil {
			log.Printf("Warning: failed to upsert identifier: %v", err)
		}
	}

	// 3. Find or create listing
	existingListing, err := s.store.GetListingBySourceAndExternalID(ctx, source, raw.MLS)
	if err != nil {
		return nil, fmt.Errorf("get listing: %w", err)
	}

	var listing *models.Listing
	var previousPrice *float64

	if existingListing == nil {
		// Check if this is a relist (property has different active listing)
		prevListing, _ := s.store.GetActiveListingForProperty(ctx, property.ID)
		if prevListing != nil && prevListing.ExternalID != raw.MLS {
			// Delist the old one first
			if err := s.store.UpdateListingStatus(ctx, prevListing.ID, models.ListingStatusDelisted, &now); err != nil {
				log.Printf("Warning: failed to delist previous listing: %v", err)
			}
			result.IsRelisted = true
		}

		// New listing
		listing = &models.Listing{
			ID:           uuid.New(),
			PropertyID:   property.ID,
			Source:       source,
			ExternalID:   raw.MLS,
			URL:          raw.URL,
			Type:         "sale", // Default to sale, can be enriched later
			Status:       models.ListingStatusActive,
			Price:        float64Ptr(float64(raw.Price)),
			Currency:     "CAD",
			PropertyType: raw.PropertyType,
			Beds:         intPtr(raw.Beds),
			Baths:        intPtr(raw.Baths),
			SqFt:         intPtr(raw.SqFt),
			Description:  raw.Description,
			RawData:      raw.Data,
			LastSeen:     now,
			ListedAt:     now,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if err := s.store.UpsertListing(ctx, listing); err != nil {
			return nil, fmt.Errorf("create listing: %w", err)
		}
		result.IsNewListing = true
		result.ListingID = listing.ID
	} else {
		listing = existingListing
		result.ListingID = listing.ID
		previousPrice = listing.Price

		// Update listing (if was delisted, quietly restore - probably false positive)
		listing.URL = raw.URL
		listing.Status = models.ListingStatusActive
		listing.Price = float64Ptr(float64(raw.Price))
		listing.Description = raw.Description
		listing.RawData = raw.Data
		listing.LastSeen = now
		listing.UpdatedAt = now
		listing.DelistedAt = nil

		if err := s.store.UpsertListing(ctx, listing); err != nil {
			return nil, fmt.Errorf("update listing: %w", err)
		}

		// Check for price change
		if previousPrice != nil && listing.Price != nil && *previousPrice != *listing.Price {
			result.PriceChanged = true
		}
	}

	// 4. Create property events
	if result.IsNewListing || result.IsRelisted {
		eventType := models.EventTypeListed
		if result.IsRelisted {
			eventType = models.EventTypeRelisted
		}

		event := &models.PropertyEvent{
			PropertyID: property.ID,
			EventType:  eventType,
			EventDate:  now,
			Price:      listing.Price,
			SourceType: "listing",
			Source:     "scraper",
			CreatedAt:  now,
		}
		if err := s.store.CreatePropertyEvent(ctx, event); err != nil {
			log.Printf("Warning: failed to create %s event: %v", eventType, err)
		} else {
			result.EventsCreated++
		}
	}

	if result.PriceChanged {
		event := &models.PropertyEvent{
			PropertyID:    property.ID,
			EventType:     models.EventTypePriceChange,
			EventDate:     now,
			Price:         listing.Price,
			PreviousPrice: previousPrice,
			SourceType:    "listing",
			Source:        "scraper",
			CreatedAt:     now,
		}
		if err := s.store.CreatePropertyEvent(ctx, event); err != nil {
			log.Printf("Warning: failed to create price_change event: %v", err)
		} else {
			result.EventsCreated++
		}
	}

	// 5. Create price point
	if listing.Price != nil && *listing.Price > 0 {
		priceType := models.PriceTypeAskingSale
		// TODO: detect rent vs sale from listing data

		pricePoint := &models.PricePoint{
			PropertyID:  property.ID,
			ListingID:   &listing.ID,
			PriceType:   priceType,
			Amount:      *listing.Price,
			Currency:    "CAD",
			Period:      "one_time",
			EffectiveAt: now,
			Source:      "scraper",
			CreatedAt:   now,
		}
		if err := s.store.CreatePricePoint(ctx, pricePoint); err != nil {
			log.Printf("Warning: failed to create price point: %v", err)
		}
	}

	// 6. Create property link
	if raw.URL != "" {
		link := &models.PropertyLink{
			PropertyID:  property.ID,
			ListingID:   &listing.ID,
			URL:         raw.URL,
			Site:        source,
			LinkType:    "listing",
			IsPrimary:   true,
			IsActive:    true,
			FirstSeenAt: now,
			LastSeenAt:  now,
		}
		if err := s.store.UpsertPropertyLink(ctx, link); err != nil {
			log.Printf("Warning: failed to upsert property link: %v", err)
		}
	}

	// 7. Queue media (photos)
	if s.media != nil && len(raw.Photos) > 0 {
		for i, photoURL := range raw.Photos {
			mediaID, err := s.media.Enqueue(ctx, EnqueueParams{
				OriginalURL: photoURL,
				MediaType:   "image",
				Category:    models.MediaCategoryListing,
				Province:    property.Province,
				City:        property.City,
			})
			if err != nil {
				log.Printf("Warning: failed to queue media %s: %v", photoURL, err)
				continue
			}

			// Link media to listing
			listingMedia := &models.ListingMedia{
				ListingID: listing.ID,
				MediaID:   mediaID,
				Position:  i,
			}
			if err := s.store.UpsertListingMedia(ctx, listingMedia); err != nil {
				log.Printf("Warning: failed to link media to listing: %v", err)
			}
			result.MediaQueued++
		}
	}

	// 8. Process realtor info (brokerage + agents)
	if raw.Realtor != nil {
		if err := s.processRealtor(ctx, raw.Realtor, listing.ID); err != nil {
			log.Printf("Warning: failed to process realtor: %v", err)
		}
	}

	return result, nil
}

func (s *ListingService) processRealtor(ctx context.Context, realtor *models.Realtor, listingID uuid.UUID) error {
	now := time.Now()

	// Process brokerage
	var brokerageID *uuid.UUID
	if realtor.Company.Name != "" {
		existing, err := s.store.GetBrokerageByName(ctx, realtor.Company.Name)
		if err != nil {
			return fmt.Errorf("get brokerage: %w", err)
		}

		if existing != nil {
			brokerageID = &existing.ID
			// Update logo if we didn't have one before
			if existing.LogoID == nil && s.media != nil && realtor.Company.Logo != "" {
				logoID, err := s.media.Enqueue(ctx, EnqueueParams{
					OriginalURL: realtor.Company.Logo,
					MediaType:   "image",
					Category:    models.MediaCategoryBrokerage,
				})
				if err == nil {
					existing.LogoID = &logoID
					s.store.UpsertBrokerage(ctx, existing)
				}
			}
		} else {
			brokerage := &models.Brokerage{
				ID:        uuid.New(),
				Name:      realtor.Company.Name,
				Phone:     realtor.Company.Phone,
				Address:   realtor.Company.Address,
				Country:   "CA",
				CreatedAt: now,
			}
			// Queue brokerage logo if available
			if s.media != nil && realtor.Company.Logo != "" {
				logoID, err := s.media.Enqueue(ctx, EnqueueParams{
					OriginalURL: realtor.Company.Logo,
					MediaType:   "image",
					Category:    models.MediaCategoryBrokerage,
				})
				if err == nil {
					brokerage.LogoID = &logoID
				}
			}
			if err := s.store.UpsertBrokerage(ctx, brokerage); err != nil {
				return fmt.Errorf("create brokerage: %w", err)
			}
			brokerageID = &brokerage.ID
		}
	}

	// Process agents
	for _, agentData := range realtor.Agents {
		if agentData.Name == "" {
			continue
		}

		existing, err := s.store.GetAgentByNameAndBrokerage(ctx, agentData.Name, brokerageID)
		if err != nil {
			log.Printf("Warning: failed to get agent: %v", err)
			continue
		}

		var agent *models.Agent
		if existing != nil {
			agent = existing
			agent.LastSeenAt = now
			if agentData.Phone != "" {
				agent.Phone = agentData.Phone
			}
		} else {
			agent = &models.Agent{
				ID:          uuid.New(),
				FullName:    agentData.Name,
				Phone:       agentData.Phone,
				BrokerageID: brokerageID,
				FirstSeenAt: now,
				LastSeenAt:  now,
				CreatedAt:   now,
			}
		}

		// Queue agent headshot if available
		if s.media != nil && agentData.Photo != "" {
			mediaID, err := s.media.Enqueue(ctx, EnqueueParams{
				OriginalURL: agentData.Photo,
				MediaType:   "image",
				Category:    models.MediaCategoryAgent,
			})
			if err == nil {
				agent.HeadshotID = &mediaID
			}
		}

		if err := s.store.UpsertAgent(ctx, agent); err != nil {
			log.Printf("Warning: failed to upsert agent: %v", err)
			continue
		}

		// Link agent to listing
		listingAgent := &models.ListingAgent{
			ListingID: listingID,
			AgentID:   agent.ID,
			Role:      "listing",
		}
		if err := s.store.UpsertListingAgent(ctx, listingAgent); err != nil {
			log.Printf("Warning: failed to link agent to listing: %v", err)
		}
	}

	return nil
}

// MarkDelisted marks a listing as delisted and creates a delisted event
func (s *ListingService) MarkDelisted(ctx context.Context, listingID uuid.UUID) error {
	now := time.Now()

	// Get the listing first
	listing, err := s.store.GetListingByID(ctx, listingID)
	if err != nil {
		return fmt.Errorf("get listing: %w", err)
	}
	if listing == nil {
		return fmt.Errorf("listing not found: %s", listingID)
	}

	// Update listing status
	if err := s.store.UpdateListingStatus(ctx, listingID, models.ListingStatusDelisted, &now); err != nil {
		return fmt.Errorf("update listing status: %w", err)
	}

	// Create delisted event
	event := &models.PropertyEvent{
		PropertyID: listing.PropertyID,
		EventType:  models.EventTypeDelisted,
		EventDate:  now,
		Price:      listing.Price,
		SourceType: "listing",
		Source:     "scraper",
		CreatedAt:  now,
	}
	if err := s.store.CreatePropertyEvent(ctx, event); err != nil {
		log.Printf("Warning: failed to create delisted event: %v", err)
	}

	return nil
}

// Helper functions
func intPtr(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}

func float64Ptr(v float64) *float64 {
	if v == 0 {
		return nil
	}
	return &v
}

// provinceFromPostalCode extracts province from Canadian postal code first letter
func provinceFromPostalCode(postalCode string) string {
	if len(postalCode) == 0 {
		return ""
	}
	switch postalCode[0] {
	case 'A', 'a':
		return "NL" // Newfoundland and Labrador
	case 'B', 'b':
		return "NS" // Nova Scotia
	case 'C', 'c':
		return "PE" // Prince Edward Island
	case 'E', 'e':
		return "NB" // New Brunswick
	case 'G', 'g', 'H', 'h', 'J', 'j':
		return "QC" // Quebec
	case 'K', 'k', 'L', 'l', 'M', 'm', 'N', 'n', 'P', 'p':
		return "ON" // Ontario
	case 'R', 'r':
		return "MB" // Manitoba
	case 'S', 's':
		return "SK" // Saskatchewan
	case 'T', 't':
		return "AB" // Alberta
	case 'V', 'v':
		return "BC" // British Columbia
	case 'X', 'x':
		return "NT" // Northwest Territories / Nunavut
	case 'Y', 'y':
		return "YT" // Yukon
	}
	return ""
}

// ProcessStats tracks aggregate statistics for a scrape run
type ProcessStats struct {
	ListingsProcessed int
	PropertiesNew     int
	ListingsNew       int
	Relisted          int
	PriceChanges      int
	Errors            int
}

// Aggregate adds a ProcessResult to the stats
func (s *ProcessStats) Aggregate(r *ProcessResult) {
	s.ListingsProcessed++
	if r.IsNewProperty {
		s.PropertiesNew++
	}
	if r.IsNewListing {
		s.ListingsNew++
	}
	if r.IsRelisted {
		s.Relisted++
	}
	if r.PriceChanged {
		s.PriceChanges++
	}
}

// ToJSON returns JSON-serializable metadata
func (s *ProcessStats) ToJSON() json.RawMessage {
	data, _ := json.Marshal(map[string]int{
		"listings_processed": s.ListingsProcessed,
		"properties_new":     s.PropertiesNew,
		"listings_new":       s.ListingsNew,
		"relisted":           s.Relisted,
		"price_changes":      s.PriceChanges,
		"errors":             s.Errors,
	})
	return data
}
