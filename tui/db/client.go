package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "modernc.org/sqlite"
)

type Client struct {
	pg     *pgxpool.Pool
	sqlite *sql.DB // Keep SQLite for commands (daemon reads from there)
	ctx    context.Context
}

type SiteStats struct {
	SiteID           string
	LastRunAt        *time.Time
	LastRunStatus    *string
	TotalProperties  int
	TotalListings    int
	SuccessRate      float64
	AvgRunDuration   int
}

type ScrapeRun struct {
	ID                 int64
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
	ID          string // UUID as string
	Address     string
	City        string
	Province    string
	PostalCode  string
	Beds        int
	Baths       int
	Sqft        int
	PropertyType string
	YearBuilt   int
	FirstSeenAt time.Time
	LastSeenAt  time.Time
	TimesListed int
	LatestPrice int64
}

type Listing struct {
	ID          string
	PropertyID  string
	Source      string
	ExternalID  string
	URL         string
	Type        string
	Status      string
	Price       int64
	Beds        int
	Baths       int
	Sqft        int
	Description string
	ListedAt    time.Time
	Agent       *AgentInfo
	Brokerage   *BrokerageInfo
}

type AgentInfo struct {
	Name  string
	Phone string
	Email string
}

type BrokerageInfo struct {
	Name    string
	Phone   string
	Website string
}

type ScrapeLog struct {
	ID        int64
	RunID     *int64
	Timestamp time.Time
	Level     string
	Message   string
	SourceID  *string
}

type PricePoint struct {
	Amount      int64
	EffectiveAt time.Time
	PriceType   string
	Source      string
}

func New(postgresURL, sqlitePath string) (*Client, error) {
	ctx := context.Background()

	// Connect to Postgres
	pgPool, err := pgxpool.New(ctx, postgresURL)
	if err != nil {
		return nil, err
	}

	// Keep SQLite for commands
	sqliteDB, err := sql.Open("sqlite", sqlitePath)
	if err != nil {
		pgPool.Close()
		return nil, err
	}

	return &Client{
		pg:     pgPool,
		sqlite: sqliteDB,
		ctx:    ctx,
	}, nil
}

func (c *Client) Close() error {
	c.pg.Close()
	return c.sqlite.Close()
}

func (c *Client) GetSiteStats() ([]SiteStats, error) {
	// Derive site stats from scrape_runs
	rows, err := c.pg.Query(c.ctx, `
		WITH latest_runs AS (
			SELECT DISTINCT ON (source)
				source, started_at, finished_at, status,
				listings_found, listings_new, properties_new, errors_count
			FROM scrape_runs
			ORDER BY source, started_at DESC
		),
		run_stats AS (
			SELECT
				source,
				COUNT(*) as total_runs,
				COUNT(*) FILTER (WHERE status = 'completed') as successful_runs,
				AVG(EXTRACT(EPOCH FROM (finished_at - started_at)))::int as avg_duration
			FROM scrape_runs
			WHERE finished_at IS NOT NULL
			GROUP BY source
		)
		SELECT
			lr.source,
			lr.started_at,
			lr.status,
			COALESCE((SELECT COUNT(*) FROM properties), 0)::int as total_properties,
			COALESCE((SELECT COUNT(*) FROM listings WHERE source = lr.source), 0)::int as total_listings,
			COALESCE(rs.successful_runs::float / NULLIF(rs.total_runs, 0), 0) as success_rate,
			COALESCE(rs.avg_duration, 0) as avg_duration
		FROM latest_runs lr
		LEFT JOIN run_stats rs ON lr.source = rs.source
		ORDER BY lr.source
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []SiteStats
	for rows.Next() {
		var s SiteStats
		var lastRunAt *time.Time
		var status *string
		err := rows.Scan(&s.SiteID, &lastRunAt, &status,
			&s.TotalProperties, &s.TotalListings, &s.SuccessRate, &s.AvgRunDuration)
		if err != nil {
			return nil, err
		}
		s.LastRunAt = lastRunAt
		s.LastRunStatus = status
		stats = append(stats, s)
	}
	return stats, nil
}

func (c *Client) GetRecentRuns(limit int) ([]ScrapeRun, error) {
	rows, err := c.pg.Query(c.ctx, `
		SELECT id, source, started_at, finished_at, status,
			COALESCE(listings_found, 0), COALESCE(listings_new, 0),
			COALESCE(properties_new, 0), 0, COALESCE(errors_count, 0)
		FROM scrape_runs
		ORDER BY started_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []ScrapeRun
	for rows.Next() {
		var r ScrapeRun
		err := rows.Scan(&r.ID, &r.SiteID, &r.StartedAt, &r.FinishedAt, &r.Status,
			&r.ListingsFound, &r.ListingsNew, &r.PropertiesNew,
			&r.PropertiesRelisted, &r.ErrorsCount)
		if err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, nil
}

func (c *Client) GetProperties(limit, offset int, activeOnly bool) ([]Property, error) {
	query := `
		SELECT
			p.id::text,
			COALESCE(p.address_full, ''),
			COALESCE(p.city, ''),
			COALESCE(p.province, ''),
			COALESCE(p.postal_code, ''),
			COALESCE(p.beds, 0),
			COALESCE(p.baths, 0),
			COALESCE(p.sqft, 0),
			COALESCE(p.property_type, ''),
			COALESCE(p.year_built, 0),
			p.created_at,
			p.updated_at,
			(SELECT COUNT(*) FROM listings WHERE property_id = p.id)::int,
			COALESCE((
				SELECT l.price::bigint
				FROM listings l
				WHERE l.property_id = p.id
				ORDER BY l.created_at DESC LIMIT 1
			), 0)
		FROM properties p
	`
	if activeOnly {
		query += ` WHERE EXISTS (SELECT 1 FROM listings l WHERE l.property_id = p.id AND l.status = 'active')`
	}
	query += ` ORDER BY p.updated_at DESC LIMIT $1 OFFSET $2`

	rows, err := c.pg.Query(c.ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var props []Property
	for rows.Next() {
		var p Property
		err := rows.Scan(&p.ID, &p.Address, &p.City, &p.Province, &p.PostalCode,
			&p.Beds, &p.Baths, &p.Sqft, &p.PropertyType, &p.YearBuilt,
			&p.FirstSeenAt, &p.LastSeenAt, &p.TimesListed, &p.LatestPrice)
		if err != nil {
			return nil, err
		}
		props = append(props, p)
	}
	return props, nil
}

func (c *Client) GetPropertyCount() (int, error) {
	var count int
	err := c.pg.QueryRow(c.ctx, "SELECT COUNT(*) FROM properties").Scan(&count)
	return count, err
}

func (c *Client) GetListingCount() (int, error) {
	var count int
	err := c.pg.QueryRow(c.ctx, "SELECT COUNT(*) FROM listings").Scan(&count)
	return count, err
}

func (c *Client) GetActiveListingCount() (int, error) {
	var count int
	err := c.pg.QueryRow(c.ctx, "SELECT COUNT(*) FROM listings WHERE status = 'active'").Scan(&count)
	return count, err
}

func (c *Client) GetListingsForProperty(propertyID string) ([]Listing, error) {
	rows, err := c.pg.Query(c.ctx, `
		SELECT
			l.id::text,
			l.property_id::text,
			l.source,
			COALESCE(l.external_id, ''),
			COALESCE(l.url, ''),
			l.type,
			COALESCE(l.status, ''),
			COALESCE(l.price, 0)::bigint,
			COALESCE(l.beds, 0),
			COALESCE(l.baths, 0),
			COALESCE(l.sqft, 0),
			COALESCE(l.description, ''),
			COALESCE(l.listed_at, l.created_at),
			a.full_name,
			a.phone,
			a.email,
			b.name,
			b.phone,
			b.website
		FROM listings l
		LEFT JOIN listing_agents la ON la.listing_id = l.id
		LEFT JOIN agents a ON a.id = la.agent_id
		LEFT JOIN brokerages b ON b.id = a.brokerage_id
		WHERE l.property_id = $1
		ORDER BY l.created_at DESC
	`, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var listings []Listing
	for rows.Next() {
		var l Listing
		var agentName, agentPhone, agentEmail *string
		var brokerageName, brokeragePhone, brokerageWebsite *string

		err := rows.Scan(&l.ID, &l.PropertyID, &l.Source, &l.ExternalID, &l.URL,
			&l.Type, &l.Status, &l.Price, &l.Beds, &l.Baths, &l.Sqft,
			&l.Description, &l.ListedAt,
			&agentName, &agentPhone, &agentEmail,
			&brokerageName, &brokeragePhone, &brokerageWebsite)
		if err != nil {
			return nil, err
		}

		if agentName != nil && *agentName != "" {
			l.Agent = &AgentInfo{
				Name:  *agentName,
				Phone: deref(agentPhone),
				Email: deref(agentEmail),
			}
		}
		if brokerageName != nil && *brokerageName != "" {
			l.Brokerage = &BrokerageInfo{
				Name:    *brokerageName,
				Phone:   deref(brokeragePhone),
				Website: deref(brokerageWebsite),
			}
		}

		listings = append(listings, l)
	}
	return listings, nil
}

func (c *Client) GetPriceHistory(propertyID string) ([]PricePoint, error) {
	rows, err := c.pg.Query(c.ctx, `
		SELECT amount::bigint, effective_at, price_type, COALESCE(source, '')
		FROM price_points
		WHERE property_id = $1
		ORDER BY effective_at DESC
		LIMIT 20
	`, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []PricePoint
	for rows.Next() {
		var p PricePoint
		err := rows.Scan(&p.Amount, &p.EffectiveAt, &p.PriceType, &p.Source)
		if err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, nil
}

func (c *Client) GetRecentLogs(limit int, level *string) ([]ScrapeLog, error) {
	var rows pgx.Rows
	var err error

	if level != nil && *level != "ALL" {
		rows, err = c.pg.Query(c.ctx, `
			SELECT id, run_id, timestamp, level, message, source_id
			FROM scrape_logs
			WHERE level = $1
			ORDER BY timestamp DESC
			LIMIT $2
		`, *level, limit)
	} else {
		rows, err = c.pg.Query(c.ctx, `
			SELECT id, run_id, timestamp, level, message, source_id
			FROM scrape_logs
			ORDER BY timestamp DESC
			LIMIT $1
		`, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []ScrapeLog
	for rows.Next() {
		var l ScrapeLog
		err := rows.Scan(&l.ID, &l.RunID, &l.Timestamp, &l.Level, &l.Message, &l.SourceID)
		if err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, nil
}

// Commands still go through SQLite (daemon reads from there)
func (c *Client) SendCommand(command string, params map[string]interface{}) error {
	_, err := c.sqlite.Exec(`
		INSERT INTO commands (command, params, created_at)
		VALUES (?, '{}', datetime('now'))
	`, command)
	return err
}

func (c *Client) ScrapeNow() error {
	return c.SendCommand("scrape_now", nil)
}

func (c *Client) SyncNow() error {
	// Sync is no longer needed with direct Postgres writes
	// But keep the command for backwards compatibility
	return c.SendCommand("sync_now", nil)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
