package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"tct_scrooper/config"
	"tct_scrooper/httputil"
	"tct_scrooper/logging"
	"tct_scrooper/scheduler"
	"tct_scrooper/scraper"
	"tct_scrooper/storage"
)

var (
	resync    = flag.Bool("resync", false, "Mark all properties unsynced and push to Supabase")
	syncOnly  = flag.Bool("sync", false, "Run sync to Supabase and exit")
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

	store, err := storage.NewSQLiteStore(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to open SQLite: %v", err)
	}
	defer store.Close()
	log.Printf("SQLite database: %s", cfg.DBPath)

	var supabase *storage.SupabaseStore
	if cfg.Supabase.URL != "" && cfg.Supabase.ServiceKey != "" {
		supabase = storage.NewSupabaseStore(&cfg.Supabase)
		log.Println("Supabase sync enabled")
	} else {
		log.Println("Supabase sync disabled (no credentials)")
	}

	orchestrator := scraper.NewOrchestrator(cfg, store, supabase)
	ctx := context.Background()

	// Handle one-shot commands
	if *resync {
		log.Println("Marking all properties as unsynced...")
		count, err := store.MarkAllUnsynced()
		if err != nil {
			log.Fatalf("Failed to mark unsynced: %v", err)
		}
		log.Printf("Marked %d properties as unsynced", count)

		log.Println("Syncing to Supabase...")
		if err := orchestrator.SyncToSupabase(ctx); err != nil {
			log.Fatalf("Sync failed: %v", err)
		}
		log.Println("Resync complete!")
		return
	}

	if *syncOnly {
		log.Println("Running sync to Supabase...")
		if err := orchestrator.SyncToSupabase(ctx); err != nil {
			log.Fatalf("Sync failed: %v", err)
		}
		log.Println("Sync complete!")
		return
	}

	if *scrapeNow {
		log.Println("Running scrape...")
		if err := orchestrator.RunAll(ctx); err != nil {
			log.Fatalf("Scrape failed: %v", err)
		}
		log.Println("Scrape complete!")
		return
	}

	// Daemon mode
	sched := scheduler.New(cfg, orchestrator, store, clients)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := sched.Start(ctx); err != nil {
		log.Fatalf("Failed to start scheduler: %v", err)
	}

	log.Println("Daemon running. Press Ctrl+C to stop.")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down...")
	sched.Stop()
	log.Println("Goodbye!")
}
