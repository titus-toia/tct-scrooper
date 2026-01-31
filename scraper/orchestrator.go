package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"tct_scrooper/config"
	"tct_scrooper/models"
	"tct_scrooper/services"
	"tct_scrooper/storage"
)

type Orchestrator struct {
	cfg      *config.Config
	store    *storage.SQLiteStore
	handlers map[string]Handler
	paused   bool

	// Postgres services
	pgStore            *storage.PostgresStore
	listingService     *services.ListingService
	matchService       *services.MatchService
	mediaService       *services.MediaService
	healthcheckService *services.HealthcheckService
}

func NewOrchestrator(cfg *config.Config, store *storage.SQLiteStore) *Orchestrator {
	handlers := make(map[string]Handler)
	for id, siteCfg := range cfg.Sites {
		handler := NewHandler(siteCfg)
		if bh, ok := handler.(*BrowserHandler); ok {
			bh.SetStore(store)
		}
		if ah, ok := handler.(*ApifyHandler); ok {
			ah.SetStore(store)
		}
		handlers[id] = handler
	}

	return &Orchestrator{
		cfg:      cfg,
		store:    store,
		handlers: handlers,
	}
}

// SetServices injects the new Postgres-based services
func (o *Orchestrator) SetServices(
	pgStore *storage.PostgresStore,
	listing *services.ListingService,
	match *services.MatchService,
	media *services.MediaService,
	healthcheck *services.HealthcheckService,
) {
	o.pgStore = pgStore
	o.listingService = listing
	o.matchService = match
	o.mediaService = media
	o.healthcheckService = healthcheck
}

func (o *Orchestrator) RunAll(ctx context.Context) error {
	if o.paused {
		log.Println("Scraper is paused, skipping run")
		return nil
	}

	for siteID := range o.cfg.Sites {
		if err := o.RunSite(ctx, siteID); err != nil {
			log.Printf("Error running site %s: %v", siteID, err)
		}
	}

	return nil
}

func (o *Orchestrator) RunSite(ctx context.Context, siteID string) error {
	siteCfg, ok := o.cfg.Sites[siteID]
	if !ok {
		return fmt.Errorf("unknown site: %s", siteID)
	}

	handler, ok := o.handlers[siteID]
	if !ok {
		return fmt.Errorf("no handler for site: %s", siteID)
	}

	// Create run record (SQLite for TUI compatibility)
	run := &models.ScrapeRun{
		SiteID:    siteID,
		StartedAt: time.Now(),
		Status:    models.RunStatusRunning,
	}

	runID, err := o.store.CreateRun(run)
	if err != nil {
		return err
	}
	run.ID = runID

	// Also create run in Postgres if available
	var pgRunID *int64
	if o.pgStore != nil {
		pgRun := &models.DomainScrapeRun{
			Source:    siteID,
			StartedAt: time.Now(),
			Status:    "running",
		}
		if err := o.pgStore.CreateScrapeRun(ctx, pgRun); err != nil {
			log.Printf("Warning: failed to create Postgres run: %v", err)
		} else {
			pgRunID = &pgRun.ID
		}
	}

	o.log(run.ID, models.LogLevelInfo, fmt.Sprintf("Starting scrape for %s", siteCfg.Name), siteID)

	// Track stats for new services
	stats := &services.ProcessStats{}

	defer func() {
		now := time.Now()
		run.FinishedAt = &now
		o.store.UpdateRun(run)
		o.store.UpdateSiteStats(siteID)

		// Update Postgres run
		if pgRunID != nil {
			pgRun := &models.DomainScrapeRun{
				ID:            *pgRunID,
				FinishedAt:    &now,
				Status:        "completed",
				ListingsFound: stats.ListingsProcessed,
				ListingsNew:   stats.ListingsNew,
				PropertiesNew: stats.PropertiesNew,
				ErrorsCount:   stats.Errors,
				Metadata:      stats.ToJSON(),
			}
			if run.Status == models.RunStatusFailed {
				pgRun.Status = "failed"
			}
			o.pgStore.UpdateScrapeRun(ctx, pgRun)
		}
	}()

	for regionID, region := range siteCfg.Regions {
		o.log(run.ID, models.LogLevelInfo, fmt.Sprintf("Scraping region: %s", regionID), siteID)

		listings, err := handler.Scrape(ctx, region)
		if err != nil {
			o.log(run.ID, models.LogLevelError, fmt.Sprintf("Scrape error for %s: %v", regionID, err), siteID)
			run.ErrorsCount++
			run.Status = models.RunStatusFailed
			return err
		}

		run.ListingsFound += len(listings)
		o.log(run.ID, models.LogLevelInfo, fmt.Sprintf("Region %s: %d listings", regionID, len(listings)), siteID)

		for _, listing := range listings {
			if err := o.processListing(ctx, run, &listing, siteID, pgRunID, stats); err != nil {
				o.log(run.ID, models.LogLevelError, fmt.Sprintf("Process error for %s: %v", listing.MLS, err), siteID)
				run.ErrorsCount++
				stats.Errors++
			}
		}
	}

	run.Status = models.RunStatusCompleted
	o.log(run.ID, models.LogLevelInfo,
		fmt.Sprintf("Completed: %d found, %d new properties, %d relisted, %d price changes",
			run.ListingsFound, stats.PropertiesNew, stats.Relisted, stats.PriceChanges), siteID)

	return nil
}

func (o *Orchestrator) processListing(ctx context.Context, run *models.ScrapeRun, listing *models.RawListing, siteID string, pgRunID *int64, stats *services.ProcessStats) error {
	if o.listingService == nil {
		return fmt.Errorf("listing service not initialized")
	}

	result, err := o.listingService.ProcessListing(ctx, listing, siteID, pgRunID)
	if err != nil {
		return err
	}
	stats.Aggregate(result)

	// Update SQLite stats for TUI compatibility
	if result.IsNewProperty {
		run.PropertiesNew++
	}
	if result.IsRelisted {
		run.PropertiesRelisted++
	}
	if result.IsNewListing {
		run.ListingsNew++
	}
	return nil
}

func (o *Orchestrator) HandleCommand(cmd *models.Command) error {
	params, err := o.store.ParseCommandParams(cmd)
	if err != nil {
		return err
	}

	ctx := context.Background()

	switch cmd.Command {
	case models.CmdScrapeNow:
		return o.RunAll(ctx)
	case models.CmdScrapeSite:
		if params.Site != "" {
			return o.RunSite(ctx, params.Site)
		}
		return o.RunAll(ctx)
	case models.CmdPause:
		o.paused = true
		log.Println("Scraper paused")
	case models.CmdResume:
		o.paused = false
		log.Println("Scraper resumed")
	}

	return nil
}

func (o *Orchestrator) IsPaused() bool {
	return o.paused
}

func (o *Orchestrator) log(runID int64, level models.LogLevel, message, siteID string) {
	log.Printf("[%s] %s: %s", level, siteID, message)
	o.store.Log(&runID, level, message, siteID)
}

func (o *Orchestrator) GetSiteIDs() []string {
	var ids []string
	for id := range o.cfg.Sites {
		ids = append(ids, id)
	}
	return ids
}

func (o *Orchestrator) MarshalStatus() ([]byte, error) {
	status := map[string]interface{}{
		"paused": o.paused,
		"sites":  o.GetSiteIDs(),
	}
	return json.Marshal(status)
}
