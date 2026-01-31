package scheduler

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/robfig/cron/v3"
	"tct_scrooper/config"
	"tct_scrooper/httputil"
	"tct_scrooper/models"
	"tct_scrooper/scraper"
	"tct_scrooper/storage"
)

// Triggerable allows workers to be triggered manually
type Triggerable interface {
	Trigger()
}

type Scheduler struct {
	cfg          *config.Config
	orchestrator *scraper.Orchestrator
	store        *storage.SQLiteStore
	clients      *httputil.Clients
	cron         *cron.Cron
	ticker       *time.Ticker
	stopCh       chan struct{}

	mediaWorker       Triggerable
	enrichmentWorker  Triggerable
	healthcheckWorker Triggerable
}

func New(cfg *config.Config, orchestrator *scraper.Orchestrator, store *storage.SQLiteStore, clients *httputil.Clients) *Scheduler {
	return &Scheduler{
		cfg:          cfg,
		orchestrator: orchestrator,
		store:        store,
		clients:      clients,
		cron:         cron.New(),
		stopCh:       make(chan struct{}),
	}
}

// SetWorkers registers background workers for manual triggering
func (s *Scheduler) SetWorkers(media, enrichment, healthcheck Triggerable) {
	s.mediaWorker = media
	s.enrichmentWorker = enrichment
	s.healthcheckWorker = healthcheck
}

func (s *Scheduler) Start(ctx context.Context) error {
	// Always start background runners
	go s.pollCommands(ctx)
	go s.pollResumes(ctx)
	// Note: healthcheck is now handled by workers.HealthcheckWorker against Postgres

	if s.cfg.Scheduler.Cron != "" {
		log.Printf("Starting scheduler with cron: %s", s.cfg.Scheduler.Cron)
		_, err := s.cron.AddFunc(s.cfg.Scheduler.Cron, func() {
			if err := s.orchestrator.RunAll(ctx); err != nil {
				log.Printf("Scheduled run error: %v", err)
			}
		})
		if err != nil {
			return fmt.Errorf("invalid cron expression: %w", err)
		}
		s.cron.Start()
	} else if s.cfg.Scheduler.Interval > 0 {
		log.Printf("Starting scheduler with interval: %s", s.cfg.Scheduler.Interval)
		s.ticker = time.NewTicker(s.cfg.Scheduler.Interval)
		go func() {
			for {
				select {
				case <-s.ticker.C:
					if err := s.orchestrator.RunAll(ctx); err != nil {
						log.Printf("Scheduled run error: %v", err)
					}
				case <-s.stopCh:
					return
				case <-ctx.Done():
					return
				}
			}
		}()
	} else {
		log.Println("No schedule configured, daemon will only respond to commands")
	}

	return nil
}

func (s *Scheduler) Stop() {
	if s.cron != nil {
		s.cron.Stop()
	}
	if s.ticker != nil {
		s.ticker.Stop()
	}
	close(s.stopCh)
}

func (s *Scheduler) pollCommands(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cmds, err := s.store.GetPendingCommands()
			if err != nil {
				log.Printf("Error getting commands: %v", err)
				continue
			}

			for _, cmd := range cmds {
				log.Printf("Processing command: %s", cmd.Command)
				if err := s.handleCommand(&cmd); err != nil {
					log.Printf("Command error: %v", err)
				}
				if err := s.store.MarkCommandProcessed(cmd.ID); err != nil {
					log.Printf("Error marking command processed: %v", err)
				}
			}
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (s *Scheduler) handleCommand(cmd *models.Command) error {
	switch cmd.Command {
	case models.CmdRunMedia:
		if s.mediaWorker != nil {
			s.mediaWorker.Trigger()
			log.Println("Media worker triggered via command")
		}
		return nil
	case models.CmdRunEnrichment:
		if s.enrichmentWorker != nil {
			s.enrichmentWorker.Trigger()
			log.Println("Enrichment worker triggered via command")
		}
		return nil
	case models.CmdRunHealthcheck:
		if s.healthcheckWorker != nil {
			s.healthcheckWorker.Trigger()
			log.Println("Healthcheck worker triggered via command")
		}
		return nil
	default:
		return s.orchestrator.HandleCommand(cmd)
	}
}

func (s *Scheduler) TriggerNow(ctx context.Context) error {
	return s.orchestrator.RunAll(ctx)
}

const resumeDelay = 15 * time.Minute

func (s *Scheduler) pollResumes(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sites, err := s.store.GetSitesWithResumePage()
			if err != nil {
				log.Printf("Error checking resume pages: %v", err)
				continue
			}

			for _, siteID := range sites {
				lastRun, err := s.store.GetLastRunTime(siteID)
				if err != nil {
					log.Printf("Error getting last run time for %s: %v", siteID, err)
					continue
				}

				if time.Since(lastRun) >= resumeDelay {
					log.Printf("Resuming scrape for %s", siteID)
					if err := s.orchestrator.RunSite(ctx, siteID); err != nil {
						log.Printf("Resume error for %s: %v", siteID, err)
					}
				}
			}
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (s *Scheduler) pollHealthcheck(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			prop, url, err := s.store.GetOldestActiveProperty()
			if err != nil {
				log.Printf("Healthcheck error getting property: %v", err)
				continue
			}
			if prop == nil || url == "" {
				continue
			}

			req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
			if err != nil {
				log.Printf("Healthcheck error creating request: %v", err)
				continue
			}
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
			req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
			req.Header.Set("Accept-Language", "en-CA,en;q=0.9")

			resp, err := s.clients.Scraping.Do(req)
			if err != nil {
				log.Printf("Healthcheck %s: request failed: %v", prop.ID[:8], err)
				s.store.TouchProperty(prop.ID, time.Now()) // bump anyway to cycle through
				continue
			}
			resp.Body.Close()

			if resp.StatusCode == 200 {
				log.Printf("Healthcheck %s: active (200)", prop.ID[:8])
				s.store.TouchProperty(prop.ID, time.Now())
			} else if resp.StatusCode == 404 || resp.StatusCode == 301 || resp.StatusCode == 302 {
				log.Printf("Healthcheck %s: delisted (%d)", prop.ID[:8], resp.StatusCode)
				s.store.MarkPropertyInactive(prop.ID)
			} else {
				log.Printf("Healthcheck %s: unexpected status %d", prop.ID[:8], resp.StatusCode)
				s.store.TouchProperty(prop.ID, time.Now())
			}
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

