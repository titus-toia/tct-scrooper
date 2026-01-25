package scraper

import (
	"context"

	"tct_scrooper/config"
	"tct_scrooper/models"
)

type Handler interface {
	ID() string
	Scrape(ctx context.Context, region config.Region) ([]models.RawListing, error)
}

func NewHandler(siteCfg *config.SiteConfig) Handler {
	switch siteCfg.Handler {
	case "api":
		return NewAPIHandler(siteCfg)
	case "browser":
		return NewBrowserHandler(siteCfg)
	case "apify":
		return NewApifyHandler(siteCfg)
	default:
		return NewAPIHandler(siteCfg)
	}
}
