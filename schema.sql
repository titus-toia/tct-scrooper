-- SQLite Schema for TUI Integration
-- Database file: scraper.db

-- Unique properties (deduplicated by fingerprint)
CREATE TABLE IF NOT EXISTS properties (
	id TEXT PRIMARY KEY,
	normalized_address TEXT,
	city TEXT,
	postal_code TEXT,
	beds INTEGER,
	beds_plus INTEGER DEFAULT 0,
	baths INTEGER,
	sqft INTEGER,
	property_type TEXT,
	first_seen_at DATETIME,
	last_seen_at DATETIME,
	times_listed INTEGER DEFAULT 1,
	synced BOOLEAN DEFAULT FALSE
);

-- Listing snapshots (full history, linked to property)
CREATE TABLE IF NOT EXISTS listing_snapshots (
	id INTEGER PRIMARY KEY,
	property_id TEXT NOT NULL,
	listing_id TEXT NOT NULL,
	site_id TEXT NOT NULL,
	url TEXT,
	price INTEGER,
	description TEXT,
	realtor JSON,
	data JSON,
	scraped_at DATETIME,
	run_id INTEGER,
	FOREIGN KEY (property_id) REFERENCES properties(id)
);

-- Scrape run history
CREATE TABLE IF NOT EXISTS scrape_runs (
	id INTEGER PRIMARY KEY,
	site_id TEXT,
	started_at DATETIME,
	finished_at DATETIME,
	status TEXT,
	listings_found INTEGER,
	listings_new INTEGER,
	properties_new INTEGER,
	properties_relisted INTEGER,
	errors_count INTEGER
);

-- Log entries (for TUI to display)
CREATE TABLE IF NOT EXISTS scrape_logs (
	id INTEGER PRIMARY KEY,
	run_id INTEGER,
	timestamp DATETIME,
	level TEXT,
	message TEXT,
	site_id TEXT
);

-- Per-site stats (aggregated for quick TUI reads)
CREATE TABLE IF NOT EXISTS site_stats (
	site_id TEXT PRIMARY KEY,
	last_run_at DATETIME,
	last_run_status TEXT,
	total_properties INTEGER,
	total_snapshots INTEGER,
	properties_synced INTEGER,
	success_rate REAL,
	avg_run_duration_sec INTEGER,
	scrape_resume_page INTEGER DEFAULT 0
);

-- TUI → Scraper commands
CREATE TABLE IF NOT EXISTS commands (
	id INTEGER PRIMARY KEY,
	command TEXT,
	params JSON,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	processed_at DATETIME
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_properties_address ON properties(normalized_address);
CREATE INDEX IF NOT EXISTS idx_snapshots_property ON listing_snapshots(property_id, scraped_at);
CREATE INDEX IF NOT EXISTS idx_snapshots_listing_id ON listing_snapshots(listing_id);
CREATE INDEX IF NOT EXISTS idx_commands_pending ON commands(processed_at) WHERE processed_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_logs_run ON scrape_logs(run_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_runs_status ON scrape_runs(status, started_at);
