package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/google/uuid"
	"tct_scrooper/services"
	"tct_scrooper/storage"
)

// EnrichmentWorker fetches listing pages and extracts detailed property info
type EnrichmentWorker struct {
	store        *storage.PostgresStore
	mediaService *services.MediaService
	httpClient   *http.Client
	proxyURL     string
}

// NewEnrichmentWorker creates a new enrichment worker
func NewEnrichmentWorker(store *storage.PostgresStore, mediaService *services.MediaService, proxyURL string) *EnrichmentWorker {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// TODO: configure proxy transport if proxyURL is set

	return &EnrichmentWorker{
		store:        store,
		mediaService: mediaService,
		httpClient:   client,
		proxyURL:     proxyURL,
	}
}

// EnrichedData contains all data extracted from the listing page
type EnrichedData struct {
	// Photos
	Photos []string `json:"photos"`

	// Property summary
	PropertyType      string `json:"property_type"`
	BuildingType      string `json:"building_type"`
	Stories           int    `json:"stories"`
	SqFt              int    `json:"sqft"`
	NeighbourhoodName string `json:"neighbourhood_name"`
	Title             string `json:"title"` // Freehold, Condo, etc.
	YearBuilt         int    `json:"year_built"`
	ParkingType       string `json:"parking_type"`
	TimeOnMarket      string `json:"time_on_market"`

	// Building details
	BathroomsTotal   int      `json:"bathrooms_total"`
	BathroomsPartial int      `json:"bathrooms_partial"`
	Appliances       []string `json:"appliances"`
	BasementType     string   `json:"basement_type"`
	Features         []string `json:"features"`
	Style            string   `json:"style"`
	FireProtection   string   `json:"fire_protection"`
	BuildingAmenities []string `json:"building_amenities"`
	Structures       []string `json:"structures"`

	// HVAC
	Cooling      string `json:"cooling"`
	HeatingType  string `json:"heating_type"`
	FireplaceType string `json:"fireplace_type"`

	// Neighbourhood
	AmenitiesNearby []string `json:"amenities_nearby"`

	// Parking
	TotalParkingSpaces int `json:"total_parking_spaces"`

	// Rooms
	Rooms []Room `json:"rooms"`

	// Land
	Fencing     string `json:"fencing"`
	LotFeatures []string `json:"lot_features"`

	// Legal
	LegalDescription string `json:"legal_description"`

	// Agent info
	Agent     *AgentInfo     `json:"agent"`
	Brokerage *BrokerageInfo `json:"brokerage"`

	// Full description (may be longer than Apify)
	Description string `json:"description"`
}

// Room represents a room with dimensions
type Room struct {
	Floor      string `json:"floor"`
	Name       string `json:"name"`
	Dimensions string `json:"dimensions"` // Imperial format
}

// AgentInfo contains agent details
type AgentInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Title    string `json:"title"`
	Phone    string `json:"phone"`
	URL      string `json:"url"`
}

// BrokerageInfo contains brokerage details
type BrokerageInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Address string `json:"address"`
	Phone   string `json:"phone"`
	Website string `json:"website"`
}

// Enrich fetches a listing URL and extracts detailed data
func (w *EnrichmentWorker) Enrich(ctx context.Context, listingURL string) (*EnrichedData, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", listingURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 || resp.StatusCode == 301 || resp.StatusCode == 302 {
		return nil, fmt.Errorf("listing not found: %d", resp.StatusCode)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	return w.ParseHTML(resp.Body)
}

// ParseHTML parses the HTML and extracts enriched data
func (w *EnrichmentWorker) ParseHTML(r io.Reader) (*EnrichedData, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}

	data := &EnrichedData{}

	// Extract photos from image grid
	doc.Find("#imageListOuterCon img.gridViewListingImage").Each(func(i int, s *goquery.Selection) {
		if src, exists := s.Attr("src"); exists && src != "" {
			data.Photos = append(data.Photos, src)
		}
	})

	// Extract description
	data.Description = strings.TrimSpace(doc.Find("#propertyDescriptionCon").Text())

	// Property summary section
	data.PropertyType = w.extractValue(doc, "#propertyDetailsSectionContentSubCon_PropertyType")
	data.BuildingType = w.extractValue(doc, "#propertyDetailsSectionContentSubCon_BuildingType")
	data.Stories = w.extractInt(doc, "#propertyDetailsSectionContentSubCon_Stories")
	data.SqFt = w.extractSqFt(doc, "#propertyDetailsSectionContentSubCon_SquareFootage")
	data.NeighbourhoodName = w.extractValue(doc, "#propertyDetailsSectionContentSubCon_NeighborhoodName")
	data.Title = w.extractValue(doc, "#propertyDetailsSectionContentSubCon_Title")
	data.YearBuilt = w.extractInt(doc, "#propertyDetailsSectionContentSubCon_BuiltIn")
	data.ParkingType = w.extractValue(doc, "#propertyDetailsSectionContentSubCon_ParkingType")
	data.TimeOnMarket = w.extractValue(doc, "#propertyDetailsSectionContentSubCon_TimeOnRealtor")

	// Building section - Bathrooms
	data.BathroomsTotal = w.extractInt(doc, "#propertyDetailsSectionVal_Total")
	data.BathroomsPartial = w.extractInt(doc, "#propertyDetailsSectionVal_Partial")

	// Interior features
	data.Appliances = w.extractList(doc, "#propertyDetailsSectionVal_AppliancesIncluded")
	data.BasementType = w.extractValue(doc, "#propertyDetailsSectionVal_BasementType")

	// Building features
	data.Features = w.extractList(doc, "#propertyDetailsSectionVal_Features")
	data.Style = w.extractValue(doc, "#propertyDetailsSectionVal_Style")
	data.FireProtection = w.extractValue(doc, "#propertyDetailsSectionVal_FireProtection")
	data.BuildingAmenities = w.extractList(doc, "#propertyDetailsSectionVal_BuildingAmenities")
	data.Structures = w.extractList(doc, "#propertyDetailsSectionVal_Structures")

	// HVAC
	data.Cooling = w.extractValue(doc, "#propertyDetailsSectionVal_Cooling")
	data.HeatingType = w.extractValue(doc, "#propertyDetailsSectionVal_HeatingType")
	data.FireplaceType = w.extractValue(doc, "#propertyDetailsSectionVal_FireplaceType")

	// Neighbourhood
	data.AmenitiesNearby = w.extractList(doc, "#propertyDetailsSectionVal_AmenitiesNearby")

	// Parking
	data.TotalParkingSpaces = w.extractInt(doc, "#propertyDetailsSectionVal_TotalParkingSpaces")

	// Rooms
	data.Rooms = w.extractRooms(doc)

	// Land
	data.Fencing = w.extractValue(doc, "#propertyDetailsSectionContentSubCon_Fencing")

	// Legal description
	data.LegalDescription = strings.TrimSpace(doc.Find("#propertyLegalDescriptionCon").Text())

	// Agent info
	data.Agent = w.extractAgent(doc)

	// Brokerage info
	data.Brokerage = w.extractBrokerage(doc)

	return data, nil
}

func (w *EnrichmentWorker) extractValue(doc *goquery.Document, selector string) string {
	return strings.TrimSpace(doc.Find(selector + " .propertyDetailsSectionContentValue").Text())
}

func (w *EnrichmentWorker) extractInt(doc *goquery.Document, selector string) int {
	text := w.extractValue(doc, selector)
	var num int
	fmt.Sscanf(text, "%d", &num)
	return num
}

func (w *EnrichmentWorker) extractSqFt(doc *goquery.Document, selector string) int {
	text := w.extractValue(doc, selector)
	// Parse "1888 sqft" -> 1888
	var num int
	fmt.Sscanf(text, "%d", &num)
	return num
}

func (w *EnrichmentWorker) extractList(doc *goquery.Document, selector string) []string {
	text := w.extractValue(doc, selector)
	if text == "" {
		return nil
	}
	parts := strings.Split(text, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func (w *EnrichmentWorker) extractRooms(doc *goquery.Document) []Room {
	var rooms []Room
	var currentFloor string

	doc.Find(".listingDetailsRoomDetailsCon").Each(func(i int, s *goquery.Selection) {
		floor := strings.TrimSpace(s.Find(".listingDetailsRoomDetails_Floor").Text())
		if floor != "" {
			currentFloor = floor
		}

		name := strings.TrimSpace(s.Find(".listingDetailsRoomDetails_Room").Text())
		dims := strings.TrimSpace(s.Find(".listingDetailsRoomDetails_Dimensions.Imperial").Text())

		if name != "" {
			rooms = append(rooms, Room{
				Floor:      currentFloor,
				Name:       name,
				Dimensions: dims,
			})
		}
	})

	return rooms
}

func (w *EnrichmentWorker) extractAgent(doc *goquery.Document) *AgentInfo {
	card := doc.Find(".realtorCardCon").First()
	if card.Length() == 0 {
		return nil
	}

	// Extract ID from card ID attribute (RealtorCard-1935675)
	cardID, _ := card.Attr("id")
	id := strings.TrimPrefix(cardID, "RealtorCard-")

	name := strings.TrimSpace(card.Find(".realtorCardName").Text())
	if name == "" {
		return nil
	}

	title := strings.TrimSpace(card.Find(".realtorCardTitle").Text())
	phone := strings.TrimSpace(card.Find(".realtorCardContactNumber").Text())
	url, _ := card.Find(".realtorCardDetailsLink").Attr("href")

	return &AgentInfo{
		ID:    id,
		Name:  name,
		Title: title,
		Phone: phone,
		URL:   url,
	}
}

func (w *EnrichmentWorker) extractBrokerage(doc *goquery.Document) *BrokerageInfo {
	card := doc.Find(".officeCardCon").First()
	if card.Length() == 0 {
		return nil
	}

	// Extract ID from class (OfficeCard-290197)
	class, _ := card.Attr("class")
	idMatch := regexp.MustCompile(`OfficeCard-(\d+)`).FindStringSubmatch(class)
	var id string
	if len(idMatch) > 1 {
		id = idMatch[1]
	}

	name := strings.TrimSpace(card.Find(".officeCardName").Text())
	if name == "" {
		return nil
	}

	address := strings.TrimSpace(card.Find(".officeCardAddress").Text())
	address = strings.ReplaceAll(address, "\n", " ")
	address = regexp.MustCompile(`\s+`).ReplaceAllString(address, " ")

	phone := strings.TrimSpace(card.Find(".officeCardContactNumber").First().Text())
	website, _ := card.Find(".officeCardWebsite").Attr("href")

	return &BrokerageInfo{
		ID:      id,
		Name:    name,
		Address: address,
		Phone:   phone,
		Website: website,
	}
}

// UpdateListing updates the listing record with enriched data
func (w *EnrichmentWorker) UpdateListing(ctx context.Context, listingID uuid.UUID, data *EnrichedData) error {
	// Convert enriched data to JSON for storage in listing.features or listing.raw_data
	featuresJSON, err := json.Marshal(map[string]interface{}{
		"appliances":          data.Appliances,
		"basement_type":       data.BasementType,
		"cooling":             data.Cooling,
		"heating_type":        data.HeatingType,
		"fireplace_type":      data.FireplaceType,
		"amenities_nearby":    data.AmenitiesNearby,
		"building_amenities":  data.BuildingAmenities,
		"structures":          data.Structures,
		"fire_protection":     data.FireProtection,
		"parking_spaces":      data.TotalParkingSpaces,
		"neighbourhood_name":  data.NeighbourhoodName,
		"title_type":          data.Title,
		"rooms":               data.Rooms,
		"legal_description":   data.LegalDescription,
	})
	if err != nil {
		return fmt.Errorf("marshal features: %w", err)
	}

	// Update listing with enriched features
	query := `
		UPDATE listings SET
			features = $2,
			description = COALESCE(NULLIF($3, ''), description),
			stories = COALESCE(NULLIF($4, 0), stories),
			updated_at = NOW()
		WHERE id = $1`

	_, err = w.store.Pool().Exec(ctx, query, listingID, featuresJSON, data.Description, data.Stories)
	if err != nil {
		return fmt.Errorf("update listing: %w", err)
	}

	// Queue photos for media worker
	if w.mediaService != nil {
		for _, photoURL := range data.Photos {
			if _, err := w.mediaService.Enqueue(ctx, photoURL, "image"); err != nil {
				log.Printf("Warning: failed to queue photo %s: %v", photoURL, err)
			}
		}
	}

	// Update property with year_built if available
	if data.YearBuilt > 0 {
		propQuery := `
			UPDATE properties SET
				year_built = COALESCE(year_built, $2),
				updated_at = NOW()
			WHERE id = (SELECT property_id FROM listings WHERE id = $1)`
		w.store.Pool().Exec(ctx, propQuery, listingID, data.YearBuilt)
	}

	return nil
}

// Run starts the enrichment worker loop
func (w *EnrichmentWorker) Run(ctx context.Context, batchSize int, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Enrichment worker stopping")
			return
		case <-ticker.C:
			w.processBatch(ctx, batchSize)
		}
	}
}

func (w *EnrichmentWorker) processBatch(ctx context.Context, batchSize int) {
	// Get listings that need enrichment (no features set yet)
	// We only try listings with < 3 attempts
	query := `
		SELECT id, url, enrichment_attempts
		FROM listings
		WHERE status = 'active' AND features IS NULL AND url IS NOT NULL AND enrichment_attempts < 3
		ORDER BY created_at
		LIMIT $1`

	rows, err := w.store.Pool().Query(ctx, query, batchSize)
	if err != nil {
		log.Printf("Enrichment: query error: %v", err)
		return
	}
	defer rows.Close()

	type listingToEnrich struct {
		ID       uuid.UUID
		URL      string
		Attempts int
	}

	var listings []listingToEnrich
	for rows.Next() {
		var l listingToEnrich
		if err := rows.Scan(&l.ID, &l.URL, &l.Attempts); err != nil {
			log.Printf("Enrichment: scan error: %v", err)
			continue
		}
		listings = append(listings, l)
	}

	if len(listings) == 0 {
		return
	}

	log.Printf("Enrichment: processing %d listings", len(listings))

	for _, l := range listings {
		data, err := w.Enrich(ctx, l.URL)
		if err != nil {
			log.Printf("Enrichment: failed to enrich %s: %v", l.URL, err)
			
			// Increment attempts
			w.store.Pool().Exec(ctx, `UPDATE listings SET enrichment_attempts = enrichment_attempts + 1, updated_at = NOW() WHERE id = $1`, l.ID)
			
			// If we reached max attempts, mark as failed (by setting empty features so it stops retrying)
			if l.Attempts+1 >= 3 {
				log.Printf("Enrichment: max attempts reached for %s, giving up", l.ID)
				w.store.Pool().Exec(ctx, `UPDATE listings SET features = '{}' WHERE id = $1`, l.ID)
			}
			continue
		}

		if err := w.UpdateListing(ctx, l.ID, data); err != nil {
			log.Printf("Enrichment: failed to update %s: %v", l.ID, err)
			continue
		}

		log.Printf("Enrichment: enriched %s (%d photos, %d rooms)", l.ID, len(data.Photos), len(data.Rooms))

		// Rate limit between requests
		time.Sleep(500 * time.Millisecond)
	}
}
