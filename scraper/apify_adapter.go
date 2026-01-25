package scraper

import (
	"encoding/json"
	"fmt"

	"tct_scrooper/config"
	"tct_scrooper/models"
)

// ApifyActorAdapter defines the interface for Apify actor-specific logic
type ApifyActorAdapter interface {
	ActorID() string
	BuildInput(region config.Region, isIncremental bool) map[string]interface{}
	ParseListing(data json.RawMessage) (models.RawListing, error)
	FilterListings(listings []models.RawListing, region config.Region) []models.RawListing
}

// GetApifyAdapter returns the appropriate adapter for the given actor type
func GetApifyAdapter(actorType string) (ApifyActorAdapter, error) {
	switch actorType {
	case "canadesk", "canadesk/realtor-canada":
		return &CanadeskAdapter{}, nil
	case "scrapemind", "scrapemind/realtor-ca-scraper":
		return &ScrapemindAdapter{}, nil
	default:
		return nil, fmt.Errorf("unknown apify actor type: %s", actorType)
	}
}
