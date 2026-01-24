package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/robfig/cron/v3"
	"tct_scrooper/config"
	"tct_scrooper/scraper"
	"tct_scrooper/storage"
)

type Scheduler struct {
	cfg          *config.Config
	orchestrator *scraper.Orchestrator
	store        *storage.SQLiteStore
	cron         *cron.Cron
	ticker       *time.Ticker
	stopCh       chan struct{}
}

func New(cfg *config.Config, orchestrator *scraper.Orchestrator, store *storage.SQLiteStore) *Scheduler {
	return &Scheduler{
		cfg:          cfg,
		orchestrator: orchestrator,
		store:        store,
		cron:         cron.New(),
		stopCh:       make(chan struct{}),
	}
}

func (s *Scheduler) Start(ctx context.Context) error {
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
		log.Println("No schedule configured, running once")
		return s.orchestrator.RunAll(ctx)
	}

	go s.pollCommands(ctx)
	go s.pollResumes(ctx)

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
				if err := s.orchestrator.HandleCommand(&cmd); err != nil {
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
