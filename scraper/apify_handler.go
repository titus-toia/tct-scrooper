package scraper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"tct_scrooper/config"
	"tct_scrooper/models"
	"tct_scrooper/storage"
)

const (
	apifyAPIBase     = "https://api.apify.com/v2"
	apifyPollTimeout = 15 * time.Minute
	apifyPollDelay   = 10 * time.Second
)

type ApifyHandler struct {
	cfg     *config.SiteConfig
	client  *http.Client
	apiKey  string
	adapter ApifyActorAdapter
	store   *storage.SQLiteStore
}

func NewApifyHandler(cfg *config.SiteConfig) *ApifyHandler {
	actorType := cfg.ApifyActor
	if actorType == "" {
		actorType = "canadesk"
	}

	adapter, err := GetApifyAdapter(actorType)
	if err != nil {
		log.Printf("Warning: %v, using canadesk adapter", err)
		adapter = &CanadeskAdapter{}
	}

	return &ApifyHandler{
		cfg:     cfg,
		apiKey:  os.Getenv("APIFY_API_KEY"),
		client:  &http.Client{Timeout: 60 * time.Second},
		adapter: adapter,
	}
}

func (h *ApifyHandler) ID() string {
	return h.cfg.ID
}

func (h *ApifyHandler) SetStore(store *storage.SQLiteStore) {
	h.store = store
}

func (h *ApifyHandler) Scrape(ctx context.Context, region config.Region) ([]models.RawListing, error) {
	if h.apiKey == "" {
		return nil, fmt.Errorf("APIFY_API_KEY not set")
	}

	isIncremental := h.hasExistingData()
	if isIncremental {
		log.Printf("Apify: incremental scrape for %s (days=1)", region.GeoName)
	} else {
		log.Printf("Apify: full backfill for %s (days=30)", region.GeoName)
	}

	runID, err := h.startRun(ctx, region, isIncremental)
	if err != nil {
		return nil, fmt.Errorf("failed to start apify run: %w", err)
	}
	log.Printf("Apify run started: %s (actor: %s)", runID, h.adapter.ActorID())

	datasetID, err := h.waitForRun(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("apify run failed: %w", err)
	}
	log.Printf("Apify run complete, dataset: %s", datasetID)

	listings, err := h.fetchDataset(ctx, datasetID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch dataset: %w", err)
	}

	filtered := h.adapter.FilterListings(listings, region)
	log.Printf("Fetched %d listings from Apify, %d after filtering", len(listings), len(filtered))
	return filtered, nil
}

func (h *ApifyHandler) hasExistingData() bool {
	if h.store == nil {
		return false
	}
	count, err := h.store.GetPropertyCount(h.cfg.ID)
	if err != nil {
		log.Printf("Error checking property count: %v", err)
		return false
	}
	return count > 0
}

func (h *ApifyHandler) startRun(ctx context.Context, region config.Region, isIncremental bool) (string, error) {
	input := h.adapter.BuildInput(region, isIncremental)
	body, _ := json.Marshal(input)
	log.Printf("Apify input: %s", string(body))

	url := fmt.Sprintf("%s/acts/%s/runs?token=%s", apifyAPIBase, h.adapter.ActorID(), h.apiKey)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("apify start run failed %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.Data.ID, nil
}

func (h *ApifyHandler) waitForRun(ctx context.Context, runID string) (string, error) {
	url := fmt.Sprintf("%s/actor-runs/%s?token=%s", apifyAPIBase, runID, h.apiKey)
	deadline := time.Now().Add(apifyPollTimeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return "", err
		}

		resp, err := h.client.Do(req)
		if err != nil {
			time.Sleep(apifyPollDelay)
			continue
		}

		var result struct {
			Data struct {
				Status           string `json:"status"`
				DefaultDatasetID string `json:"defaultDatasetId"`
			} `json:"data"`
		}
		json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()

		switch result.Data.Status {
		case "SUCCEEDED":
			return result.Data.DefaultDatasetID, nil
		case "FAILED", "ABORTED", "TIMED-OUT":
			return "", fmt.Errorf("run %s: %s", runID, result.Data.Status)
		}

		log.Printf("Apify run status: %s", result.Data.Status)
		time.Sleep(apifyPollDelay)
	}

	return "", fmt.Errorf("timeout waiting for run %s", runID)
}

func (h *ApifyHandler) fetchDataset(ctx context.Context, datasetID string) ([]models.RawListing, error) {
	url := fmt.Sprintf("%s/datasets/%s/items?token=%s&format=json", apifyAPIBase, datasetID, h.apiKey)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("dataset fetch failed %d: %s", resp.StatusCode, string(respBody))
	}

	var items []json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, err
	}

	var listings []models.RawListing
	for _, item := range items {
		listing, err := h.adapter.ParseListing(item)
		if err != nil {
			log.Printf("Failed to parse listing: %v", err)
			continue
		}
		listings = append(listings, listing)
	}

	return listings, nil
}
