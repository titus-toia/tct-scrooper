package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"tct_scrooper/config"
	"tct_scrooper/scheduler"
	"tct_scrooper/scraper"
	"tct_scrooper/storage"
	"tct_scrooper/vpn"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting tct_scrooper daemon...")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Loaded %d site configs", len(cfg.Sites))
	for id, site := range cfg.Sites {
		log.Printf("  - %s (%s)", site.Name, id)
	}

	vpnClient := vpn.NewExpressVPN(&cfg.ExpressVPN)
	if err := vpnClient.EnsureConnected(); err != nil {
		log.Fatalf("VPN required but not connected: %v", err)
	}
	status, _ := vpnClient.GetStatus()
	log.Printf("VPN status: %s", status)

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

	orchestrator := scraper.NewOrchestrator(cfg, store, supabase, vpnClient)
	sched := scheduler.New(cfg, orchestrator, store)

	ctx, cancel := context.WithCancel(context.Background())
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
