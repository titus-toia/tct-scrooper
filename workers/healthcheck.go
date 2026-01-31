package workers

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"tct_scrooper/models"
	"tct_scrooper/storage"
)

// HealthcheckWorker checks if active listings are still live and monitors price changes
type HealthcheckWorker struct {
	store      *storage.PostgresStore
	httpClient *http.Client
	proxyURL   string
}

// NewHealthcheckWorker creates a new healthcheck worker
func NewHealthcheckWorker(store *storage.PostgresStore, proxyURL string) *HealthcheckWorker {
	client := &http.Client{
		Timeout: 30 * time.Second,
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
	IsLive       bool
	StatusCode   int
	CurrentPrice *float64 // Price extracted from page (nil if not found)
	Error        error
}

// Check fetches a listing URL and determines if it's still active, also extracts current price
func (w *HealthcheckWorker) Check(ctx context.Context, url string) CheckResult {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return CheckResult{Error: err}
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return CheckResult{Error: err}
	}
	defer resp.Body.Close()

	result := CheckResult{StatusCode: resp.StatusCode}

	// 200 = still active, parse for price
	// 404, 410 = delisted
	// 301, 302 to different path = likely delisted
	switch resp.StatusCode {
	case 200:
		result.IsLive = true
		// Read body and extract price
		body, err := io.ReadAll(io.LimitReader(resp.Body, 500*1024)) // 500KB limit
		if err == nil {
			result.CurrentPrice = extractPrice(string(body))
		}
	case 404, 410:
		result.IsLive = false
	case 301, 302:
		location := resp.Header.Get("Location")
		if isDelistRedirect(location) {
			result.IsLive = false
		} else {
			result.IsLive = true
		}
	default:
		// For other codes (500, 503, etc.), assume still live but don't update price
		result.IsLive = true
	}

	return result
}

// extractPrice extracts price from HTML page using JSON-LD structured data
func extractPrice(html string) *float64 {
	// Try JSON-LD first (most reliable)
	// Look for: "price": "564900.00"
	jsonLDPattern := regexp.MustCompile(`"price"\s*:\s*"(\d+(?:\.\d+)?)"`)
	if matches := jsonLDPattern.FindStringSubmatch(html); len(matches) > 1 {
		if price, err := strconv.ParseFloat(matches[1], 64); err == nil {
			return &price
		}
	}

	// Fallback: try to find price in JavaScript variable
	// Look for: price: '564900.00',
	jsPattern := regexp.MustCompile(`price:\s*'(\d+(?:\.\d+)?)'`)
	if matches := jsPattern.FindStringSubmatch(html); len(matches) > 1 {
		if price, err := strconv.ParseFloat(matches[1], 64); err == nil {
			return &price
		}
	}

	// Fallback: try data-value attribute
	// Look for: data-value-cad="$564,900 "
	dataPattern := regexp.MustCompile(`data-value-cad="\$?([\d,]+)`)
	if matches := dataPattern.FindStringSubmatch(html); len(matches) > 1 {
		priceStr := strings.ReplaceAll(matches[1], ",", "")
		if price, err := strconv.ParseFloat(priceStr, 64); err == nil {
			return &price
		}
	}

	return nil
}

// extractJSONLD extracts and parses JSON-LD data from HTML (for future use)
func extractJSONLD(html string) map[string]interface{} {
	pattern := regexp.MustCompile(`<script[^>]*type="application/ld\+json"[^>]*>([\s\S]*?)</script>`)
	if matches := pattern.FindStringSubmatch(html); len(matches) > 1 {
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(matches[1]), &data); err == nil {
			return data
		}
	}
	return nil
}

// isDelistRedirect checks if a redirect URL indicates delisting
func isDelistRedirect(location string) bool {
	delistPatterns := []string{
		"/map",
		"/search",
		"PropertySearchTypeId",
		"notfound",
		"error",
	}

	for _, pattern := range delistPatterns {
		if strings.Contains(strings.ToLower(location), strings.ToLower(pattern)) {
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

	var checked, delisted, priceChanges int
	for _, listing := range listings {
		if listing.URL == "" {
			continue
		}

		result := w.Check(ctx, listing.URL)
		checked++

		if result.Error != nil {
			log.Printf("Healthcheck: error checking %s: %v", listing.URL, result.Error)
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
			// Check for price change
			if result.CurrentPrice != nil && listing.Price != nil {
				if *result.CurrentPrice != *listing.Price {
					log.Printf("Healthcheck: price change %s: $%.0f -> $%.0f", listing.URL, *listing.Price, *result.CurrentPrice)
					if err := w.recordPriceChange(ctx, &listing, *result.CurrentPrice); err != nil {
						log.Printf("Healthcheck: failed to record price change: %v", err)
					} else {
						priceChanges++
					}
				}
			}
			w.touchListing(ctx, &listing)
		}

		// Rate limit between requests
		time.Sleep(500 * time.Millisecond)
	}

	if delisted > 0 || priceChanges > 0 {
		log.Printf("Healthcheck: checked %d, delisted %d, price changes %d", checked, delisted, priceChanges)
	}
}

func (w *HealthcheckWorker) touchListing(ctx context.Context, listing *models.Listing) {
	now := time.Now()
	query := `UPDATE listings SET last_seen = $2, updated_at = $2 WHERE id = $1`
	w.store.Pool().Exec(ctx, query, listing.ID, now)
}

func (w *HealthcheckWorker) recordPriceChange(ctx context.Context, listing *models.Listing, newPrice float64) error {
	now := time.Now()
	previousPrice := listing.Price

	// Update listing price
	query := `UPDATE listings SET price = $2, updated_at = $3, last_seen = $3 WHERE id = $1`
	if _, err := w.store.Pool().Exec(ctx, query, listing.ID, newPrice, now); err != nil {
		return err
	}

	// Create price_change event
	event := &models.PropertyEvent{
		PropertyID:    listing.PropertyID,
		EventType:     models.EventTypePriceChange,
		EventDate:     now,
		Price:         &newPrice,
		PreviousPrice: previousPrice,
		SourceType:    "listing",
		Source:        "healthcheck",
		CreatedAt:     now,
	}
	if err := w.store.CreatePropertyEvent(ctx, event); err != nil {
		log.Printf("Healthcheck: failed to create price_change event: %v", err)
	}

	// Create price point
	pricePoint := &models.PricePoint{
		PropertyID:  listing.PropertyID,
		ListingID:   &listing.ID,
		PriceType:   models.PriceTypeAskingSale,
		Amount:      newPrice,
		Currency:    "CAD",
		Period:      "one_time",
		EffectiveAt: now,
		Source:      "healthcheck",
		CreatedAt:   now,
	}
	if err := w.store.CreatePricePoint(ctx, pricePoint); err != nil {
		log.Printf("Healthcheck: failed to create price_point: %v", err)
	}

	return nil
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
