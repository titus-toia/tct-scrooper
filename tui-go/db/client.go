package db

import (
	"database/sql"
	"encoding/json"
	"time"

	_ "modernc.org/sqlite"
)

type Client struct {
	db *sql.DB
}

type SiteStats struct {
	SiteID           string
	LastRunAt        *time.Time
	LastRunStatus    *string
	TotalProperties  int
	TotalSnapshots   int
	PropertiesSynced int
	SuccessRate      float64
	AvgRunDuration   int
	ResumeFromPage   int
}

type ScrapeRun struct {
	ID                 int
	SiteID             string
	StartedAt          time.Time
	FinishedAt         *time.Time
	Status             string
	ListingsFound      int
	ListingsNew        int
	PropertiesNew      int
	PropertiesRelisted int
	ErrorsCount        int
}

type Property struct {
	ID                string
	NormalizedAddress string
	City              string
	Beds              int
	Baths             int
	Sqft              int
	PropertyType      string
	FirstSeenAt       time.Time
	LastSeenAt        time.Time
	TimesListed       int
	Synced            bool
	LatestPrice       int
}

type Snapshot struct {
	ID          int
	PropertyID  string
	ListingID   string
	SiteID      string
	URL         string
	Price       int
	Description string
	Realtor     RealtorInfo
	ScrapedAt   time.Time
}

type RealtorInfo struct {
	AgentName    string
	AgentPhone   string
	CompanyName  string
	CompanyPhone string
}

type ScrapeLog struct {
	ID        int
	RunID     *int
	Timestamp time.Time
	Level     string
	Message   string
	SiteID    *string
}

type ExtractionStrategy struct {
	SiteID       string
	Strategy     string
	Priority     int
	SuccessCount int
	FailCount    int
}

func New(dbPath string) (*Client, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	return &Client{db: db}, nil
}

func (c *Client) Close() error {
	return c.db.Close()
}

func (c *Client) GetSiteStats() ([]SiteStats, error) {
	rows, err := c.db.Query(`
		SELECT site_id, last_run_at, last_run_status, total_properties,
			total_snapshots, properties_synced, success_rate, avg_run_duration_sec,
			COALESCE(scrape_resume_page, 0)
		FROM site_stats ORDER BY site_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []SiteStats
	for rows.Next() {
		var s SiteStats
		var lastRunAt, lastRunStatus sql.NullString
		err := rows.Scan(&s.SiteID, &lastRunAt, &lastRunStatus, &s.TotalProperties,
			&s.TotalSnapshots, &s.PropertiesSynced, &s.SuccessRate, &s.AvgRunDuration,
			&s.ResumeFromPage)
		if err != nil {
			return nil, err
		}
		if lastRunAt.Valid {
			t, _ := time.Parse(time.RFC3339, lastRunAt.String)
			s.LastRunAt = &t
		}
		if lastRunStatus.Valid {
			s.LastRunStatus = &lastRunStatus.String
		}
		stats = append(stats, s)
	}
	return stats, nil
}

func (c *Client) GetRecentRuns(limit int) ([]ScrapeRun, error) {
	rows, err := c.db.Query(`
		SELECT id, site_id, started_at, finished_at, status, listings_found,
			listings_new, properties_new, properties_relisted, errors_count
		FROM scrape_runs ORDER BY started_at DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []ScrapeRun
	for rows.Next() {
		var r ScrapeRun
		var startedAt, finishedAt sql.NullString
		err := rows.Scan(&r.ID, &r.SiteID, &startedAt, &finishedAt, &r.Status,
			&r.ListingsFound, &r.ListingsNew, &r.PropertiesNew, &r.PropertiesRelisted,
			&r.ErrorsCount)
		if err != nil {
			return nil, err
		}
		if startedAt.Valid {
			r.StartedAt, _ = time.Parse(time.RFC3339, startedAt.String)
		}
		if finishedAt.Valid {
			t, _ := time.Parse(time.RFC3339, finishedAt.String)
			r.FinishedAt = &t
		}
		runs = append(runs, r)
	}
	return runs, nil
}

func (c *Client) GetProperties(limit int, unsyncedOnly bool) ([]Property, error) {
	query := `
		SELECT p.id, p.normalized_address, p.city, p.beds, p.baths, p.sqft, p.property_type,
			p.first_seen_at, p.last_seen_at, p.times_listed, p.synced,
			(SELECT ls.price FROM listing_snapshots ls
			 WHERE ls.property_id = p.id ORDER BY ls.scraped_at DESC LIMIT 1) as latest_price
		FROM properties p
	`
	if unsyncedOnly {
		query += " WHERE p.synced = FALSE"
	}
	query += " ORDER BY p.last_seen_at DESC LIMIT ?"

	rows, err := c.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var props []Property
	for rows.Next() {
		var p Property
		var firstSeen, lastSeen sql.NullString
		var latestPrice sql.NullInt64
		err := rows.Scan(&p.ID, &p.NormalizedAddress, &p.City, &p.Beds, &p.Baths,
			&p.Sqft, &p.PropertyType, &firstSeen, &lastSeen, &p.TimesListed, &p.Synced,
			&latestPrice)
		if err != nil {
			return nil, err
		}
		if firstSeen.Valid {
			p.FirstSeenAt, _ = time.Parse(time.RFC3339, firstSeen.String)
		}
		if lastSeen.Valid {
			p.LastSeenAt, _ = time.Parse(time.RFC3339, lastSeen.String)
		}
		if latestPrice.Valid {
			p.LatestPrice = int(latestPrice.Int64)
		}
		props = append(props, p)
	}
	return props, nil
}

func (c *Client) GetPropertyCount() (int, error) {
	var count int
	err := c.db.QueryRow("SELECT COUNT(*) FROM properties").Scan(&count)
	return count, err
}

func (c *Client) GetSnapshotCount() (int, error) {
	var count int
	err := c.db.QueryRow("SELECT COUNT(*) FROM listing_snapshots").Scan(&count)
	return count, err
}

func (c *Client) GetUnsyncedCount() (int, error) {
	var count int
	err := c.db.QueryRow("SELECT COUNT(*) FROM properties WHERE synced = FALSE").Scan(&count)
	return count, err
}

func (c *Client) GetSnapshotsForProperty(propertyID string) ([]Snapshot, error) {
	rows, err := c.db.Query(`
		SELECT id, property_id, listing_id, site_id, url, price,
			COALESCE(description, ''), COALESCE(realtor, '{}'), scraped_at
		FROM listing_snapshots WHERE property_id = ? ORDER BY scraped_at DESC
	`, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snaps []Snapshot
	for rows.Next() {
		var s Snapshot
		var scrapedAt sql.NullString
		var realtorJSON string
		err := rows.Scan(&s.ID, &s.PropertyID, &s.ListingID, &s.SiteID, &s.URL,
			&s.Price, &s.Description, &realtorJSON, &scrapedAt)
		if err != nil {
			return nil, err
		}
		if scrapedAt.Valid {
			s.ScrapedAt, _ = time.Parse(time.RFC3339, scrapedAt.String)
		}
		s.Realtor = parseRealtorJSON(realtorJSON)
		snaps = append(snaps, s)
	}
	return snaps, nil
}

func parseRealtorJSON(data string) RealtorInfo {
	var info RealtorInfo
	var raw struct {
		Company struct {
			Name  string `json:"name"`
			Phone string `json:"phone"`
		} `json:"company"`
		Agents []struct {
			Name  string `json:"name"`
			Phone string `json:"phone"`
		} `json:"agents"`
	}
	if err := json.Unmarshal([]byte(data), &raw); err == nil {
		info.CompanyName = raw.Company.Name
		info.CompanyPhone = raw.Company.Phone
		if len(raw.Agents) > 0 {
			info.AgentName = raw.Agents[0].Name
			info.AgentPhone = raw.Agents[0].Phone
		}
	}
	return info
}

func (c *Client) GetRecentLogs(limit int, level *string) ([]ScrapeLog, error) {
	var rows *sql.Rows
	var err error
	if level != nil && *level != "ALL" {
		rows, err = c.db.Query(`
			SELECT id, run_id, timestamp, level, message, site_id
			FROM scrape_logs WHERE level = ? ORDER BY timestamp DESC LIMIT ?
		`, *level, limit)
	} else {
		rows, err = c.db.Query(`
			SELECT id, run_id, timestamp, level, message, site_id
			FROM scrape_logs ORDER BY timestamp DESC LIMIT ?
		`, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []ScrapeLog
	for rows.Next() {
		var l ScrapeLog
		var ts sql.NullString
		var runID sql.NullInt64
		var siteID sql.NullString
		err := rows.Scan(&l.ID, &runID, &ts, &l.Level, &l.Message, &siteID)
		if err != nil {
			return nil, err
		}
		if ts.Valid {
			l.Timestamp, _ = time.Parse(time.RFC3339, ts.String)
		}
		if runID.Valid {
			id := int(runID.Int64)
			l.RunID = &id
		}
		if siteID.Valid {
			l.SiteID = &siteID.String
		}
		logs = append(logs, l)
	}
	return logs, nil
}

func (c *Client) GetExtractionStrategies() ([]ExtractionStrategy, error) {
	rows, err := c.db.Query(`
		SELECT site_id, strategy, priority, success_count, fail_count
		FROM extraction_strategies ORDER BY site_id, priority
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var strategies []ExtractionStrategy
	for rows.Next() {
		var s ExtractionStrategy
		err := rows.Scan(&s.SiteID, &s.Strategy, &s.Priority, &s.SuccessCount, &s.FailCount)
		if err != nil {
			return nil, err
		}
		strategies = append(strategies, s)
	}
	return strategies, nil
}

func (c *Client) SendCommand(command string, params map[string]interface{}) error {
	paramsJSON, _ := json.Marshal(params)
	_, err := c.db.Exec(`
		INSERT INTO commands (command, params, created_at)
		VALUES (?, ?, ?)
	`, command, string(paramsJSON), time.Now().Format(time.RFC3339))
	return err
}

func (c *Client) ScrapeNow() error {
	return c.SendCommand("scrape_now", nil)
}

func (c *Client) SyncNow() error {
	return c.SendCommand("sync_now", nil)
}
