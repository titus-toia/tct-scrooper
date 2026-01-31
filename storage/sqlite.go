package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
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
		beds_plus INTEGER DEFAULT 0,
		baths INTEGER,
		sqft INTEGER,
		property_type TEXT,
		first_seen_at DATETIME,
		last_seen_at DATETIME,
		times_listed INTEGER DEFAULT 1,
		synced BOOLEAN DEFAULT FALSE,
		is_active BOOLEAN DEFAULT TRUE
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

	CREATE TABLE IF NOT EXISTS property_matches (
		id INTEGER PRIMARY KEY,
		matched_id TEXT NOT NULL,
		incoming_id TEXT NOT NULL,
		confidence REAL,
		match_reasons JSON,
		status TEXT DEFAULT 'pending',
		reviewed_at DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(matched_id, incoming_id)
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
	CREATE INDEX IF NOT EXISTS idx_matches_status ON property_matches(status);
	CREATE INDEX IF NOT EXISTS idx_matches_matched ON property_matches(matched_id);
	CREATE INDEX IF NOT EXISTS idx_matches_incoming ON property_matches(incoming_id);
	`
	_, err := s.db.Exec(schema)
	return err
}

func (s *SQLiteStore) GetProperty(id string) (*models.Property, error) {
	row := s.db.QueryRow(`
		SELECT id, normalized_address, city, postal_code, beds, beds_plus, baths, sqft, property_type,
			first_seen_at, last_seen_at, times_listed, synced, COALESCE(is_active, TRUE)
		FROM properties WHERE id = ?`, id)

	var p models.Property
	var postalCode sql.NullString
	err := row.Scan(&p.ID, &p.NormalizedAddress, &p.City, &postalCode, &p.Beds, &p.BedsPlus, &p.Baths,
		&p.SqFt, &p.PropertyType, &p.FirstSeenAt, &p.LastSeenAt, &p.TimesListed, &p.Synced, &p.IsActive)
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
			first_seen_at, last_seen_at, times_listed, synced, is_active)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, TRUE)
		ON CONFLICT(id) DO UPDATE SET
			last_seen_at = excluded.last_seen_at,
			times_listed = excluded.times_listed,
			postal_code = COALESCE(excluded.postal_code, postal_code),
			synced = FALSE,
			is_active = TRUE`,
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
			COALESCE(
				(SELECT started_at FROM scrape_runs WHERE site_id = ? AND status = 'completed' ORDER BY started_at DESC LIMIT 1),
				(SELECT started_at FROM scrape_runs WHERE site_id = ? ORDER BY started_at DESC LIMIT 1)
			),
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
		siteID, siteID, siteID, siteID, siteID, siteID, siteID, siteID, siteID)
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
			first_seen_at, last_seen_at, times_listed, synced, COALESCE(is_active, TRUE)
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
			&p.SqFt, &p.PropertyType, &p.FirstSeenAt, &p.LastSeenAt, &p.TimesListed, &p.Synced, &p.IsActive); err != nil {
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

func (s *SQLiteStore) TouchProperty(id string, t time.Time) error {
	_, err := s.db.Exec(`UPDATE properties SET last_seen_at = ?, synced = FALSE WHERE id = ?`, t, id)
	return err
}

func (s *SQLiteStore) GetOldestActiveProperty() (*models.Property, string, error) {
	row := s.db.QueryRow(`
		SELECT p.id, p.normalized_address, p.city, p.postal_code, p.beds, p.beds_plus, p.baths, p.sqft, p.property_type,
			p.first_seen_at, p.last_seen_at, p.times_listed, p.synced, p.is_active,
			(SELECT ls.url FROM listing_snapshots ls WHERE ls.property_id = p.id ORDER BY ls.scraped_at DESC LIMIT 1) as url
		FROM properties p
		WHERE p.is_active = TRUE AND p.last_seen_at < datetime('now', '-1 day')
		ORDER BY p.last_seen_at ASC
		LIMIT 1`)

	var p models.Property
	var postalCode, url sql.NullString
	err := row.Scan(&p.ID, &p.NormalizedAddress, &p.City, &postalCode, &p.Beds, &p.BedsPlus, &p.Baths,
		&p.SqFt, &p.PropertyType, &p.FirstSeenAt, &p.LastSeenAt, &p.TimesListed, &p.Synced, &p.IsActive, &url)
	if err == sql.ErrNoRows {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	p.PostalCode = postalCode.String
	return &p, url.String, nil
}

func (s *SQLiteStore) MarkPropertyInactive(id string) error {
	_, err := s.db.Exec(`UPDATE properties SET is_active = FALSE, synced = FALSE WHERE id = ?`, id)
	return err
}

func (s *SQLiteStore) MarkAllUnsynced() (int64, error) {
	result, err := s.db.Exec(`UPDATE properties SET synced = FALSE`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
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
		var params sql.NullString
		if err := rows.Scan(&cmd.ID, &cmd.Command, &params, &cmd.CreatedAt, &cmd.ProcessedAt); err != nil {
			return nil, err
		}
		if params.Valid {
			cmd.Params = json.RawMessage(params.String)
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

type propertyMatchCandidate struct {
	ID                string
	NormalizedAddress string
	City              string
	PostalCode        string
	Beds              int
	Baths             int
	SqFt              int
	PropertyType      string
}

func (s *SQLiteStore) InsertPotentialMatches(incoming *models.Property) (int, error) {
	if incoming == nil {
		return 0, nil
	}

	normalized := strings.TrimSpace(incoming.NormalizedAddress)
	if normalized == "" {
		return 0, nil
	}

	prefix := addressPrefix(normalized, 2)
	if incoming.PostalCode == "" && prefix == "" {
		return 0, nil
	}

	query := `
		SELECT id, normalized_address, city, postal_code, beds, baths, sqft, property_type
		FROM properties
		WHERE id != ?`
	args := []interface{}{incoming.ID}

	if incoming.City != "" {
		query += " AND city = ?"
		args = append(args, incoming.City)
	}
	if incoming.PostalCode != "" {
		query += " AND postal_code = ?"
		args = append(args, incoming.PostalCode)
	}
	if prefix != "" {
		query += " AND normalized_address LIKE ?"
		args = append(args, prefix+"%")
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	baseIncoming := baseAddress(normalized)
	inserted := 0

	for rows.Next() {
		var candidate propertyMatchCandidate
		var postalCode sql.NullString
		if err := rows.Scan(&candidate.ID, &candidate.NormalizedAddress, &candidate.City, &postalCode,
			&candidate.Beds, &candidate.Baths, &candidate.SqFt, &candidate.PropertyType); err != nil {
			return inserted, err
		}
		candidate.PostalCode = postalCode.String

		confidence, reasons, ok := scorePotentialMatch(incoming, &candidate, baseIncoming)
		if !ok {
			continue
		}

		reasonsJSON, _ := json.Marshal(reasons)
		result, err := s.db.Exec(`
			INSERT OR IGNORE INTO property_matches (matched_id, incoming_id, confidence, match_reasons, status)
			VALUES (?, ?, ?, ?, 'pending')`,
			candidate.ID, incoming.ID, confidence, reasonsJSON)
		if err != nil {
			return inserted, err
		}

		if rowsAffected, _ := result.RowsAffected(); rowsAffected > 0 {
			inserted++
		}
	}

	return inserted, rows.Err()
}

func scorePotentialMatch(incoming *models.Property, candidate *propertyMatchCandidate, baseIncoming string) (float64, []string, bool) {
	reasons := []string{}
	strongAddress := false
	sameAddress := false

	if incoming.NormalizedAddress != "" && candidate.NormalizedAddress != "" &&
		incoming.NormalizedAddress == candidate.NormalizedAddress {
		reasons = append(reasons, "same_address")
		strongAddress = true
		sameAddress = true
	} else if baseIncoming != "" {
		baseCandidate := baseAddress(candidate.NormalizedAddress)
		if baseCandidate != "" && baseCandidate == baseIncoming {
			reasons = append(reasons, "same_base_address")
			strongAddress = true
		}
	}

	samePostal := incoming.PostalCode != "" && candidate.PostalCode != "" &&
		incoming.PostalCode == candidate.PostalCode
	if samePostal {
		reasons = append(reasons, "same_postal")
	}

	sameType := incoming.PropertyType != "" && candidate.PropertyType != "" &&
		strings.EqualFold(incoming.PropertyType, candidate.PropertyType)
	if sameType {
		reasons = append(reasons, "same_property_type")
	}

	closeAttrCount := 0
	if incoming.Beds > 0 && candidate.Beds > 0 {
		diff := absInt(incoming.Beds - candidate.Beds)
		if diff == 0 {
			reasons = append(reasons, "same_beds")
			closeAttrCount++
		} else if diff == 1 {
			reasons = append(reasons, "close_beds")
			closeAttrCount++
		}
	}
	if incoming.Baths > 0 && candidate.Baths > 0 {
		diff := absInt(incoming.Baths - candidate.Baths)
		if diff == 0 {
			reasons = append(reasons, "same_baths")
			closeAttrCount++
		} else if diff == 1 {
			reasons = append(reasons, "close_baths")
			closeAttrCount++
		}
	}
	if closeSqFt(incoming.SqFt, candidate.SqFt) {
		reasons = append(reasons, "close_sqft")
		closeAttrCount++
	}

	if !strongAddress {
		if !(samePostal && sameType && closeAttrCount >= 2) {
			return 0, nil, false
		}
		confidence := 0.55 + 0.05*float64(closeAttrCount)
		if confidence > 0.85 {
			confidence = 0.85
		}
		return confidence, reasons, true
	}

	confidence := 0.75
	if sameAddress {
		confidence = 0.9
	}
	confidence += 0.03 * float64(closeAttrCount)
	if samePostal {
		confidence += 0.03
	}
	if sameType {
		confidence += 0.03
	}
	if confidence > 0.95 {
		confidence = 0.95
	}

	return confidence, reasons, true
}

func addressPrefix(normalized string, minTokens int) string {
	parts := strings.Fields(normalized)
	if len(parts) < minTokens {
		return ""
	}
	return strings.Join(parts[:minTokens], " ")
}

func baseAddress(normalized string) string {
	parts := strings.Fields(normalized)
	if len(parts) == 0 {
		return ""
	}

	unitTokens := map[string]bool{
		"apt":  true,
		"unit": true,
		"ste":  true,
		"fl":   true,
		"bldg": true,
	}

	for i, part := range parts {
		if unitTokens[part] {
			parts = parts[:i]
			break
		}
	}

	if len(parts) >= 4 && isNumericToken(parts[len(parts)-1]) {
		parts = parts[:len(parts)-1]
	}

	return strings.Join(parts, " ")
}

func isNumericToken(token string) bool {
	if token == "" {
		return false
	}
	for _, r := range token {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func closeSqFt(a, b int) bool {
	if a <= 0 || b <= 0 {
		return false
	}
	diff := absInt(a - b)
	if diff <= 200 {
		return true
	}
	maxVal := a
	if b > maxVal {
		maxVal = b
	}
	return float64(diff) <= 0.1*float64(maxVal)
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// ResetAllData clears all SQLite operational tables
func (s *SQLiteStore) ResetAllData() error {
	tables := []string{
		"scrape_logs",
		"scrape_runs",
		"property_matches",
		"listing_snapshots",
		"properties",
		"site_stats",
		"commands",
	}

	for _, table := range tables {
		_, err := s.db.Exec(fmt.Sprintf("DELETE FROM %s", table))
		if err != nil {
			return fmt.Errorf("clear %s: %w", table, err)
		}
	}

	return nil
}
