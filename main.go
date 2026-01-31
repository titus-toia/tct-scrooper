package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tct_scrooper/config"
	"tct_scrooper/httputil"
	"tct_scrooper/logging"
	"tct_scrooper/scheduler"
	"tct_scrooper/scraper"
	"tct_scrooper/services"
	"tct_scrooper/storage"
	"tct_scrooper/workers"
)

var (
	scrapeNow = flag.Bool("scrape", false, "Run scrape once and exit")
)

func main() {
	flag.Parse()
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	logFile, err := logging.Setup("daemon.log")
	if err != nil {
		log.Printf("Warning: could not set up file logging: %v", err)
	} else {
		defer logFile.Close()
	}

	log.Println("Starting tct_scrooper...")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Loaded %d site configs", len(cfg.Sites))
	for id, site := range cfg.Sites {
		log.Printf("  - %s (%s)", site.Name, id)
	}

	clients := httputil.NewClients(&cfg.Proxy)
	log.Printf("Proxy: %s", cfg.Proxy.URL)

	ctx := context.Background()

	// Initialize Postgres store (required for domain data)
	pgStore, err := storage.NewPostgresStore(ctx, cfg.Supabase.DBURL)
	if err != nil {
		log.Fatalf("Failed to connect to Postgres: %v", err)
	}
	defer pgStore.Close()
	log.Printf("Connected to Postgres: %s", maskConnectionString(cfg.Supabase.DBURL))

	// Initialize services
	matchService := services.NewMatchService(pgStore)
	mediaService := services.NewMediaService(pgStore)
	listingService := services.NewListingService(pgStore, matchService, mediaService)
	healthcheckService := services.NewHealthcheckService(pgStore, listingService)

	log.Println("Services initialized")

	// Initialize SQLite for operational data (TUI commands, legacy support)
	sqliteStore, err := storage.NewSQLiteStore(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to open SQLite: %v", err)
	}
	defer sqliteStore.Close()
	log.Printf("SQLite database: %s", cfg.DBPath)

	// Create orchestrator
	orchestrator := scraper.NewOrchestrator(cfg, sqliteStore)
	orchestrator.SetServices(pgStore, listingService, matchService, mediaService, healthcheckService)

	// Handle one-shot commands
	if *scrapeNow {
		log.Println("Running scrape...")
		if err := orchestrator.RunAll(ctx); err != nil {
			log.Fatalf("Scrape failed: %v", err)
		}
		log.Println("Scrape complete!")
		return
	}

	// Daemon mode
	sched := scheduler.New(cfg, orchestrator, sqliteStore, clients)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := sched.Start(ctx); err != nil {
		log.Fatalf("Failed to start scheduler: %v", err)
	}

	// Start background workers
	enrichmentWorker := workers.NewEnrichmentWorker(pgStore, mediaService, cfg.Proxy.URL)
	go enrichmentWorker.Run(ctx, 10, 5*time.Minute) // batch of 10 every 5 min
	log.Println("Enrichment worker started")

	healthcheckWorker := workers.NewHealthcheckWorker(pgStore, cfg.Proxy.URL)
	go healthcheckWorker.Run(ctx, 24*time.Hour, 20, 30*time.Minute) // check listings older than 24h, batch 20, every 30 min
	log.Println("Healthcheck worker started")

	// Media worker (with NoOp uploader for now - replace with real S3 uploader when configured)
	mediaUploader := workers.NewNoOpUploader() // TODO: replace with real S3Uploader
	mediaWorker := workers.NewMediaWorker(pgStore, mediaUploader, cfg.Proxy.URL)
	go mediaWorker.Run(ctx, 20, 2*time.Minute) // batch of 20 every 2 min
	log.Println("Media worker started")

	log.Println("Daemon running. Press Ctrl+C to stop.")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down...")
	sched.Stop()
	log.Println("Goodbye!")
}

// maskConnectionString masks password in connection string for logging
func maskConnectionString(connStr string) string {
	// Simple mask - find :// and mask until @
	start := 0
	for i := 0; i < len(connStr)-3; i++ {
		if connStr[i:i+3] == "://" {
			start = i + 3
			break
		}
	}
	if start == 0 {
		return connStr
	}

	// Find : after user
	colonIdx := -1
	atIdx := -1
	for i := start; i < len(connStr); i++ {
		if connStr[i] == ':' && colonIdx == -1 {
			colonIdx = i
		}
		if connStr[i] == '@' {
			atIdx = i
			break
		}
	}

	if colonIdx > 0 && atIdx > colonIdx {
		return connStr[:colonIdx+1] + "****" + connStr[atIdx:]
	}
	return connStr
}
