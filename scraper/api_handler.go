package scraper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"tct_scrooper/config"
	"tct_scrooper/models"
)

type APIHandler struct {
	cfg    *config.SiteConfig
	client *http.Client
}

func NewAPIHandler(cfg *config.SiteConfig) *APIHandler {
	return &APIHandler{
		cfg: cfg,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (h *APIHandler) ID() string {
	return h.cfg.ID
}

func (h *APIHandler) Scrape(ctx context.Context, region config.Region) ([]models.RawListing, error) {
	if h.cfg.ID == "realtor_ca" {
		return h.scrapeRealtorCA(ctx, region)
	}
	return nil, fmt.Errorf("unknown site: %s", h.cfg.ID)
}

func (h *APIHandler) scrapeRealtorCA(ctx context.Context, region config.Region) ([]models.RawListing, error) {
	var allListings []models.RawListing
	recordsPerPage := 200

	for page := 1; ; page++ {
		log.Printf("API: fetching page %d for %s", page, region.GeoName)

		listings, err := h.fetchRealtorCAPage(ctx, region, page, recordsPerPage)
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", page, err)
		}

		if len(listings) == 0 {
			log.Printf("API: no more listings at page %d", page)
			break
		}

		allListings = append(allListings, listings...)
		log.Printf("API: page %d: %d listings (total: %d)", page, len(listings), len(allListings))

		if len(listings) < recordsPerPage {
			log.Printf("API: partial page, scrape complete")
			break
		}
	}

	return allListings, nil
}

func (h *APIHandler) fetchRealtorCAPage(ctx context.Context, region config.Region, page, recordsPerPage int) ([]models.RawListing, error) {
	endpoint := h.cfg.Endpoints["search"]

	reqBody := map[string]interface{}{
		"LatitudeMax":          region.LatMax,
		"LatitudeMin":          region.LatMin,
		"LongitudeMax":         region.LngMax,
		"LongitudeMin":         region.LngMin,
		"PropertySearchTypeId": 1,
		"TransactionTypeId":    2,
		"RecordsPerPage":       recordsPerPage,
		"CurrentPage":          page,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Origin", "https://www.realtor.ca")
	req.Header.Set("Referer", "https://www.realtor.ca/")

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("realtor.ca API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result realtorCASearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var listings []models.RawListing
	for _, r := range result.Results {
		listing := models.RawListing{
			ID:           fmt.Sprintf("%d", r.ID),
			MLS:          r.MlsNumber,
			Address:      r.Property.Address.AddressText,
			City:         extractCity(r.Property.Address.AddressText),
			Price:        parsePrice(r.Property.Price),
			Beds:         r.Building.Bedrooms,
			Baths:        r.Building.BathroomTotal,
			SqFt:         parseSqFt(r.Building.SizeInterior),
			PropertyType: r.Property.Type,
			URL:          "https://www.realtor.ca" + r.RelativeURLEn,
			Photos:       extractPhotos(r.Property.Photo),
		}

		data, _ := json.Marshal(r)
		listing.Data = data
		listings = append(listings, listing)
	}

	return listings, nil
}

type realtorCASearchResponse struct {
	Results []realtorCAListing `json:"Results"`
	Paging  struct {
		RecordsPerPage int `json:"RecordsPerPage"`
		CurrentPage    int `json:"CurrentPage"`
		TotalRecords   int `json:"TotalRecords"`
		MaxRecords     int `json:"MaxRecords"`
		TotalPages     int `json:"TotalPages"`
		RecordsShowing int `json:"RecordsShowing"`
		Pins           int `json:"Pins"`
	} `json:"Paging"`
}

type realtorCAListing struct {
	ID            int    `json:"Id"`
	MlsNumber     string `json:"MlsNumber"`
	RelativeURLEn string `json:"RelativeURLEn"`
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
		Bedrooms      int    `json:"Bedrooms"`
		BathroomTotal int    `json:"BathroomTotal"`
		SizeInterior  string `json:"SizeInterior"`
	} `json:"Building"`
}

func parsePrice(price string) int {
	var result int
	for _, c := range price {
		if c >= '0' && c <= '9' {
			result = result*10 + int(c-'0')
		}
	}
	return result
}

func parseSqFt(size string) int {
	var result int
	for _, c := range size {
		if c >= '0' && c <= '9' {
			result = result*10 + int(c-'0')
		}
	}
	return result
}

func extractCity(address string) string {
	parts := splitAddress(address)
	if len(parts) >= 2 {
		return parts[len(parts)-2]
	}
	return ""
}

func splitAddress(addr string) []string {
	var parts []string
	var current string
	for _, c := range addr {
		if c == '|' || c == ',' {
			if current != "" {
				parts = append(parts, trimSpace(current))
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, trimSpace(current))
	}
	return parts
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

func extractPhotos(photos []struct {
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
