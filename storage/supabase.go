package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"tct_scrooper/config"
	"tct_scrooper/models"
)

type SupabaseStore struct {
	url        string
	serviceKey string
	client     *http.Client
}

func NewSupabaseStore(cfg *config.SupabaseConfig) *SupabaseStore {
	return &SupabaseStore{
		url:        cfg.URL,
		serviceKey: cfg.ServiceKey,
		client:     &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *SupabaseStore) UpsertProperty(prop *models.SupabaseProperty) error {
	prop.LastSyncedAt = time.Now()

	data, err := json.Marshal(prop)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", s.url+"/rest/v1/properties", bytes.NewReader(data))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", s.serviceKey)
	req.Header.Set("Authorization", "Bearer "+s.serviceKey)
	req.Header.Set("Prefer", "resolution=merge-duplicates")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("supabase error %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func BuildSupabaseProperty(prop *models.Property, snapshots []models.Snapshot, siteID string) (*models.SupabaseProperty, error) {
	if len(snapshots) == 0 {
		return nil, fmt.Errorf("no snapshots for property %s", prop.ID)
	}

	history := buildHistory(snapshots)
	historyJSON, err := json.Marshal(history)
	if err != nil {
		return nil, err
	}

	latest := snapshots[len(snapshots)-1]
	var photos []string
	if latest.Data != nil {
		var data map[string]interface{}
		if err := json.Unmarshal(latest.Data, &data); err == nil {
			if p, ok := data["photos"].([]interface{}); ok {
				for _, photo := range p {
					if s, ok := photo.(string); ok {
						photos = append(photos, s)
					}
				}
			}
		}
	}

	photosJSON, _ := json.Marshal(photos)

	return &models.SupabaseProperty{
		ID:               prop.ID,
		Address:          prop.NormalizedAddress,
		City:             prop.City,
		Beds:             prop.Beds,
		BedsPlus:         prop.BedsPlus,
		Baths:            prop.Baths,
		SqFt:             prop.SqFt,
		PropertyType:     prop.PropertyType,
		CurrentPrice:     latest.Price,
		CurrentListingID: latest.ListingID,
		CurrentURL:       latest.URL,
		Description:      latest.Description,
		Photos:           photosJSON,
		Realtor:          latest.Realtor,
		IsActive:         prop.IsActive,
		History:          historyJSON,
		TimesListed:      prop.TimesListed,
		FirstSeenAt:      prop.FirstSeenAt,
		LastSeenAt:       prop.LastSeenAt,
		SiteID:           siteID,
	}, nil
}

func buildHistory(snapshots []models.Snapshot) []models.HistoryEvent {
	var events []models.HistoryEvent
	var prevSnapshot *models.Snapshot

	for i := range snapshots {
		snap := &snapshots[i]

		if prevSnapshot == nil || snap.ListingID != prevSnapshot.ListingID {
			event := "listed"
			if prevSnapshot != nil {
				event = "relisted"
				events = append(events, models.HistoryEvent{
					Event:        "delisted",
					Date:         prevSnapshot.ScrapedAt,
					ListingID:    prevSnapshot.ListingID,
					DaysOnMarket: calcDaysOnMarket(prevSnapshot, &snapshots[0]),
				})
			}

			var photos []string
			if snap.Data != nil {
				var data map[string]interface{}
				if err := json.Unmarshal(snap.Data, &data); err == nil {
					if p, ok := data["photos"].([]interface{}); ok {
						for _, photo := range p {
							if s, ok := photo.(string); ok {
								photos = append(photos, s)
							}
						}
					}
				}
			}

			events = append(events, models.HistoryEvent{
				Event:     event,
				Date:      snap.ScrapedAt,
				Price:     snap.Price,
				ListingID: snap.ListingID,
				URL:       snap.URL,
				Photos:    photos,
			})
		} else if snap.Price != prevSnapshot.Price {
			events = append(events, models.HistoryEvent{
				Event:         "price_change",
				Date:          snap.ScrapedAt,
				Price:         snap.Price,
				ListingID:     snap.ListingID,
				PreviousPrice: prevSnapshot.Price,
			})
		}

		prevSnapshot = snap
	}

	return events
}

func calcDaysOnMarket(lastSnap *models.Snapshot, firstSnap *models.Snapshot) int {
	duration := lastSnap.ScrapedAt.Sub(firstSnap.ScrapedAt)
	return int(duration.Hours() / 24)
}
