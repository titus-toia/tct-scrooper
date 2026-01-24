package storage

import (
	"database/sql"
	"encoding/json"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"tct_scrooper/models"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}

	store := &SQLiteStore{db: db}
	if err := store.migrate(); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS properties (
		id TEXT PRIMARY KEY,
		normalized_address TEXT,
		city TEXT,
		postal_code TEXT,
		beds INTEGER,
		baths INTEGER,
		sqft INTEGER,
		property_type TEXT,
		first_seen_at DATETIME,
		last_seen_at DATETIME,
		times_listed INTEGER DEFAULT 1,
		synced BOOLEAN DEFAULT FALSE
	);

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

	CREATE TABLE IF NOT EXISTS scrape_logs (
		id INTEGER PRIMARY KEY,
		run_id INTEGER,
		timestamp DATETIME,
		level TEXT,
		message TEXT,
		site_id TEXT
	);

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

	CREATE TABLE IF NOT EXISTS commands (
		id INTEGER PRIMARY KEY,
		command TEXT,
		params JSON,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		processed_at DATETIME
	);

	CREATE INDEX IF NOT EXISTS idx_properties_address ON properties(normalized_address);
	CREATE INDEX IF NOT EXISTS idx_snapshots_property ON listing_snapshots(property_id, scraped_at);
	CREATE INDEX IF NOT EXISTS idx_snapshots_listing_id ON listing_snapshots(listing_id);
	CREATE INDEX IF NOT EXISTS idx_commands_pending ON commands(processed_at) WHERE processed_at IS NULL;
	CREATE INDEX IF NOT EXISTS idx_logs_run ON scrape_logs(run_id, timestamp);
	CREATE INDEX IF NOT EXISTS idx_runs_status ON scrape_runs(status, started_at);
	`
	_, err := s.db.Exec(schema)
	return err
}

func (s *SQLiteStore) GetProperty(id string) (*models.Property, error) {
	row := s.db.QueryRow(`
		SELECT id, normalized_address, city, postal_code, beds, beds_plus, baths, sqft, property_type,
			first_seen_at, last_seen_at, times_listed, synced
		FROM properties WHERE id = ?`, id)

	var p models.Property
	var postalCode sql.NullString
	err := row.Scan(&p.ID, &p.NormalizedAddress, &p.City, &postalCode, &p.Beds, &p.BedsPlus, &p.Baths,
		&p.SqFt, &p.PropertyType, &p.FirstSeenAt, &p.LastSeenAt, &p.TimesListed, &p.Synced)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.PostalCode = postalCode.String
	return &p, nil
}

func (s *SQLiteStore) UpsertProperty(p *models.Property) error {
	_, err := s.db.Exec(`
		INSERT INTO properties (id, normalized_address, city, postal_code, beds, beds_plus, baths, sqft, property_type,
			first_seen_at, last_seen_at, times_listed, synced)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			last_seen_at = excluded.last_seen_at,
			times_listed = excluded.times_listed,
			postal_code = COALESCE(excluded.postal_code, postal_code),
			synced = FALSE`,
		p.ID, p.NormalizedAddress, p.City, p.PostalCode, p.Beds, p.BedsPlus, p.Baths, p.SqFt, p.PropertyType,
		p.FirstSeenAt, p.LastSeenAt, p.TimesListed, p.Synced)
	return err
}

func (s *SQLiteStore) CreateSnapshot(snap *models.Snapshot) error {
	_, err := s.db.Exec(`
		INSERT INTO listing_snapshots (property_id, listing_id, site_id, url, price, description, realtor, data, scraped_at, run_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snap.PropertyID, snap.ListingID, snap.SiteID, snap.URL, snap.Price, snap.Description, snap.Realtor, snap.Data, snap.ScrapedAt, snap.RunID)
	return err
}

func (s *SQLiteStore) GetSnapshotsForProperty(propertyID string) ([]models.Snapshot, error) {
	rows, err := s.db.Query(`
		SELECT id, property_id, listing_id, site_id, url, price, description, realtor, data, scraped_at, run_id
		FROM listing_snapshots WHERE property_id = ? ORDER BY scraped_at`, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snapshots []models.Snapshot
	for rows.Next() {
		var snap models.Snapshot
		var desc, realtor sql.NullString
		if err := rows.Scan(&snap.ID, &snap.PropertyID, &snap.ListingID, &snap.SiteID,
			&snap.URL, &snap.Price, &desc, &realtor, &snap.Data, &snap.ScrapedAt, &snap.RunID); err != nil {
			return nil, err
		}
		snap.Description = desc.String
		if realtor.Valid {
			snap.Realtor = []byte(realtor.String)
		}
		snapshots = append(snapshots, snap)
	}
	return snapshots, rows.Err()
}

func (s *SQLiteStore) GetLastSnapshotForProperty(propertyID string) (*models.Snapshot, error) {
	row := s.db.QueryRow(`
		SELECT id, property_id, listing_id, site_id, url, price, description, realtor, data, scraped_at, run_id
		FROM listing_snapshots WHERE property_id = ? ORDER BY scraped_at DESC LIMIT 1`, propertyID)

	var snap models.Snapshot
	var desc, realtor sql.NullString
	err := row.Scan(&snap.ID, &snap.PropertyID, &snap.ListingID, &snap.SiteID,
		&snap.URL, &snap.Price, &desc, &realtor, &snap.Data, &snap.ScrapedAt, &snap.RunID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	snap.Description = desc.String
	if realtor.Valid {
		snap.Realtor = []byte(realtor.String)
	}
	return &snap, nil
}

func (s *SQLiteStore) GetLastSnapshotByMLS(listingID string) (*models.Snapshot, error) {
	row := s.db.QueryRow(`
		SELECT id, property_id, listing_id, site_id, url, price, description, realtor, data, scraped_at, run_id
		FROM listing_snapshots WHERE listing_id = ? ORDER BY scraped_at DESC LIMIT 1`, listingID)

	var snap models.Snapshot
	var desc, realtor sql.NullString
	err := row.Scan(&snap.ID, &snap.PropertyID, &snap.ListingID, &snap.SiteID,
		&snap.URL, &snap.Price, &desc, &realtor, &snap.Data, &snap.ScrapedAt, &snap.RunID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	snap.Description = desc.String
	if realtor.Valid {
		snap.Realtor = []byte(realtor.String)
	}
	return &snap, nil
}

func (s *SQLiteStore) CreateRun(run *models.ScrapeRun) (int64, error) {
	result, err := s.db.Exec(`
		INSERT INTO scrape_runs (site_id, started_at, status, listings_found, listings_new,
			properties_new, properties_relisted, errors_count)
		VALUES (?, ?, ?, 0, 0, 0, 0, 0)`,
		run.SiteID, run.StartedAt, run.Status)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *SQLiteStore) UpdateRun(run *models.ScrapeRun) error {
	_, err := s.db.Exec(`
		UPDATE scrape_runs SET finished_at = ?, status = ?, listings_found = ?,
			listings_new = ?, properties_new = ?, properties_relisted = ?, errors_count = ?
		WHERE id = ?`,
		run.FinishedAt, run.Status, run.ListingsFound, run.ListingsNew,
		run.PropertiesNew, run.PropertiesRelisted, run.ErrorsCount, run.ID)
	return err
}

func (s *SQLiteStore) Log(runID *int64, level models.LogLevel, message, siteID string) error {
	_, err := s.db.Exec(`
		INSERT INTO scrape_logs (run_id, timestamp, level, message, site_id)
		VALUES (?, ?, ?, ?, ?)`,
		runID, time.Now(), level, message, siteID)
	return err
}

func (s *SQLiteStore) UpdateSiteStats(siteID string) error {
	_, err := s.db.Exec(`
		INSERT INTO site_stats (site_id, last_run_at, last_run_status, total_properties,
			total_snapshots, properties_synced, success_rate, avg_run_duration_sec)
		SELECT
			?,
			(SELECT started_at FROM scrape_runs WHERE site_id = ? ORDER BY started_at DESC LIMIT 1),
			(SELECT status FROM scrape_runs WHERE site_id = ? ORDER BY started_at DESC LIMIT 1),
			(SELECT COUNT(DISTINCT property_id) FROM listing_snapshots WHERE site_id = ?),
			(SELECT COUNT(*) FROM listing_snapshots WHERE site_id = ?),
			(SELECT COUNT(*) FROM properties WHERE synced = TRUE AND id IN
				(SELECT DISTINCT property_id FROM listing_snapshots WHERE site_id = ?)),
			(SELECT CAST(SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) AS REAL) /
				NULLIF(COUNT(*), 0) FROM scrape_runs WHERE site_id = ?),
			(SELECT AVG(CAST((julianday(finished_at) - julianday(started_at)) * 86400 AS INTEGER))
				FROM scrape_runs WHERE site_id = ? AND finished_at IS NOT NULL)
		ON CONFLICT(site_id) DO UPDATE SET
			last_run_at = excluded.last_run_at,
			last_run_status = excluded.last_run_status,
			total_properties = excluded.total_properties,
			total_snapshots = excluded.total_snapshots,
			properties_synced = excluded.properties_synced,
			success_rate = excluded.success_rate,
			avg_run_duration_sec = excluded.avg_run_duration_sec`,
		siteID, siteID, siteID, siteID, siteID, siteID, siteID, siteID)
	return err
}

func (s *SQLiteStore) GetPropertyCount(siteID string) (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(DISTINCT property_id) FROM listing_snapshots WHERE site_id = ?`, siteID).Scan(&count)
	return count, err
}

func (s *SQLiteStore) GetUnsyncedProperties() ([]models.Property, error) {
	rows, err := s.db.Query(`
		SELECT id, normalized_address, city, postal_code, beds, beds_plus, baths, sqft, property_type,
			first_seen_at, last_seen_at, times_listed, synced
		FROM properties WHERE synced = FALSE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var props []models.Property
	for rows.Next() {
		var p models.Property
		var postalCode sql.NullString
		if err := rows.Scan(&p.ID, &p.NormalizedAddress, &p.City, &postalCode, &p.Beds, &p.BedsPlus, &p.Baths,
			&p.SqFt, &p.PropertyType, &p.FirstSeenAt, &p.LastSeenAt, &p.TimesListed, &p.Synced); err != nil {
			return nil, err
		}
		p.PostalCode = postalCode.String
		props = append(props, p)
	}
	return props, rows.Err()
}

func (s *SQLiteStore) MarkPropertySynced(id string) error {
	_, err := s.db.Exec(`UPDATE properties SET synced = TRUE WHERE id = ?`, id)
	return err
}

func (s *SQLiteStore) GetPendingCommands() ([]models.Command, error) {
	rows, err := s.db.Query(`
		SELECT id, command, params, created_at, processed_at
		FROM commands WHERE processed_at IS NULL ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cmds []models.Command
	for rows.Next() {
		var cmd models.Command
		if err := rows.Scan(&cmd.ID, &cmd.Command, &cmd.Params, &cmd.CreatedAt, &cmd.ProcessedAt); err != nil {
			return nil, err
		}
		cmds = append(cmds, cmd)
	}
	return cmds, rows.Err()
}

func (s *SQLiteStore) MarkCommandProcessed(id int64) error {
	_, err := s.db.Exec(`UPDATE commands SET processed_at = ? WHERE id = ?`, time.Now(), id)
	return err
}

func (s *SQLiteStore) ParseCommandParams(cmd *models.Command) (*models.CommandParams, error) {
	if cmd.Params == nil || string(cmd.Params) == "null" {
		return &models.CommandParams{}, nil
	}
	var params models.CommandParams
	if err := json.Unmarshal(cmd.Params, &params); err != nil {
		return nil, err
	}
	return &params, nil
}

func (s *SQLiteStore) HasRecentSnapshot(listingID string) (bool, error) {
	var exists int
	err := s.db.QueryRow(`
		SELECT 1 FROM listing_snapshots
		WHERE listing_id = ?
		LIMIT 1`, listingID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (s *SQLiteStore) GetResumePage(siteID string) (int, error) {
	var page int
	err := s.db.QueryRow(`
		SELECT COALESCE(scrape_resume_page, 0) FROM site_stats WHERE site_id = ?`, siteID).Scan(&page)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return page, err
}

func (s *SQLiteStore) SetResumePage(siteID string, page int) error {
	_, err := s.db.Exec(`
		INSERT INTO site_stats (site_id, scrape_resume_page)
		VALUES (?, ?)
		ON CONFLICT(site_id) DO UPDATE SET scrape_resume_page = ?`, siteID, page, page)
	return err
}

func (s *SQLiteStore) ClearResumePage(siteID string) error {
	_, err := s.db.Exec(`
		UPDATE site_stats SET scrape_resume_page = 0 WHERE site_id = ?`, siteID)
	return err
}

func (s *SQLiteStore) GetSitesWithResumePage() ([]string, error) {
	rows, err := s.db.Query(`
		SELECT site_id FROM site_stats WHERE scrape_resume_page > 0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sites []string
	for rows.Next() {
		var siteID string
		if err := rows.Scan(&siteID); err != nil {
			return nil, err
		}
		sites = append(sites, siteID)
	}
	return sites, rows.Err()
}

func (s *SQLiteStore) GetLastRunTime(siteID string) (time.Time, error) {
	var lastRun time.Time
	err := s.db.QueryRow(`
		SELECT last_run_at FROM site_stats WHERE site_id = ?`, siteID).Scan(&lastRun)
	if err == sql.ErrNoRows {
		return time.Time{}, nil
	}
	return lastRun, err
}
