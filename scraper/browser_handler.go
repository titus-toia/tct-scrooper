package scraper

/*
Realtor.ca API Response Structure Reference
============================================
GET /api/v1/PropertySearch (intercepted via browser)

{
  "ErrorCode": { "Id": 200, "Description": "Success - OK", ... },
  "Paging": {
    "RecordsPerPage": 12,
    "CurrentPage": 1,
    "TotalRecords": 752,
    "MaxRecords": 600,
    "TotalPages": 50,
    "RecordsShowing": 600
  },
  "Results": [{
    "Id": "29279012",
    "MlsNumber": "26001716",
    "PublicRemarks": "Full property description text...",
    "PostalCode": "N8P0E6",
    "ProvinceName": "Ontario",
    "StatusId": "1",
    "TimeOnRealtor": "1 day ago",
    "InsertedDateUTC": "639046946815570000",
    "PhotoChangeDateUTC": "2026-01-22 4:04:41 PM",
    "RelativeURLEn": "/real-estate/29279012/939-chateau-windsor",
    "RelativeURLFr": "/immobilier/29279012/939-chateau-windsor",

    "Building": {
      "BathroomTotal": "3",
      "Bedrooms": "3",          // Can be "3" or "3 + 1" (+ basement)
      "HalfBathTotal": "1",
      "StoriesTotal": "2",
      "SizeInterior": "2360.0000",
      "Type": "House",          // House, Row / Townhouse, etc.
      "Ammenities": "Fireplace(s)",
      "FloorAreaMeasurements": [{
        "Area": "2360 sqft",
        "Type": "Square Footage",
        "MeasureUnitId": "1"
      }]
    },

    "Property": {
      "Price": "$1,149,900",
      "PriceUnformattedValue": "1149900",
      "ShortValue": "1.15M",
      "Type": "Single Family",
      "TypeId": "300",
      "OwnershipType": "Freehold",
      "ParkingType": "Attached Garage, Garage, Inside Entry",
      "Address": {
        "AddressText": "939 Chateau|Windsor, Ontario N8P0E6",
        "Longitude": "-82.908614",
        "Latitude": "42.33104",
        "PermitShowAddress": true
      },
      "Photo": [{
        "SequenceId": "1",
        "HighResPath": "https://cdn.realtor.ca/listings/.../highres/.../26001716_1.jpg",
        "MedResPath": "https://cdn.realtor.ca/listings/.../medres/.../26001716_1.jpg",
        "LowResPath": "https://cdn.realtor.ca/listings/.../lowres/.../26001716_1.jpg",
        "LastUpdated": "2026-01-22 11:04:41 AM"
      }],
      "Parking": [{ "Name": "Attached Garage" }, { "Name": "Garage" }]
    },

    "Land": {
      "SizeTotal": "59.62 X 121.92 FT",   // or "105.46 X 107.93 / 0.261 AC"
      "SizeFrontage": "55 ft, 3 in",
      "LandscapeFeatures": "Landscaped"
    },

    "Individual": [{                       // Array of agents (can be 1-3+)
      "IndividualID": 1631864,
      "Name": "FRAN GREBENC",
      "FirstName": "FRAN",
      "LastName": "GREBENC",
      "Position": "Sales Person",          // or "Broker", "REALTOR®", etc.
      "Photo": "https://cdn.realtor.ca/individual/.../lowres/1163226.jpg",
      "PhotoHighRes": "https://cdn.realtor.ca/individual/.../highres/1163226.jpg",
      "Phones": [{
        "PhoneType": "Telephone",
        "PhoneNumber": "735-7222",
        "AreaCode": "519"
      }],
      "Organization": {
        "OrganizationID": 187768,
        "Name": "ROYAL LEPAGE BINDER REAL ESTATE",
        "Logo": "https://cdn.realtor.ca/organization/.../royallepage.gif",
        "Address": { "AddressText": "13158 TECUMSEH ROAD EAST|TECUMSEH, Ontario N8N3T6" },
        "Phones": [{ "AreaCode": "519", "PhoneNumber": "735-7222" }]
      },
      "RelativeDetailsURL": "/agent/1631864/fran-grebenc-..."
    }],

    "OpenHouse": [{                        // Optional, only if scheduled
      "StartTime": "Jan 24/26 - 12:00 PM To 2:00 PM",
      "StartDateTime": "24/01/2026 12:00:00 PM",
      "EndDateTime": "24/01/2026 2:00:00 PM"
    }],

    "AlternateURL": {                      // Optional
      "DetailsLink": "https://rlpbinder.ca/listing/...",
      "VideoLink": "https://youriguide.com/..."
    },

    "Media": [{                            // Optional - floor plans, videos, etc.
      "MediaCategoryId": "2",
      "MediaCategoryURL": "https://youriguide.com/...",
      "Description": "VideoTourWebsite",
      "VideoType": "iGuide"
    }, {
      "MediaCategoryId": "8",
      "MediaCategoryURL": "https://.../floorplan.pdf",
      "Description": "FloorPlan"
    }],

    "Tags": [{
      "Label": "1 day ago",
      "HTMLColorCode": "#23A1C0",
      "ListingTagTypeID": "1"
    }],

    "Business": {}                         // Empty for residential
  }]
}

Key parsing notes:
- Address format: "Street|City, Province PostalCode" (split on |)
- Bedrooms can be "3" or "3 + 1" (basement bedroom)
- Price is formatted string, use PriceUnformattedValue for int
- Individual[] is array - listings can have multiple agents
- Photos are in Property.Photo[], get HighResPath
*/

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/playwright-community/playwright-go"
	"tct_scrooper/config"
	"tct_scrooper/models"
	"tct_scrooper/storage"
)

const (
	listingsPerPage = 12
	minPageDelay    = 15 * time.Second
	maxPageDelay    = 25 * time.Second
)

type BrowserHandler struct {
	cfg         *config.SiteConfig
	store       *storage.SQLiteStore
	pw          *playwright.Playwright
	context     playwright.BrowserContext
	mu          sync.Mutex
	initialized bool

	activePage     playwright.Page
	currentPageNum int
	warmedUp       bool
	lastGeoID      string
	lastGeoName    string
	warmupListings map[int][]models.RawListing
}

func NewBrowserHandler(cfg *config.SiteConfig) *BrowserHandler {
	return &BrowserHandler{cfg: cfg}
}

func (h *BrowserHandler) SetStore(store *storage.SQLiteStore) {
	h.store = store
}

func (h *BrowserHandler) ID() string {
	return h.cfg.ID
}

func (h *BrowserHandler) Scrape(ctx context.Context, region config.Region) ([]models.RawListing, error) {
	if h.cfg.ID == "realtor_ca" {
		return h.scrapeRealtorCA(ctx, region)
	}
	return nil, fmt.Errorf("unknown site: %s", h.cfg.ID)
}

func (h *BrowserHandler) scrapeRealtorCA(ctx context.Context, region config.Region) ([]models.RawListing, error) {
	if err := h.ensureBrowser(); err != nil {
		return nil, err
	}

	if err := h.startSession(region); err != nil {
		return nil, err
	}
	defer h.Close()

	var allListings []models.RawListing

	for page := 1; ; page++ {
		listings, err := h.navigateToPage(page)
		if err != nil {
			log.Printf("Error on page %d: %v", page, err)
			break
		}

		if len(listings) == 0 {
			log.Printf("No more listings at page %d", page)
			break
		}

		allListings = append(allListings, listings...)
		log.Printf("Page %d: %d listings (total: %d)", page, len(listings), len(allListings))

		if len(listings) < listingsPerPage {
			log.Printf("Partial page, scrape complete")
			break
		}

		// Human-like delay between pages
		delay := minPageDelay + time.Duration(rand.Intn(int(maxPageDelay-minPageDelay)))
		log.Printf("Sleeping %.1fs before next page", delay.Seconds())
		time.Sleep(delay)
	}

	return allListings, nil
}

func (h *BrowserHandler) ensureBrowser() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.initialized {
		return nil
	}

	var err error
	h.pw, err = playwright.Run()
	if err != nil {
		return fmt.Errorf("failed to start playwright: %w", err)
	}

	cwd, _ := os.Getwd()
	userDataDir := filepath.Join(cwd, "browser_data")
	h.context, err = h.pw.Chromium.LaunchPersistentContext(userDataDir, playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless: playwright.Bool(false),
		Args: []string{
			"--disable-blink-features=AutomationControlled",
			"--disable-dev-shm-usage",
			"--no-sandbox",
		},
	})
	if err != nil {
		return fmt.Errorf("failed to launch browser: %w", err)
	}

	h.initialized = true
	return nil
}

func (h *BrowserHandler) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.activePage != nil {
		h.activePage.Close()
		h.activePage = nil
	}
	if h.context != nil {
		h.context.Close()
	}
	if h.pw != nil {
		h.pw.Stop()
	}
	h.initialized = false
	h.warmedUp = false
}

func (h *BrowserHandler) startSession(region config.Region) error {
	log.Printf("Starting new browsing session for %s", region.GeoName)

	page, err := h.context.NewPage()
	if err != nil {
		return fmt.Errorf("failed to create page: %w", err)
	}
	h.activePage = page
	h.lastGeoID = region.GeoID
	h.lastGeoName = region.GeoName
	h.currentPageNum = 0
	h.warmupListings = make(map[int][]models.RawListing)

	h.setupAPIIntercept()

	searchURL := fmt.Sprintf(
		"https://www.realtor.ca/map#view=list&CurrentPage=1&Sort=6-D&GeoIds=%s&GeoName=%s&PropertyTypeGroupID=1&PropertySearchTypeId=1&Currency=CAD",
		region.GeoID, url.QueryEscape(region.GeoName),
	)
	log.Printf("Navigating to: %s", searchURL)

	_, err = page.Goto(searchURL, playwright.PageGotoOptions{
		Timeout:   playwright.Float(60000),
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
	})
	if err != nil {
		log.Printf("Navigation error (continuing): %v", err)
	}

	h.humanDelay(3000, 5000)
	h.simulateHumanBehavior()
	h.handleConsent(page)

	// Warmup: click through pages 1-3
	for warmupPage := 1; warmupPage <= 3; warmupPage++ {
		if warmupPage > 1 {
			log.Printf("Warmup: clicking to page %d", warmupPage)
			if err := h.clickNextPage(); err != nil {
				log.Printf("Warmup click failed at page %d: %v", warmupPage, err)
				break
			}
			h.humanDelay(4000, 7000)
			h.simulateHumanBehavior()
		}

		h.waitForListings()
		h.currentPageNum = warmupPage

		if listings := h.parseCurrentPage(); listings != nil {
			h.warmupListings[warmupPage] = listings
			log.Printf("Warmup page %d: %d listings", warmupPage, len(listings))
		}
	}

	h.warmedUp = true
	log.Printf("Session warmed up, currently on page %d", h.currentPageNum)
	return nil
}

func (h *BrowserHandler) setupAPIIntercept() {
	h.activePage.OnResponse(func(response playwright.Response) {
		if contains(response.URL(), "/api/v1/PropertySearch") && response.Status() == 200 {
			go func() {
				body, err := response.Body()
				if err != nil || len(body) < 500 {
					return
				}
				h.activePage.Evaluate(fmt.Sprintf(`window.__apiResponse = %q`, string(body)))
				log.Printf("Intercepted /api/v1/PropertySearch: %d bytes", len(body))
			}()
		}
	})
}

func (h *BrowserHandler) parseCurrentPage() []models.RawListing {
	result, err := h.activePage.Evaluate(`window.__apiResponse`)
	if err != nil || result == nil {
		return nil
	}
	str, ok := result.(string)
	if !ok {
		return nil
	}
	data := []byte(str)
	if len(data) == 0 {
		return nil
	}
	listings, err := h.parseRealtorCAResponse(data)
	if err != nil {
		log.Printf("Failed to parse page response: %v", err)
		return nil
	}
	return listings
}

func (h *BrowserHandler) navigateToPage(targetPage int) ([]models.RawListing, error) {
	if listings, ok := h.warmupListings[targetPage]; ok {
		log.Printf("Page %d: returning warmup data (%d listings)", targetPage, len(listings))
		delete(h.warmupListings, targetPage)
		return listings, nil
	}

	page := h.activePage
	page.Evaluate(`window.__apiResponse = null`)

	needsJump := targetPage < h.currentPageNum || targetPage > h.currentPageNum+5
	if needsJump {
		jumpTo := targetPage - 2
		if jumpTo < 1 {
			jumpTo = 1
		}
		log.Printf("Jumping from page %d to %d via dropdown (target: %d)", h.currentPageNum, jumpTo, targetPage)

		if err := h.selectPageFromDropdown(jumpTo); err != nil {
			log.Printf("Dropdown jump failed: %v, trying URL fallback", err)
			searchURL := fmt.Sprintf(
				"https://www.realtor.ca/map#view=list&CurrentPage=%d&Sort=6-D&GeoIds=%s&GeoName=%s&PropertyTypeGroupID=1&PropertySearchTypeId=1&Currency=CAD",
				jumpTo, h.lastGeoID, url.QueryEscape(h.lastGeoName),
			)
			page.Goto(searchURL, playwright.PageGotoOptions{
				Timeout:   playwright.Float(60000),
				WaitUntil: playwright.WaitUntilStateDomcontentloaded,
			})
		}
		h.humanDelay(3000, 5000)
		h.simulateHumanBehavior()
		h.waitForListings()
		h.currentPageNum = jumpTo
	}

	for h.currentPageNum < targetPage {
		log.Printf("Clicking from page %d to %d", h.currentPageNum, h.currentPageNum+1)
		if err := h.clickNextPage(); err != nil {
			return nil, fmt.Errorf("failed to click to page %d: %w", h.currentPageNum+1, err)
		}
		h.currentPageNum++

		if h.currentPageNum < targetPage {
			h.humanDelay(2000, 4000)
		}
	}

	h.waitForListings()

	listings := h.parseCurrentPage()
	if listings == nil {
		content, _ := page.Content()
		page.Screenshot(playwright.PageScreenshotOptions{Path: playwright.String("debug_screenshot.png")})
		os.WriteFile("debug_page.html", []byte(content), 0644)
		return nil, fmt.Errorf("no API response on page %d (saved debug files)", targetPage)
	}

	return listings, nil
}

func (h *BrowserHandler) selectPageFromDropdown(pageNum int) error {
	page := h.activePage
	page.Evaluate(`window.__apiResponse = null`)

	dropdown := page.Locator(".paginationInnerCon .select2-selection").First()
	if visible, _ := dropdown.IsVisible(); !visible {
		return fmt.Errorf("page dropdown not visible")
	}

	dropdown.Click()
	h.humanDelay(300, 500)

	option := page.Locator(fmt.Sprintf(".select2-results__option:has-text('%d')", pageNum)).First()
	if visible, _ := option.IsVisible(); !visible {
		page.Keyboard().Press("Escape")
		return fmt.Errorf("page %d not in dropdown", pageNum)
	}

	option.Click()
	log.Printf("Selected page %d from dropdown", pageNum)
	return nil
}

func (h *BrowserHandler) clickNextPage() error {
	page := h.activePage
	page.Evaluate(`window.__apiResponse = null`)

	nextSelectors := []string{
		"a.lnkNextResultsPage",
		"a[aria-label='Go to the next page']",
	}

	var clicked bool
	for _, sel := range nextSelectors {
		btn := page.Locator(sel).First()
		if visible, _ := btn.IsVisible(); visible {
			if disabled, _ := btn.GetAttribute("disabled"); disabled == "" {
				btn.Click()
				clicked = true
				log.Printf("Clicked next button: %s", sel)
				break
			}
		}
	}

	if !clicked {
		return fmt.Errorf("could not find clickable next button")
	}

	h.humanDelay(1000, 2000)
	return nil
}

func (h *BrowserHandler) waitForListings() {
	page := h.activePage

	for i := 0; i < 20; i++ {
		page.WaitForTimeout(500)

		result, _ := page.Evaluate(`window.__apiResponse`)
		if result != nil && result.(string) != "" {
			log.Println("API response received")
			return
		}

		content, _ := page.Content()
		if trigger := h.detectIncapsula(content); trigger != "" {
			log.Printf("Incapsula detected: %s", trigger)
			h.handleIncapsula(page)
		}
	}
	log.Println("Timeout waiting for listings")
}

func (h *BrowserHandler) simulateHumanBehavior() {
	page := h.activePage

	page.Mouse().Move(float64(300+rand.Intn(400)), float64(200+rand.Intn(300)))
	page.WaitForTimeout(float64(200 + rand.Intn(300)))
	page.Mouse().Move(float64(400+rand.Intn(300)), float64(300+rand.Intn(200)))
	page.WaitForTimeout(float64(200 + rand.Intn(300)))

	scrollAmount := 100 + rand.Intn(300)
	page.Evaluate(fmt.Sprintf(`window.scrollBy(0, %d)`, scrollAmount))
}

func (h *BrowserHandler) humanDelay(minMs, maxMs int) {
	delay := minMs + rand.Intn(maxMs-minMs)
	time.Sleep(time.Duration(delay) * time.Millisecond)
}

func (h *BrowserHandler) detectIncapsula(content string) string {
	if contains(content, "listingCard") || contains(content, "ResultsPaginationCon") {
		return ""
	}

	triggers := []string{
		"Request unsuccessful. Incapsula",
		"Incapsula incident ID",
		"Access Denied",
		"This request was blocked",
	}
	for _, t := range triggers {
		if contains(content, t) {
			return t
		}
	}
	return ""
}

func (h *BrowserHandler) handleIncapsula(page playwright.Page) {
	content, _ := page.Content()
	trigger := h.detectIncapsula(content)
	if trigger == "" {
		return
	}

	log.Printf("Handling Incapsula challenge (trigger: %s)...", trigger)

	page.WaitForTimeout(2000)

	clickSelectors := []string{
		"iframe#main-iframe",
		"[id*='checkbox']",
		"input[type='checkbox']",
		"button:has-text('Verify')",
		"button:has-text('Continue')",
		"a:has-text('Click')",
		"div[class*='verify']",
	}

	for _, selector := range clickSelectors {
		el := page.Locator(selector).First()
		if visible, _ := el.IsVisible(); visible {
			log.Printf("Clicking Incapsula element: %s", selector)
			el.Click()
			page.WaitForTimeout(3000)
			break
		}
	}

	frames := page.Frames()
	for _, frame := range frames {
		if frame == page.MainFrame() {
			continue
		}

		log.Println("Checking iframe for clickable elements...")

		iframeSelectors := []string{
			"[id*='checkbox']",
			"input[type='checkbox']",
			"button",
			"a",
			"div[role='button']",
			"span[role='checkbox']",
		}

		for _, selector := range iframeSelectors {
			el := frame.Locator(selector).First()
			if visible, _ := el.IsVisible(); visible {
				log.Printf("Clicking iframe element: %s", selector)
				el.Click()
				page.WaitForTimeout(3000)

				newContent, _ := page.Content()
				if !contains(newContent, "Incapsula") {
					log.Println("Incapsula challenge passed!")
					return
				}
			}
		}
	}

	iframe := page.Locator("iframe#main-iframe").First()
	if visible, _ := iframe.IsVisible(); visible {
		log.Println("Clicking center of Incapsula iframe...")
		iframe.Click()
		page.WaitForTimeout(5000)
	}
}

func (h *BrowserHandler) handleConsent(page playwright.Page) {
	consentSelectors := []string{
		"button:has-text('Consent')",
		"button:text-is('Consent')",
		"button[id*='accept']",
		"button[class*='accept']",
		"button[class*='consent']",
		"#didomi-notice-agree-button",
		"button:has-text('Accept')",
		"button:has-text('Accept All')",
		"button:has-text('I Accept')",
		"button:has-text('Agree')",
		"button:has-text('OK')",
	}

	for _, selector := range consentSelectors {
		btn := page.Locator(selector).First()
		if visible, _ := btn.IsVisible(); visible {
			log.Printf("Clicking consent button: %s", selector)
			btn.Click()
			page.WaitForTimeout(2000)
			break
		}
	}
}

func (h *BrowserHandler) parseRealtorCAResponse(data []byte) ([]models.RawListing, error) {
	var rawResp struct {
		Results []json.RawMessage `json:"Results"`
		Paging  json.RawMessage   `json:"Paging"`
	}
	if err := json.Unmarshal(data, &rawResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var listings []models.RawListing
	for _, rawResult := range rawResp.Results {
		var r realtorCAResult
		if err := json.Unmarshal(rawResult, &r); err != nil {
			log.Printf("Failed to parse listing: %v", err)
			continue
		}

		beds, bedsPlus := parseBedsInterface(r.Building.Bedrooms)

		listing := models.RawListing{
			ID:           fmt.Sprintf("%v", r.ID),
			MLS:          r.MlsNumber,
			Address:      r.Property.Address.AddressText,
			City:         extractCityFromAddress(r.Property.Address.AddressText),
			PostalCode:   r.PostalCode,
			Price:        parsePriceString(r.Property.Price),
			Beds:         beds,
			BedsPlus:     bedsPlus,
			Baths:        toInt(r.Building.BathroomTotal),
			SqFt:         parseSqFtString(r.Building.SizeInterior),
			PropertyType: r.Property.Type,
			URL:          "https://www.realtor.ca" + r.RelativeURLEn,
			Photos:       extractPhotoURLs(r.Property.Photo),
			Description:  r.PublicRemarks,
			Realtor:      extractRealtor(r.Individual),
			Data:         rawResult,
		}

		listings = append(listings, listing)
	}

	log.Printf("Parsed %d listings from response", len(listings))
	return listings, nil
}

func extractRealtor(individuals []struct {
	IndividualID int    `json:"IndividualID"`
	Name         string `json:"Name"`
	Photo        string `json:"Photo"`
	Phones       []struct {
		PhoneType   string `json:"PhoneType"`
		PhoneNumber string `json:"PhoneNumber"`
		AreaCode    string `json:"AreaCode"`
	} `json:"Phones"`
	Organization struct {
		OrganizationID int    `json:"OrganizationID"`
		Name           string `json:"Name"`
		Logo           string `json:"Logo"`
		Address        struct {
			AddressText string `json:"AddressText"`
		} `json:"Address"`
		Phones []struct {
			PhoneType   string `json:"PhoneType"`
			PhoneNumber string `json:"PhoneNumber"`
			AreaCode    string `json:"AreaCode"`
		} `json:"Phones"`
	} `json:"Organization"`
}) *models.Realtor {
	if len(individuals) == 0 {
		return nil
	}

	firstOrg := individuals[0].Organization
	realtor := &models.Realtor{
		Company: models.RealtorCompany{
			ID:      firstOrg.OrganizationID,
			Name:    firstOrg.Name,
			Phone:   formatPhone(firstOrg.Phones),
			Address: firstOrg.Address.AddressText,
			Logo:    firstOrg.Logo,
		},
	}

	for _, ind := range individuals {
		agent := models.RealtorAgent{
			ID:    ind.IndividualID,
			Name:  ind.Name,
			Phone: formatPhone(ind.Phones),
			Photo: ind.Photo,
		}
		realtor.Agents = append(realtor.Agents, agent)
	}

	return realtor
}

func formatPhone(phones []struct {
	PhoneType   string `json:"PhoneType"`
	PhoneNumber string `json:"PhoneNumber"`
	AreaCode    string `json:"AreaCode"`
}) string {
	for _, p := range phones {
		if p.PhoneType == "Telephone" && p.AreaCode != "" && p.PhoneNumber != "" {
			return p.AreaCode + "-" + p.PhoneNumber
		}
	}
	if len(phones) > 0 && phones[0].AreaCode != "" && phones[0].PhoneNumber != "" {
		return phones[0].AreaCode + "-" + phones[0].PhoneNumber
	}
	return ""
}

type realtorCASearchResp struct {
	Results []realtorCAResult `json:"Results"`
	Paging  struct {
		TotalRecords int `json:"TotalRecords"`
		TotalPages   int `json:"TotalPages"`
	} `json:"Paging"`
}

type realtorCAResult struct {
	ID            interface{} `json:"Id"`
	MlsNumber     string      `json:"MlsNumber"`
	PublicRemarks string      `json:"PublicRemarks"`
	PostalCode    string      `json:"PostalCode"`
	RelativeURLEn string      `json:"RelativeURLEn"`
	Property      struct {
		Price   string `json:"Price"`
		Type    string `json:"Type"`
		Address struct {
			AddressText string `json:"AddressText"`
		} `json:"Address"`
		Photo []struct {
			HighResPath string `json:"HighResPath"`
			LowResPath  string `json:"LowResPath"`
		} `json:"Photo"`
	} `json:"Property"`
	Building struct {
		Bedrooms      interface{} `json:"Bedrooms"`
		BathroomTotal interface{} `json:"BathroomTotal"`
		SizeInterior  string      `json:"SizeInterior"`
	} `json:"Building"`
	Individual []struct {
		IndividualID int    `json:"IndividualID"`
		Name         string `json:"Name"`
		Photo        string `json:"Photo"`
		Phones       []struct {
			PhoneType   string `json:"PhoneType"`
			PhoneNumber string `json:"PhoneNumber"`
			AreaCode    string `json:"AreaCode"`
		} `json:"Phones"`
		Organization struct {
			OrganizationID int    `json:"OrganizationID"`
			Name           string `json:"Name"`
			Logo           string `json:"Logo"`
			Address        struct {
				AddressText string `json:"AddressText"`
			} `json:"Address"`
			Phones []struct {
				PhoneType   string `json:"PhoneType"`
				PhoneNumber string `json:"PhoneNumber"`
				AreaCode    string `json:"AreaCode"`
			} `json:"Phones"`
		} `json:"Organization"`
	} `json:"Individual"`
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func extractCityFromAddress(address string) string {
	var parts []string
	var current string
	for _, c := range address {
		if c == '|' || c == ',' {
			if trimmed := trimStr(current); trimmed != "" {
				parts = append(parts, trimmed)
			}
			current = ""
		} else {
			current += string(c)
		}
	}
	if trimmed := trimStr(current); trimmed != "" {
		parts = append(parts, trimmed)
	}
	if len(parts) >= 2 {
		return parts[len(parts)-2]
	}
	return ""
}

func trimStr(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

func parsePriceString(price string) int {
	var result int
	for _, c := range price {
		if c >= '0' && c <= '9' {
			result = result*10 + int(c-'0')
		}
	}
	return result
}

func parseSqFtString(size string) int {
	var result int
	for _, c := range size {
		if c >= '0' && c <= '9' {
			result = result*10 + int(c-'0')
		}
	}
	return result
}

func toInt(v interface{}) int {
	switch val := v.(type) {
	case int:
		return val
	case float64:
		return int(val)
	case string:
		var result int
		started := false
		for _, c := range val {
			if c >= '0' && c <= '9' {
				result = result*10 + int(c-'0')
				started = true
			} else if started {
				break
			}
		}
		return result
	default:
		return 0
	}
}

func parseBedsInterface(v interface{}) (int, int) {
	s, ok := v.(string)
	if !ok {
		return toInt(v), 0
	}
	return parseBedrooms(s)
}

func extractPhotoURLs(photos []struct {
	HighResPath string `json:"HighResPath"`
	LowResPath  string `json:"LowResPath"`
}) []string {
	var urls []string
	for _, p := range photos {
		if p.HighResPath != "" {
			urls = append(urls, p.HighResPath)
		} else if p.LowResPath != "" {
			urls = append(urls, p.LowResPath)
		}
	}
	return urls
}
