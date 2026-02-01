package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/playwright-community/playwright-go"
	"tct_scrooper/models"
	"tct_scrooper/services"
	"tct_scrooper/storage"
)

type EnrichmentWorker struct {
	store           *storage.PostgresStore
	mediaService    *services.MediaService
	proxyURL        string
	scrapingBeeKey  string
	httpClient      *http.Client
	triggerCh       chan struct{}
	logFunc         LogFunc

	mu          sync.Mutex
	pw          *playwright.Playwright
	context     playwright.BrowserContext
	initialized bool
}

func (w *EnrichmentWorker) SetLogger(fn LogFunc) {
	w.logFunc = fn
}

func NewEnrichmentWorker(store *storage.PostgresStore, mediaService *services.MediaService, proxyURL string) *EnrichmentWorker {
	scrapingBeeKey := os.Getenv("SCRAPINGBEE_API_KEY")
	if scrapingBeeKey != "" {
		log.Printf("Enrichment: ScrapingBee API key loaded (%d chars)", len(scrapingBeeKey))
	} else {
		log.Println("Enrichment: No ScrapingBee API key, will use Playwright only")
	}

	return &EnrichmentWorker{
		store:          store,
		mediaService:   mediaService,
		proxyURL:       proxyURL,
		scrapingBeeKey: scrapingBeeKey,
		triggerCh:      make(chan struct{}, 1),
		logFunc:        NoOpLogger,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// Trigger causes the worker to run immediately
func (w *EnrichmentWorker) Trigger() {
	select {
	case w.triggerCh <- struct{}{}:
	default:
	}
}

func (w *EnrichmentWorker) ensureBrowser() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.initialized {
		return nil
	}

	var err error
	w.pw, err = playwright.Run()
	if err != nil {
		return fmt.Errorf("failed to start playwright: %w", err)
	}

	// Use same browser_data as browser_handler for shared cookies/session
	cwd, _ := os.Getwd()
	userDataDir := filepath.Join(cwd, "browser_data")

	launchOpts := playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless: playwright.Bool(false), // Headed mode to bypass Incapsula (use with xvfb)
		Args: []string{
			"--disable-blink-features=AutomationControlled",
			"--disable-dev-shm-usage",
			"--no-sandbox",
			"--disable-gpu",
		},
		UserAgent: playwright.String("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
	}

	if w.proxyURL != "" {
		proxyParsed, err := parseProxyURL(w.proxyURL)
		if err != nil {
			log.Printf("Warning: failed to parse proxy URL: %v", err)
		} else {
			launchOpts.Proxy = proxyParsed
			log.Printf("Enrichment browser using proxy: %s", proxyParsed.Server)
		}
	}

	w.context, err = w.pw.Chromium.LaunchPersistentContext(userDataDir, launchOpts)
	if err != nil {
		w.pw.Stop()
		return fmt.Errorf("failed to launch browser: %w", err)
	}

	w.initialized = true
	log.Println("Enrichment browser initialized (persistent context)")

	// Warmup to get cookies/session
	if err := w.warmup(); err != nil {
		log.Printf("Enrichment warmup failed: %v", err)
	}

	return nil
}

func (w *EnrichmentWorker) closeBrowser() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.context != nil {
		w.context.Close()
	}
	if w.pw != nil {
		w.pw.Stop()
	}
	w.initialized = false
}

func (w *EnrichmentWorker) warmup() error {
	log.Println("Enrichment: warming up browser session...")

	page, err := w.context.NewPage()
	if err != nil {
		return err
	}
	defer page.Close()

	// First visit homepage to establish initial cookies
	log.Println("Enrichment warmup: visiting homepage first")
	_, err = page.Goto("https://www.realtor.ca/", playwright.PageGotoOptions{
		Timeout:   playwright.Float(60000),
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
	})
	if err != nil {
		log.Printf("Enrichment warmup: homepage error: %v", err)
	}

	page.WaitForTimeout(3000)
	w.simulateHuman(page)

	// Handle any Incapsula on homepage
	for i := 0; i < 5; i++ {
		content, _ := page.Content()
		if !strings.Contains(content, "Incapsula") {
			log.Println("Enrichment warmup: homepage loaded")
			break
		}
		log.Println("Enrichment warmup: handling Incapsula on homepage...")
		w.handleIncapsula(page)
		page.WaitForTimeout(3000)
	}

	// Now navigate to search results
	searchURL := "https://www.realtor.ca/map#view=list&CurrentPage=1&Sort=6-D&GeoIds=g30_dpsbxd9c&GeoName=Windsor%2C+ON&PropertyTypeGroupID=1&PropertySearchTypeId=1&Currency=CAD"
	log.Printf("Enrichment warmup: navigating to search page")

	_, err = page.Goto(searchURL, playwright.PageGotoOptions{
		Timeout:   playwright.Float(60000),
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
	})
	if err != nil {
		log.Printf("Enrichment warmup: navigation error: %v", err)
	}

	// Wait and simulate human behavior
	page.WaitForTimeout(3000)
	page.Mouse().Move(float64(300+rand.Intn(400)), float64(200+rand.Intn(300)))
	page.WaitForTimeout(500)
	page.Mouse().Move(float64(400+rand.Intn(300)), float64(300+rand.Intn(200)))
	page.WaitForTimeout(500)

	// Scroll a bit
	page.Evaluate(`window.scrollBy(0, 200)`)
	page.WaitForTimeout(1000)

	// Handle consent popup
	consentSelectors := []string{
		"button:has-text('Consent')",
		"button:has-text('Accept')",
		"button:has-text('I Accept')",
		"#didomi-notice-agree-button",
	}
	for _, sel := range consentSelectors {
		btn := page.Locator(sel).First()
		if visible, _ := btn.IsVisible(); visible {
			log.Printf("Enrichment warmup: clicking consent button")
			btn.Click()
			page.WaitForTimeout(1000)
			break
		}
	}

	// Check for Incapsula and handle - wait longer
	for i := 0; i < 10; i++ {
		content, _ := page.Content()
		if strings.Contains(content, "listingCard") || strings.Contains(content, "ResultsPaginationCon") {
			log.Println("Enrichment warmup: search results loaded successfully")
			break
		}
		if strings.Contains(content, "Incapsula") {
			log.Println("Enrichment warmup: handling Incapsula challenge...")
			w.handleIncapsula(page)
		}
		page.WaitForTimeout(2000)
	}

	// Click on a listing to warm up that path too
	listingLink := page.Locator("a.listingDetailsLink").First()
	if visible, _ := listingLink.IsVisible(); visible {
		log.Println("Enrichment warmup: clicking a listing to warm up detail page")
		listingLink.Click()
		page.WaitForTimeout(5000)

		// Check for Incapsula on detail page
		content, _ := page.Content()
		if strings.Contains(content, "Incapsula") {
			log.Println("Enrichment warmup: handling Incapsula on detail page...")
			w.handleIncapsula(page)
			page.WaitForTimeout(3000)
		}
	}

	log.Println("Enrichment: warmup complete")
	return nil
}

type EnrichedData struct {
	Photos            []string `json:"photos"`
	Description       string   `json:"description"`
	PropertyType      string   `json:"property_type"`
	Stories           int      `json:"stories"`
	YearBuilt         int      `json:"year_built"`
	Appliances        []string `json:"appliances"`
	BasementType      string   `json:"basement_type"`
	Cooling           string   `json:"cooling"`
	HeatingType       string   `json:"heating_type"`
	FireplaceType     string   `json:"fireplace_type"`
	AmenitiesNearby   []string `json:"amenities_nearby"`
	BuildingAmenities []string `json:"building_amenities"`
	Structures        []string `json:"structures"`
	FireProtection    string   `json:"fire_protection"`
	TotalParkingSpaces int     `json:"total_parking_spaces"`
	NeighbourhoodName string   `json:"neighbourhood_name"`
	Title             string   `json:"title"`
	Rooms             []Room   `json:"rooms"`
	LegalDescription  string   `json:"legal_description"`
}

type Room struct {
	Type       string `json:"type"`
	Level      string `json:"level"`
	Dimensions string `json:"dimensions"`
}

func (w *EnrichmentWorker) Enrich(ctx context.Context, listingURL string) (*EnrichedData, error) {
	// Try ScrapingBee first if available (faster, cheaper - 1 credit per request)
	if w.scrapingBeeKey != "" {
		data, err := w.enrichWithScrapingBee(ctx, listingURL)
		if err == nil {
			return data, nil
		}
		// ScrapingBee failed - return error, don't fallback to Playwright
		return nil, fmt.Errorf("scrapingbee: %w", err)
	}

	// No ScrapingBee key - use Playwright
	return w.enrichWithPlaywright(ctx, listingURL)
}

func (w *EnrichmentWorker) enrichWithScrapingBee(ctx context.Context, listingURL string) (*EnrichedData, error) {
	params := url.Values{}
	params.Set("api_key", w.scrapingBeeKey)
	params.Set("url", listingURL)
	params.Set("render_js", "true") // 5 credits - full JS rendering for property details

	apiURL := "https://app.scrapingbee.com/api/v1/?" + params.Encode()

	log.Printf("Enrichment (ScrapingBee): fetching %s", listingURL)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("ScrapingBee returned %d: %s", resp.StatusCode, string(body[:min(500, len(body))]))
	}

	html := string(body)

	// Check for Incapsula block (shouldn't happen with ScrapingBee but just in case)
	if strings.Contains(html, "Request unsuccessful") && !strings.Contains(html, "listingPhotosCon") {
		return nil, fmt.Errorf("blocked by Incapsula")
	}

	// Check we got actual listing content
	if !strings.Contains(html, "cdn.realtor.ca") && !strings.Contains(html, "listingPhotosCon") {
		return nil, fmt.Errorf("no listing content found")
	}

	log.Printf("Enrichment (ScrapingBee): got %d bytes", len(body))
	return w.extractDataFromHTML(html)
}

func (w *EnrichmentWorker) enrichWithPlaywright(ctx context.Context, listingURL string) (*EnrichedData, error) {
	if err := w.ensureBrowser(); err != nil {
		return nil, err
	}

	page, err := w.context.NewPage()
	if err != nil {
		return nil, fmt.Errorf("create page: %w", err)
	}
	defer page.Close()

	page.SetViewportSize(1920, 1080)

	log.Printf("Enrichment (Playwright): navigating to %s", listingURL)
	_, err = page.Goto(listingURL, playwright.PageGotoOptions{
		Timeout:   playwright.Float(60000),
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
	})
	if err != nil {
		return nil, fmt.Errorf("navigate: %w", err)
	}

	page.WaitForTimeout(3000)
	w.simulateHuman(page)

	for attempt := 0; attempt < 5; attempt++ {
		content, _ := page.Content()

		if strings.Contains(content, "listingPhotosCon") || strings.Contains(content, "propertyDescriptionCon") {
			log.Println("Enrichment (Playwright): listing page loaded successfully")
			break
		}

		if strings.Contains(content, "Incapsula") || strings.Contains(content, "Request unsuccessful") {
			log.Printf("Enrichment (Playwright): Incapsula detected (attempt %d), handling...", attempt+1)
			w.handleIncapsula(page)
			page.WaitForTimeout(5000)
			w.simulateHuman(page)
		} else {
			page.WaitForTimeout(2000)
		}
	}

	content, _ := page.Content()
	if strings.Contains(content, "Incapsula") {
		return nil, fmt.Errorf("blocked by Incapsula after retries")
	}

	os.WriteFile("/tmp/enrich_debug.html", []byte(content), 0644)
	log.Printf("Enrichment (Playwright): saved page HTML (%d bytes)", len(content))

	return w.extractDataFromHTML(content)
}

func (w *EnrichmentWorker) extractData(page playwright.Page) (*EnrichedData, error) {
	data := &EnrichedData{}

	// Extract photos - try multiple selectors
	photoSelectors := []string{
		"#imageGrid img.gridViewListingImage",
		"#imageListOuterCon img",
		".imageGallerySidebarCon img",
		"#listingPhotosCon img",
	}

	photoSet := make(map[string]bool)
	for _, sel := range photoSelectors {
		photos, _ := page.Locator(sel).All()
		for _, photo := range photos {
			src, _ := photo.GetAttribute("src")
			dataSrc, _ := photo.GetAttribute("data-src")

			url := src
			if dataSrc != "" && strings.Contains(dataSrc, "realtor.ca") {
				url = dataSrc
			}

			if url != "" && strings.Contains(url, "realtor.ca") && !photoSet[url] {
				// Convert to high-res
				url = strings.Replace(url, "/lowres/", "/highres/", 1)
				url = strings.Replace(url, "/medres/", "/highres/", 1)
				photoSet[url] = true
				data.Photos = append(data.Photos, url)
			}
		}
	}

	// Extract description
	desc, _ := page.Locator("#propertyDescriptionCon").TextContent()
	data.Description = strings.TrimSpace(desc)

	// Extract property details
	data.PropertyType = w.extractText(page, "#propertyDetailsSectionContentSubCon_PropertyType .propertyDetailsSectionContentValue")
	data.Stories = w.extractInt(page, "#propertyDetailsSectionContentSubCon_Stories .propertyDetailsSectionContentValue")
	data.YearBuilt = w.extractInt(page, "#propertyDetailsSectionContentSubCon_BuiltIn .propertyDetailsSectionContentValue")
	data.NeighbourhoodName = w.extractText(page, "#propertyDetailsSectionContentSubCon_NeighborhoodName .propertyDetailsSectionContentValue")
	data.Title = w.extractText(page, "#propertyDetailsSectionContentSubCon_Title .propertyDetailsSectionContentValue")

	// Building details
	data.Appliances = w.extractList(page, "#propertyDetailsSectionVal_AppliancesIncluded .propertyDetailsSectionContentValue")
	data.BasementType = w.extractText(page, "#propertyDetailsSectionVal_BasementType .propertyDetailsSectionContentValue")
	data.Cooling = w.extractText(page, "#propertyDetailsSectionVal_Cooling .propertyDetailsSectionContentValue")
	data.HeatingType = w.extractText(page, "#propertyDetailsSectionVal_HeatingType .propertyDetailsSectionContentValue")
	data.FireplaceType = w.extractText(page, "#propertyDetailsSectionVal_FireplaceType .propertyDetailsSectionContentValue")
	data.FireProtection = w.extractText(page, "#propertyDetailsSectionVal_FireProtection .propertyDetailsSectionContentValue")
	data.BuildingAmenities = w.extractList(page, "#propertyDetailsSectionVal_BuildingAmenities .propertyDetailsSectionContentValue")
	data.Structures = w.extractList(page, "#propertyDetailsSectionVal_Structures .propertyDetailsSectionContentValue")
	data.AmenitiesNearby = w.extractList(page, "#propertyDetailsSectionVal_AmenitiesNearby .propertyDetailsSectionContentValue")
	data.TotalParkingSpaces = w.extractInt(page, "#propertyDetailsSectionVal_TotalParkingSpaces .propertyDetailsSectionContentValue")

	// Legal description
	legal, _ := page.Locator("#propertyLegalDescriptionCon").TextContent()
	data.LegalDescription = strings.TrimSpace(legal)

	// Rooms
	data.Rooms = w.extractRooms(page)

	return data, nil
}

func (w *EnrichmentWorker) extractText(page playwright.Page, selector string) string {
	text, err := page.Locator(selector).First().TextContent()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(text)
}

func (w *EnrichmentWorker) extractInt(page playwright.Page, selector string) int {
	text := w.extractText(page, selector)
	var num int
	fmt.Sscanf(text, "%d", &num)
	return num
}

func (w *EnrichmentWorker) extractList(page playwright.Page, selector string) []string {
	text := w.extractText(page, selector)
	if text == "" {
		return nil
	}
	var items []string
	for _, item := range strings.Split(text, ",") {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}

// extractDataFromHTML parses HTML directly (faster than Playwright locators)
func (w *EnrichmentWorker) extractDataFromHTML(html string) (*EnrichedData, error) {
	data := &EnrichedData{}

	// Extract photo URLs - look for cdn.realtor.ca listing images
	photoRegex := regexp.MustCompile(`https://cdn\.realtor\.ca/listing/[^"'\s]+\.jpg`)
	photoMatches := photoRegex.FindAllString(html, -1)

	photoSet := make(map[string]bool)
	for _, url := range photoMatches {
		// Convert to highres
		url = strings.Replace(url, "/lowres/", "/highres/", 1)
		url = strings.Replace(url, "/medres/", "/highres/", 1)
		if !photoSet[url] {
			photoSet[url] = true
			data.Photos = append(data.Photos, url)
		}
	}

	// Extract description
	descRegex := regexp.MustCompile(`id="PropertyDescription"[^>]*>([^<]+)`)
	if match := descRegex.FindStringSubmatch(html); len(match) > 1 {
		data.Description = strings.TrimSpace(match[1])
	}

	// Extract property details using ID patterns
	data.PropertyType = extractHTMLValue(html, "propertyDetailsSectionContentSubCon_PropertyType")
	data.Stories = extractHTMLInt(html, "propertyDetailsSectionContentSubCon_Stories")
	data.YearBuilt = extractHTMLInt(html, "propertyDetailsSectionContentSubCon_BuiltIn")
	data.NeighbourhoodName = extractHTMLValue(html, "propertyDetailsSectionContentSubCon_NeighborhoodName")
	data.Title = extractHTMLValue(html, "propertyDetailsSectionContentSubCon_Title")

	// Building details
	data.BasementType = extractHTMLValue(html, "propertyDetailsSectionVal_BasementType")
	data.Cooling = extractHTMLValue(html, "propertyDetailsSectionVal_Cooling")
	data.HeatingType = extractHTMLValue(html, "propertyDetailsSectionVal_HeatingType")
	data.FireplaceType = extractHTMLValue(html, "propertyDetailsSectionVal_FireplaceType")
	data.FireProtection = extractHTMLValue(html, "propertyDetailsSectionVal_FireProtection")
	data.TotalParkingSpaces = extractHTMLInt(html, "propertyDetailsSectionVal_TotalParkingSpaces")

	// Lists
	data.Appliances = extractHTMLList(html, "propertyDetailsSectionVal_AppliancesIncluded")
	data.BuildingAmenities = extractHTMLList(html, "propertyDetailsSectionVal_BuildingAmenities")
	data.Structures = extractHTMLList(html, "propertyDetailsSectionVal_Structures")
	data.AmenitiesNearby = extractHTMLList(html, "propertyDetailsSectionVal_AmenitiesNearby")

	// Legal description
	legalRegex := regexp.MustCompile(`id="propertyLegalDescriptionCon"[^>]*>([^<]+)`)
	if match := legalRegex.FindStringSubmatch(html); len(match) > 1 {
		data.LegalDescription = strings.TrimSpace(match[1])
	}

	// Extract rooms - pattern: listingDetailsRoomDetails_Room">Room Type</div>
	roomRegex := regexp.MustCompile(`listingDetailsRoomDetails_Floor">([^<]+)</div>\s*<div class="listingDetailsRoomDetails_Room">([^<]+)`)
	roomMatches := roomRegex.FindAllStringSubmatch(html, -1)
	for _, match := range roomMatches {
		if len(match) >= 3 {
			data.Rooms = append(data.Rooms, Room{
				Level: strings.TrimSpace(match[1]),
				Type:  strings.TrimSpace(match[2]),
			})
		}
	}

	log.Printf("Enrichment: extracted %d photos, %d rooms, type=%s, year=%d", len(data.Photos), len(data.Rooms), data.PropertyType, data.YearBuilt)
	return data, nil
}

// extractHTMLValue extracts text content from an element with given ID prefix
func extractHTMLValue(html, idPrefix string) string {
	// Look for: id="idPrefix"...class="propertyDetailsSectionContentValue">VALUE</
	// Use (?s) to make . match newlines
	pattern := regexp.MustCompile(`(?s)id="` + idPrefix + `"[^>]*>.*?class="propertyDetailsSectionContentValue"[^>]*>\s*([^<]+)`)
	if match := pattern.FindStringSubmatch(html); len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

// extractHTMLInt extracts an integer from HTML element
func extractHTMLInt(html, idPrefix string) int {
	val := extractHTMLValue(html, idPrefix)
	if val == "" {
		return 0
	}
	// Extract first number found
	numRegex := regexp.MustCompile(`(\d+)`)
	if match := numRegex.FindStringSubmatch(val); len(match) > 1 {
		if n, err := strconv.Atoi(match[1]); err == nil {
			return n
		}
	}
	return 0
}

// extractHTMLList extracts a comma-separated list from HTML element
func extractHTMLList(html, idPrefix string) []string {
	val := extractHTMLValue(html, idPrefix)
	if val == "" {
		return nil
	}
	parts := strings.Split(val, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func (w *EnrichmentWorker) extractRooms(page playwright.Page) []Room {
	var rooms []Room

	rows, _ := page.Locator(".propertyRoomsCon tr").All()
	for _, row := range rows {
		cols, _ := row.Locator("td").All()
		if len(cols) >= 3 {
			roomType, _ := cols[0].TextContent()
			level, _ := cols[1].TextContent()
			dims, _ := cols[2].TextContent()

			rooms = append(rooms, Room{
				Type:       strings.TrimSpace(roomType),
				Level:      strings.TrimSpace(level),
				Dimensions: strings.TrimSpace(dims),
			})
		}
	}
	return rooms
}

func (w *EnrichmentWorker) handleIncapsula(page playwright.Page) {
	// Wait for challenge to load
	page.WaitForTimeout(2000)

	// Try clicking various elements
	clickSelectors := []string{
		"iframe#main-iframe",
		"[id*='checkbox']",
		"input[type='checkbox']",
		"button:has-text('Verify')",
		"button:has-text('Continue')",
	}

	for _, sel := range clickSelectors {
		el := page.Locator(sel).First()
		if visible, _ := el.IsVisible(); visible {
			log.Printf("Enrichment: clicking %s", sel)
			el.Click()
			page.WaitForTimeout(3000)
			break
		}
	}

	// Check iframes
	for _, frame := range page.Frames() {
		if frame == page.MainFrame() {
			continue
		}

		iframeSelectors := []string{"[id*='checkbox']", "input[type='checkbox']", "button"}
		for _, sel := range iframeSelectors {
			el := frame.Locator(sel).First()
			if visible, _ := el.IsVisible(); visible {
				el.Click()
				page.WaitForTimeout(3000)
				return
			}
		}
	}
}

func (w *EnrichmentWorker) simulateHuman(page playwright.Page) {
	// Random mouse movements
	page.Mouse().Move(float64(300+rand.Intn(400)), float64(200+rand.Intn(300)))
	page.WaitForTimeout(float64(100 + rand.Intn(200)))

	// Small scroll
	page.Evaluate(fmt.Sprintf(`window.scrollBy(0, %d)`, 100+rand.Intn(200)))
	page.WaitForTimeout(float64(200 + rand.Intn(300)))
}

func (w *EnrichmentWorker) UpdateListing(ctx context.Context, listingID uuid.UUID, data *EnrichedData) error {
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

	query := `
		UPDATE listings SET
			features = $2,
			description = COALESCE(NULLIF($3, ''), description),
			stories = COALESCE(NULLIF($4, 0), stories),
			enriched_at = NOW(),
			updated_at = NOW()
		WHERE id = $1`

	_, err = w.store.Pool().Exec(ctx, query, listingID, string(featuresJSON), data.Description, data.Stories)
	if err != nil {
		return fmt.Errorf("update listing: %w", err)
	}

	// Queue photos
	if w.mediaService != nil && len(data.Photos) > 0 {
		province, city := w.getListingLocation(ctx, listingID)
		for _, photoURL := range data.Photos {
			if _, err := w.mediaService.Enqueue(ctx, services.EnqueueParams{
				OriginalURL: photoURL,
				MediaType:   "image",
				Category:    models.MediaCategoryListing,
				Province:    province,
				City:        city,
			}); err != nil {
				log.Printf("Warning: failed to queue photo %s: %v", photoURL, err)
			}
		}
	}

	// Update year_built on property
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

func (w *EnrichmentWorker) Run(ctx context.Context, batchSize int, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	defer w.closeBrowser()

	for {
		select {
		case <-ctx.Done():
			log.Println("Enrichment worker stopping")
			return
		case <-ticker.C:
			w.processBatch(ctx, batchSize)
		case <-w.triggerCh:
			log.Println("Enrichment worker triggered manually")
			w.processBatch(ctx, batchSize)
		}
	}
}

func (w *EnrichmentWorker) processBatch(ctx context.Context, batchSize int) {
	query := `
		SELECT id, url, enrichment_attempts
		FROM listings
		WHERE status = 'active' AND enriched_at IS NULL AND url IS NOT NULL AND enrichment_attempts < 3
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
			continue
		}
		listings = append(listings, l)
	}

	if len(listings) == 0 {
		return
	}

	log.Printf("Enrichment: processing %d listings", len(listings))

	var enriched, blocked, failed int
	for _, l := range listings {
		data, err := w.Enrich(ctx, l.URL)
		if err != nil {
			log.Printf("Enrichment: failed to enrich %s: %v", l.URL, err)

			w.store.Pool().Exec(ctx, `UPDATE listings SET enrichment_attempts = enrichment_attempts + 1, updated_at = NOW() WHERE id = $1`, l.ID)

			if strings.Contains(err.Error(), "Incapsula") || strings.Contains(err.Error(), "blocked") {
				blocked++
			} else {
				failed++
			}

			if l.Attempts+1 >= 3 {
				log.Printf("Enrichment: max attempts reached for %s, giving up", l.ID)
				w.logFunc(models.LogLevelWarn, "enrichment", fmt.Sprintf("Gave up after 3 attempts: %s", l.URL))
			}
			continue
		}

		if err := w.UpdateListing(ctx, l.ID, data); err != nil {
			log.Printf("Enrichment: failed to update %s: %v", l.ID, err)
			failed++
			continue
		}

		enriched++
		log.Printf("Enrichment: enriched %s (%d photos, %d rooms)", l.ID, len(data.Photos), len(data.Rooms))

		// Longer delay between requests to avoid detection
		time.Sleep(time.Duration(2000+rand.Intn(3000)) * time.Millisecond)
	}

	// Log batch summary
	if enriched > 0 || blocked > 0 || failed > 0 {
		msg := fmt.Sprintf("Batch done: %d enriched", enriched)
		if blocked > 0 {
			msg += fmt.Sprintf(", %d blocked", blocked)
		}
		if failed > 0 {
			msg += fmt.Sprintf(", %d failed", failed)
		}
		w.logFunc(models.LogLevelInfo, "enrichment", msg)
	}
}

func (w *EnrichmentWorker) getListingLocation(ctx context.Context, listingID uuid.UUID) (province, city string) {
	query := `
		SELECT p.province, p.city
		FROM listings l
		JOIN properties p ON p.id = l.property_id
		WHERE l.id = $1`
	w.store.Pool().QueryRow(ctx, query, listingID).Scan(&province, &city)
	return
}

// parseProxyURL parses a proxy URL with embedded credentials into Playwright format
func parseProxyURL(proxyURL string) (*playwright.Proxy, error) {
	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}

	proxy := &playwright.Proxy{
		Server: fmt.Sprintf("%s://%s", u.Scheme, u.Host),
	}

	if u.User != nil {
		username := u.User.Username()
		proxy.Username = &username
		if password, ok := u.User.Password(); ok {
			proxy.Password = &password
		}
	}

	return proxy, nil
}

// Keep these for compatibility but they're unused now
var _ = regexp.MustCompile
var _ = os.Getwd
var _ = filepath.Join
