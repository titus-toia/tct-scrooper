package services

import (
	"context"
	"log"
	"time"

	"tct_scrooper/models"
	"tct_scrooper/storage"
)

// HealthcheckService handles listing health monitoring
type HealthcheckService struct {
	store   *storage.PostgresStore
	listing *ListingService
}

// NewHealthcheckService creates a new HealthcheckService
func NewHealthcheckService(store *storage.PostgresStore, listing *ListingService) *HealthcheckService {
	return &HealthcheckService{
		store:   store,
		listing: listing,
	}
}

// GetStaleListings returns active listings that haven't been seen recently
func (s *HealthcheckService) GetStaleListings(ctx context.Context, staleDuration time.Duration, limit int) ([]models.Listing, error) {
	return s.store.GetStaleActiveListings(ctx, staleDuration, limit)
}

// MarkDelisted marks a listing as delisted and creates a delisted event
func (s *HealthcheckService) MarkDelisted(ctx context.Context, listing *models.Listing) error {
	now := time.Now()

	// Update listing status
	if err := s.store.UpdateListingStatus(ctx, listing.ID, models.ListingStatusDelisted, &now); err != nil {
		return err
	}

	// Create delisted event
	event := &models.PropertyEvent{
		PropertyID: listing.PropertyID,
		EventType:  models.EventTypeDelisted,
		EventDate:  now,
		Price:      listing.Price,
		SourceType: "listing",
		Source:     "healthcheck",
		CreatedAt:  now,
	}
	if err := s.store.CreatePropertyEvent(ctx, event); err != nil {
		log.Printf("Warning: failed to create delisted event: %v", err)
	}

	return nil
}

// TouchListing updates the last_seen timestamp for a listing
func (s *HealthcheckService) TouchListing(ctx context.Context, listing *models.Listing) error {
	now := time.Now()
	listing.LastSeen = now
	listing.UpdatedAt = now
	return s.store.UpsertListing(ctx, listing)
}
