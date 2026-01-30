package workers

import (
	"context"
	"log"
	"net/http"
	"time"

	"tct_scrooper/models"
	"tct_scrooper/storage"
)

// HealthcheckWorker checks if active listings are still live
type HealthcheckWorker struct {
	store      *storage.PostgresStore
	httpClient *http.Client
	proxyURL   string
}

// NewHealthcheckWorker creates a new healthcheck worker
func NewHealthcheckWorker(store *storage.PostgresStore, proxyURL string) *HealthcheckWorker {
	// Create client that doesn't follow redirects
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Don't follow redirects
		},
	}

	// TODO: configure proxy transport if proxyURL is set

	return &HealthcheckWorker{
		store:      store,
		httpClient: client,
		proxyURL:   proxyURL,
	}
}

// CheckResult contains the outcome of checking a listing
type CheckResult struct {
	IsLive     bool
	StatusCode int
	Error      error
}

// Check fetches a listing URL and determines if it's still active
func (w *HealthcheckWorker) Check(ctx context.Context, url string) CheckResult {
	req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return CheckResult{Error: err}
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "text/html")

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return CheckResult{Error: err}
	}
	defer resp.Body.Close()

	result := CheckResult{StatusCode: resp.StatusCode}

	// 200 = still active
	// 404, 410 = delisted
	// 301, 302 to different path = likely delisted
	switch resp.StatusCode {
	case 200:
		result.IsLive = true
	case 404, 410:
		result.IsLive = false
	case 301, 302:
		// Check if redirect goes to a "not found" or search page
		location := resp.Header.Get("Location")
		if isDelistRedirect(location) {
			result.IsLive = false
		} else {
			result.IsLive = true // Might just be URL normalization
		}
	default:
		// For other codes (500, 503, etc.), assume still live but log
		result.IsLive = true
	}

	return result
}

// isDelistRedirect checks if a redirect URL indicates delisting
func isDelistRedirect(location string) bool {
	// Common patterns for delisted redirects on realtor.ca
	delistPatterns := []string{
		"/map",
		"/search",
		"PropertySearchTypeId",
		"notfound",
		"error",
	}

	for _, pattern := range delistPatterns {
		if contains(location, pattern) {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Run starts the healthcheck worker loop
func (w *HealthcheckWorker) Run(ctx context.Context, staleDuration time.Duration, batchSize int, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Healthcheck worker stopping")
			return
		case <-ticker.C:
			w.processBatch(ctx, staleDuration, batchSize)
		}
	}
}

func (w *HealthcheckWorker) processBatch(ctx context.Context, staleDuration time.Duration, batchSize int) {
	listings, err := w.store.GetStaleActiveListings(ctx, staleDuration, batchSize)
	if err != nil {
		log.Printf("Healthcheck: query error: %v", err)
		return
	}

	if len(listings) == 0 {
		return
	}

	log.Printf("Healthcheck: checking %d stale listings", len(listings))

	var checked, delisted int
	for _, listing := range listings {
		if listing.URL == "" {
			continue
		}

		result := w.Check(ctx, listing.URL)
		checked++

		if result.Error != nil {
			log.Printf("Healthcheck: error checking %s: %v", listing.URL, result.Error)
			// Update last_seen anyway to avoid re-checking immediately
			w.touchListing(ctx, &listing)
			continue
		}

		if !result.IsLive {
			log.Printf("Healthcheck: listing delisted (status %d): %s", result.StatusCode, listing.URL)
			if err := w.markDelisted(ctx, &listing); err != nil {
				log.Printf("Healthcheck: failed to mark delisted: %v", err)
			} else {
				delisted++
			}
		} else {
			// Still live, update last_seen
			w.touchListing(ctx, &listing)
		}

		// Rate limit between requests
		time.Sleep(500 * time.Millisecond)
	}

	if delisted > 0 {
		log.Printf("Healthcheck: checked %d, marked %d as delisted", checked, delisted)
	}
}

func (w *HealthcheckWorker) touchListing(ctx context.Context, listing *models.Listing) {
	now := time.Now()
	query := `UPDATE listings SET last_seen = $2, updated_at = $2 WHERE id = $1`
	w.store.Pool().Exec(ctx, query, listing.ID, now)
}

func (w *HealthcheckWorker) markDelisted(ctx context.Context, listing *models.Listing) error {
	now := time.Now()

	// Update listing status
	if err := w.store.UpdateListingStatus(ctx, listing.ID, models.ListingStatusDelisted, &now); err != nil {
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
	if err := w.store.CreatePropertyEvent(ctx, event); err != nil {
		log.Printf("Healthcheck: failed to create delisted event: %v", err)
	}

	// Mark property link as inactive
	linkQuery := `UPDATE property_links SET is_active = false, last_seen_at = $2 WHERE listing_id = $1`
	w.store.Pool().Exec(ctx, linkQuery, listing.ID, now)

	return nil
}
