package scraper

import (
	"os"
	"path/filepath"
	"testing"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", name, err)
	}
	return data
}

func TestParseRealtorCAResponse_Basic(t *testing.T) {
	handler := &BrowserHandler{}
	data := loadFixture(t, "realtor_ca_basic.json")

	listings, err := handler.parseRealtorCAResponse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(listings) != 1 {
		t.Fatalf("expected 1 listing, got %d", len(listings))
	}

	listing := listings[0]
	if listing.MLS != "26001716" {
		t.Fatalf("expected MLS 26001716, got %s", listing.MLS)
	}
	if listing.City != "Windsor" {
		t.Fatalf("expected city Windsor, got %s", listing.City)
	}
	if listing.Price != 1149900 {
		t.Fatalf("expected price 1149900, got %d", listing.Price)
	}
	if listing.Beds != 3 || listing.BedsPlus != 1 {
		t.Fatalf("expected beds 3 + 1, got %d + %d", listing.Beds, listing.BedsPlus)
	}
	if listing.Baths != 3 {
		t.Fatalf("expected baths 3, got %d", listing.Baths)
	}
	if listing.SqFt != 2360 {
		t.Fatalf("expected sqft 2360, got %d", listing.SqFt)
	}
	if listing.URL != "https://www.realtor.ca/real-estate/29279012/939-chateau-windsor" {
		t.Fatalf("unexpected URL %s", listing.URL)
	}
	if len(listing.Photos) != 2 {
		t.Fatalf("expected 2 photos, got %d", len(listing.Photos))
	}
	if listing.Photos[0] != "https://cdn.realtor.ca/listings/26001716_1.jpg" {
		t.Fatalf("unexpected first photo %s", listing.Photos[0])
	}
	if listing.Photos[1] != "https://cdn.realtor.ca/listings/26001716_2.jpg" {
		t.Fatalf("unexpected second photo %s", listing.Photos[1])
	}
	if listing.Realtor == nil {
		t.Fatalf("expected realtor details")
	}
	if listing.Realtor.Company.Name != "ROYAL LEPAGE BINDER REAL ESTATE" {
		t.Fatalf("unexpected brokerage name %s", listing.Realtor.Company.Name)
	}
	if len(listing.Realtor.Agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(listing.Realtor.Agents))
	}
	if listing.Realtor.Agents[0].Photo == "" {
		t.Fatalf("expected agent photo")
	}
}

func TestParseRealtorCAResponse_Variation(t *testing.T) {
	handler := &BrowserHandler{}
	data := loadFixture(t, "realtor_ca_variation.json")

	listings, err := handler.parseRealtorCAResponse(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(listings) != 1 {
		t.Fatalf("expected 1 listing, got %d", len(listings))
	}

	listing := listings[0]
	if listing.MLS != "X1234567" {
		t.Fatalf("expected MLS X1234567, got %s", listing.MLS)
	}
	if listing.City != "Toronto" {
		t.Fatalf("expected city Toronto, got %s", listing.City)
	}
	if listing.Price != 499000 {
		t.Fatalf("expected price 499000, got %d", listing.Price)
	}
	if listing.Beds != 2 || listing.BedsPlus != 0 {
		t.Fatalf("expected beds 2 + 0, got %d + %d", listing.Beds, listing.BedsPlus)
	}
	if listing.Realtor != nil {
		t.Fatalf("expected no realtor info")
	}
	if len(listing.Photos) != 0 {
		t.Fatalf("expected 0 photos, got %d", len(listing.Photos))
	}
}
