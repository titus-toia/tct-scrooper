package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"tct_scrooper/models"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(ctx context.Context, connString string) (*PostgresStore, error) {
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	config.MaxConns = 10
	config.MinConns = 2
	config.MaxConnLifetime = 30 * time.Minute
	config.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	return &PostgresStore{pool: pool}, nil
}

func (s *PostgresStore) Close() {
	s.pool.Close()
}

func (s *PostgresStore) Pool() *pgxpool.Pool {
	return s.pool
}

// =============================================================================
// Properties
// =============================================================================

func (s *PostgresStore) UpsertProperty(ctx context.Context, p *models.DomainProperty) error {
	query := `
		INSERT INTO properties (
			id, fingerprint, country, province, city, postal_code, address_full,
			lat, lng, unit_number, floor, stories, property_type, year_built,
			lot_sqft, beds, baths, sqft, details, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21
		)
		ON CONFLICT (fingerprint) DO UPDATE SET
			province = COALESCE(EXCLUDED.province, properties.province),
			city = COALESCE(EXCLUDED.city, properties.city),
			postal_code = COALESCE(EXCLUDED.postal_code, properties.postal_code),
			address_full = COALESCE(EXCLUDED.address_full, properties.address_full),
			lat = COALESCE(EXCLUDED.lat, properties.lat),
			lng = COALESCE(EXCLUDED.lng, properties.lng),
			unit_number = COALESCE(EXCLUDED.unit_number, properties.unit_number),
			floor = COALESCE(NULLIF(EXCLUDED.floor, 0), properties.floor),
			stories = COALESCE(NULLIF(EXCLUDED.stories, 0), properties.stories),
			property_type = COALESCE(EXCLUDED.property_type, properties.property_type),
			year_built = COALESCE(EXCLUDED.year_built, properties.year_built),
			lot_sqft = COALESCE(EXCLUDED.lot_sqft, properties.lot_sqft),
			beds = COALESCE(EXCLUDED.beds, properties.beds),
			baths = COALESCE(EXCLUDED.baths, properties.baths),
			sqft = COALESCE(EXCLUDED.sqft, properties.sqft),
			details = COALESCE(EXCLUDED.details, properties.details),
			updated_at = NOW()
		RETURNING id`

	return s.pool.QueryRow(ctx, query,
		p.ID, p.Fingerprint, p.Country, p.Province, p.City, p.PostalCode, p.AddressFull,
		p.Lat, p.Lng, p.UnitNumber, p.Floor, p.Stories, p.PropertyType, p.YearBuilt,
		p.LotSqFt, p.Beds, p.Baths, p.SqFt, p.Details, p.CreatedAt, p.UpdatedAt,
	).Scan(&p.ID)
}

func (s *PostgresStore) GetPropertyByFingerprint(ctx context.Context, fingerprint string) (*models.DomainProperty, error) {
	query := `
		SELECT id, fingerprint, country, province, city, postal_code, address_full,
			lat, lng, unit_number, floor, stories, property_type, year_built,
			lot_sqft, beds, baths, sqft, details, created_at, updated_at
		FROM properties WHERE fingerprint = $1`

	var p models.DomainProperty
	err := s.pool.QueryRow(ctx, query, fingerprint).Scan(
		&p.ID, &p.Fingerprint, &p.Country, &p.Province, &p.City, &p.PostalCode, &p.AddressFull,
		&p.Lat, &p.Lng, &p.UnitNumber, &p.Floor, &p.Stories, &p.PropertyType, &p.YearBuilt,
		&p.LotSqFt, &p.Beds, &p.Baths, &p.SqFt, &p.Details, &p.CreatedAt, &p.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *PostgresStore) GetPropertyByID(ctx context.Context, id uuid.UUID) (*models.DomainProperty, error) {
	query := `
		SELECT id, fingerprint, country, province, city, postal_code, address_full,
			lat, lng, unit_number, floor, stories, property_type, year_built,
			lot_sqft, beds, baths, sqft, details, created_at, updated_at
		FROM properties WHERE id = $1`

	var p models.DomainProperty
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&p.ID, &p.Fingerprint, &p.Country, &p.Province, &p.City, &p.PostalCode, &p.AddressFull,
		&p.Lat, &p.Lng, &p.UnitNumber, &p.Floor, &p.Stories, &p.PropertyType, &p.YearBuilt,
		&p.LotSqFt, &p.Beds, &p.Baths, &p.SqFt, &p.Details, &p.CreatedAt, &p.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// =============================================================================
// Property Identifiers
// =============================================================================

func (s *PostgresStore) UpsertPropertyIdentifier(ctx context.Context, pi *models.PropertyIdentifier) error {
	query := `
		INSERT INTO property_identifiers (property_id, type, identifier, source)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (property_id, type, identifier) DO NOTHING`

	_, err := s.pool.Exec(ctx, query, pi.PropertyID, pi.Type, pi.Identifier, pi.Source)
	return err
}

// =============================================================================
// Listings
// =============================================================================

func (s *PostgresStore) UpsertListing(ctx context.Context, l *models.Listing) error {
	query := `
		INSERT INTO listings (
			id, property_id, source, external_id, url, type, status, price, currency,
			fees, property_type, beds, baths, sqft, sqft_lot, floor, stories,
			description, features, raw_data, last_seen, listed_at, delisted_at,
			enrichment_attempts, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26
		)
		ON CONFLICT (source, external_id) DO UPDATE SET
			url = COALESCE(EXCLUDED.url, listings.url),
			status = EXCLUDED.status,
			price = EXCLUDED.price,
			fees = COALESCE(EXCLUDED.fees, listings.fees),
			property_type = COALESCE(EXCLUDED.property_type, listings.property_type),
			beds = COALESCE(EXCLUDED.beds, listings.beds),
			baths = COALESCE(EXCLUDED.baths, listings.baths),
			sqft = COALESCE(EXCLUDED.sqft, listings.sqft),
			sqft_lot = COALESCE(EXCLUDED.sqft_lot, listings.sqft_lot),
			floor = COALESCE(NULLIF(EXCLUDED.floor, 0), listings.floor),
			stories = COALESCE(NULLIF(EXCLUDED.stories, 0), listings.stories),
			description = COALESCE(EXCLUDED.description, listings.description),
			features = COALESCE(EXCLUDED.features, listings.features),
			raw_data = EXCLUDED.raw_data,
			last_seen = EXCLUDED.last_seen,
			delisted_at = EXCLUDED.delisted_at,
			enrichment_attempts = COALESCE(EXCLUDED.enrichment_attempts, listings.enrichment_attempts),
			updated_at = NOW()
		RETURNING id`

	return s.pool.QueryRow(ctx, query,
		l.ID, l.PropertyID, l.Source, l.ExternalID, l.URL, l.Type, l.Status, l.Price, l.Currency,
		l.Fees, l.PropertyType, l.Beds, l.Baths, l.SqFt, l.SqFtLot, l.Floor, l.Stories,
		l.Description, l.Features, l.RawData, l.LastSeen, l.ListedAt, l.DelistedAt,
		l.EnrichmentAttempts, l.CreatedAt, l.UpdatedAt,
	).Scan(&l.ID)
}

func (s *PostgresStore) GetListingBySourceAndExternalID(ctx context.Context, source, externalID string) (*models.Listing, error) {
	query := `
		SELECT id, property_id, source, external_id, url, type, status, price, currency,
			fees, property_type, beds, baths, sqft, sqft_lot, floor, stories,
			description, features, raw_data, last_seen, listed_at, delisted_at,
			enrichment_attempts, created_at, updated_at
		FROM listings WHERE source = $1 AND external_id = $2`

	var l models.Listing
	err := s.pool.QueryRow(ctx, query, source, externalID).Scan(
		&l.ID, &l.PropertyID, &l.Source, &l.ExternalID, &l.URL, &l.Type, &l.Status, &l.Price, &l.Currency,
		&l.Fees, &l.PropertyType, &l.Beds, &l.Baths, &l.SqFt, &l.SqFtLot, &l.Floor, &l.Stories,
		&l.Description, &l.Features, &l.RawData, &l.LastSeen, &l.ListedAt, &l.DelistedAt,
		&l.EnrichmentAttempts, &l.CreatedAt, &l.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (s *PostgresStore) GetListingByID(ctx context.Context, id uuid.UUID) (*models.Listing, error) {
	query := `
		SELECT id, property_id, source, external_id, url, type, status, price, currency,
			fees, property_type, beds, baths, sqft, sqft_lot, floor, stories,
			description, features, raw_data, last_seen, listed_at, delisted_at,
			enrichment_attempts, created_at, updated_at
		FROM listings WHERE id = $1`

	var l models.Listing
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&l.ID, &l.PropertyID, &l.Source, &l.ExternalID, &l.URL, &l.Type, &l.Status, &l.Price, &l.Currency,
		&l.Fees, &l.PropertyType, &l.Beds, &l.Baths, &l.SqFt, &l.SqFtLot, &l.Floor, &l.Stories,
		&l.Description, &l.Features, &l.RawData, &l.LastSeen, &l.ListedAt, &l.DelistedAt,
		&l.EnrichmentAttempts, &l.CreatedAt, &l.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (s *PostgresStore) GetActiveListingForProperty(ctx context.Context, propertyID uuid.UUID) (*models.Listing, error) {
	query := `
		SELECT id, property_id, source, external_id, url, type, status, price, currency,
			fees, property_type, beds, baths, sqft, sqft_lot, floor, stories,
			description, features, raw_data, last_seen, listed_at, delisted_at,
			enrichment_attempts, created_at, updated_at
		FROM listings WHERE property_id = $1 AND status = 'active'
		LIMIT 1`

	var l models.Listing
	err := s.pool.QueryRow(ctx, query, propertyID).Scan(
		&l.ID, &l.PropertyID, &l.Source, &l.ExternalID, &l.URL, &l.Type, &l.Status, &l.Price, &l.Currency,
		&l.Fees, &l.PropertyType, &l.Beds, &l.Baths, &l.SqFt, &l.SqFtLot, &l.Floor, &l.Stories,
		&l.Description, &l.Features, &l.RawData, &l.LastSeen, &l.ListedAt, &l.DelistedAt,
		&l.EnrichmentAttempts, &l.CreatedAt, &l.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (s *PostgresStore) UpdateListingStatus(ctx context.Context, id uuid.UUID, status string, delistedAt *time.Time) error {
	query := `UPDATE listings SET status = $2, delisted_at = $3, updated_at = NOW() WHERE id = $1`
	_, err := s.pool.Exec(ctx, query, id, status, delistedAt)
	return err
}

// =============================================================================
// Property Events
// =============================================================================

func (s *PostgresStore) CreatePropertyEvent(ctx context.Context, e *models.PropertyEvent) error {
	query := `
		INSERT INTO property_events (
			property_id, event_type, event_date, price, previous_price,
			summary, source_type, source, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id`

	return s.pool.QueryRow(ctx, query,
		e.PropertyID, e.EventType, e.EventDate, e.Price, e.PreviousPrice,
		e.Summary, e.SourceType, e.Source, e.CreatedAt,
	).Scan(&e.ID)
}

func (s *PostgresStore) GetLatestEventForProperty(ctx context.Context, propertyID uuid.UUID, eventType string) (*models.PropertyEvent, error) {
	query := `
		SELECT id, property_id, event_type, event_date, price, previous_price,
			summary, source_type, source, created_at
		FROM property_events
		WHERE property_id = $1 AND event_type = $2
		ORDER BY event_date DESC
		LIMIT 1`

	var e models.PropertyEvent
	err := s.pool.QueryRow(ctx, query, propertyID, eventType).Scan(
		&e.ID, &e.PropertyID, &e.EventType, &e.EventDate, &e.Price, &e.PreviousPrice,
		&e.Summary, &e.SourceType, &e.Source, &e.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// =============================================================================
// Price Points
// =============================================================================

func (s *PostgresStore) CreatePricePoint(ctx context.Context, pp *models.PricePoint) error {
	query := `
		INSERT INTO price_points (
			property_id, listing_id, price_type, amount, currency, period, effective_at, source, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id`

	return s.pool.QueryRow(ctx, query,
		pp.PropertyID, pp.ListingID, pp.PriceType, pp.Amount, pp.Currency, pp.Period, pp.EffectiveAt, pp.Source, pp.CreatedAt,
	).Scan(&pp.ID)
}

func (s *PostgresStore) GetLatestPricePoint(ctx context.Context, propertyID uuid.UUID, priceType string) (*models.PricePoint, error) {
	query := `
		SELECT id, property_id, listing_id, price_type, amount, currency, period, effective_at, source, created_at
		FROM price_points
		WHERE property_id = $1 AND price_type = $2
		ORDER BY effective_at DESC
		LIMIT 1`

	var pp models.PricePoint
	err := s.pool.QueryRow(ctx, query, propertyID, priceType).Scan(
		&pp.ID, &pp.PropertyID, &pp.ListingID, &pp.PriceType, &pp.Amount, &pp.Currency, &pp.Period, &pp.EffectiveAt, &pp.Source, &pp.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &pp, nil
}

// =============================================================================
// Property Links
// =============================================================================

func (s *PostgresStore) UpsertPropertyLink(ctx context.Context, pl *models.PropertyLink) error {
	query := `
		INSERT INTO property_links (
			property_id, listing_id, url, site, link_type, is_primary, is_active,
			first_seen_at, last_seen_at, notes
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (url) DO UPDATE SET
			listing_id = COALESCE(EXCLUDED.listing_id, property_links.listing_id),
			is_active = EXCLUDED.is_active,
			last_seen_at = EXCLUDED.last_seen_at
		RETURNING id`

	return s.pool.QueryRow(ctx, query,
		pl.PropertyID, pl.ListingID, pl.URL, pl.Site, pl.LinkType, pl.IsPrimary, pl.IsActive,
		pl.FirstSeenAt, pl.LastSeenAt, pl.Notes,
	).Scan(&pl.ID)
}

// =============================================================================
// Media
// =============================================================================

func (s *PostgresStore) UpsertMedia(ctx context.Context, m *models.Media) error {
	query := `
		INSERT INTO media (
			id, s3_key, content_hash, media_type, category, province, city, mime_type, file_size_bytes,
			original_url, height, width, pages, duration, metadata, status, attempts, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		ON CONFLICT (original_url) DO UPDATE SET
			s3_key = COALESCE(EXCLUDED.s3_key, media.s3_key),
			content_hash = COALESCE(EXCLUDED.content_hash, media.content_hash),
			file_size_bytes = COALESCE(EXCLUDED.file_size_bytes, media.file_size_bytes),
			height = COALESCE(EXCLUDED.height, media.height),
			width = COALESCE(EXCLUDED.width, media.width),
			status = EXCLUDED.status,
			attempts = EXCLUDED.attempts
		RETURNING id`

	return s.pool.QueryRow(ctx, query,
		m.ID, m.S3Key, m.ContentHash, m.MediaType, m.Category, m.Province, m.City, m.MimeType, m.FileSizeBytes,
		m.OriginalURL, m.Height, m.Width, m.Pages, m.Duration, m.Metadata, m.Status, m.Attempts, m.CreatedAt,
	).Scan(&m.ID)
}

func (s *PostgresStore) GetMediaByOriginalURL(ctx context.Context, url string) (*models.Media, error) {
	query := `
		SELECT id, s3_key, content_hash, media_type, category, province, city, mime_type, file_size_bytes,
			original_url, height, width, pages, duration, metadata, status, attempts, created_at
		FROM media WHERE original_url = $1`

	var m models.Media
	err := s.pool.QueryRow(ctx, query, url).Scan(
		&m.ID, &m.S3Key, &m.ContentHash, &m.MediaType, &m.Category, &m.Province, &m.City, &m.MimeType, &m.FileSizeBytes,
		&m.OriginalURL, &m.Height, &m.Width, &m.Pages, &m.Duration, &m.Metadata, &m.Status, &m.Attempts, &m.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *PostgresStore) GetPendingMedia(ctx context.Context, limit int) ([]models.Media, error) {
	query := `
		SELECT id, s3_key, content_hash, media_type, category, province, city, mime_type, file_size_bytes,
			original_url, height, width, pages, duration, metadata, status, attempts, created_at
		FROM media
		WHERE status = 'pending' AND attempts < 3
		ORDER BY created_at
		LIMIT $1`

	rows, err := s.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var media []models.Media
	for rows.Next() {
		var m models.Media
		if err := rows.Scan(
			&m.ID, &m.S3Key, &m.ContentHash, &m.MediaType, &m.Category, &m.Province, &m.City, &m.MimeType, &m.FileSizeBytes,
			&m.OriginalURL, &m.Height, &m.Width, &m.Pages, &m.Duration, &m.Metadata, &m.Status, &m.Attempts, &m.CreatedAt,
		); err != nil {
			return nil, err
		}
		media = append(media, m)
	}
	return media, rows.Err()
}

func (s *PostgresStore) UpdateMediaStatus(ctx context.Context, id uuid.UUID, status string, s3Key *string, contentHash string, attempts int) error {
	query := `UPDATE media SET status = $2, s3_key = COALESCE($3, s3_key), content_hash = COALESCE($4, content_hash), attempts = $5 WHERE id = $1`
	_, err := s.pool.Exec(ctx, query, id, status, s3Key, contentHash, attempts)
	return err
}

// =============================================================================
// Listing Media
// =============================================================================

func (s *PostgresStore) UpsertListingMedia(ctx context.Context, lm *models.ListingMedia) error {
	query := `
		INSERT INTO listing_media (listing_id, media_id, position)
		VALUES ($1, $2, $3)
		ON CONFLICT (listing_id, media_id) DO UPDATE SET position = EXCLUDED.position`

	_, err := s.pool.Exec(ctx, query, lm.ListingID, lm.MediaID, lm.Position)
	return err
}

// =============================================================================
// Brokerages
// =============================================================================

func (s *PostgresStore) UpsertBrokerage(ctx context.Context, b *models.Brokerage) error {
	query := `
		INSERT INTO brokerages (id, name, brand, phone, email, website, address, country, city, province, logo_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO UPDATE SET
			name = COALESCE(EXCLUDED.name, brokerages.name),
			brand = COALESCE(EXCLUDED.brand, brokerages.brand),
			phone = COALESCE(EXCLUDED.phone, brokerages.phone),
			email = COALESCE(EXCLUDED.email, brokerages.email),
			website = COALESCE(EXCLUDED.website, brokerages.website),
			address = COALESCE(EXCLUDED.address, brokerages.address)
		RETURNING id`

	return s.pool.QueryRow(ctx, query,
		b.ID, b.Name, b.Brand, b.Phone, b.Email, b.Website, b.Address, b.Country, b.City, b.Province, b.LogoID, b.CreatedAt,
	).Scan(&b.ID)
}

func (s *PostgresStore) GetBrokerageByName(ctx context.Context, name string) (*models.Brokerage, error) {
	query := `
		SELECT id, name, brand, phone, email, website, address, country, city, province, logo_id, created_at
		FROM brokerages WHERE name = $1`

	var b models.Brokerage
	err := s.pool.QueryRow(ctx, query, name).Scan(
		&b.ID, &b.Name, &b.Brand, &b.Phone, &b.Email, &b.Website, &b.Address, &b.Country, &b.City, &b.Province, &b.LogoID, &b.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// =============================================================================
// Agents
// =============================================================================

func (s *PostgresStore) UpsertAgent(ctx context.Context, a *models.Agent) error {
	query := `
		INSERT INTO agents (id, full_name, license_number, email, phone, bio, brokerage_id, headshot_id, first_seen_at, last_seen_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (id) DO UPDATE SET
			full_name = COALESCE(EXCLUDED.full_name, agents.full_name),
			license_number = COALESCE(EXCLUDED.license_number, agents.license_number),
			email = COALESCE(EXCLUDED.email, agents.email),
			phone = COALESCE(EXCLUDED.phone, agents.phone),
			bio = COALESCE(EXCLUDED.bio, agents.bio),
			brokerage_id = COALESCE(EXCLUDED.brokerage_id, agents.brokerage_id),
			headshot_id = COALESCE(EXCLUDED.headshot_id, agents.headshot_id),
			last_seen_at = EXCLUDED.last_seen_at
		RETURNING id`

	return s.pool.QueryRow(ctx, query,
		a.ID, a.FullName, a.LicenseNumber, a.Email, a.Phone, a.Bio, a.BrokerageID, a.HeadshotID, a.FirstSeenAt, a.LastSeenAt, a.CreatedAt,
	).Scan(&a.ID)
}

func (s *PostgresStore) GetAgentByNameAndBrokerage(ctx context.Context, name string, brokerageID *uuid.UUID) (*models.Agent, error) {
	var query string
	var args []interface{}

	if brokerageID != nil {
		query = `
			SELECT id, full_name, license_number, email, phone, bio, brokerage_id, headshot_id, first_seen_at, last_seen_at, created_at
			FROM agents WHERE full_name = $1 AND brokerage_id = $2`
		args = []interface{}{name, brokerageID}
	} else {
		query = `
			SELECT id, full_name, license_number, email, phone, bio, brokerage_id, headshot_id, first_seen_at, last_seen_at, created_at
			FROM agents WHERE full_name = $1 AND brokerage_id IS NULL`
		args = []interface{}{name}
	}

	var a models.Agent
	err := s.pool.QueryRow(ctx, query, args...).Scan(
		&a.ID, &a.FullName, &a.LicenseNumber, &a.Email, &a.Phone, &a.Bio, &a.BrokerageID, &a.HeadshotID, &a.FirstSeenAt, &a.LastSeenAt, &a.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// =============================================================================
// Listing Agents
// =============================================================================

func (s *PostgresStore) UpsertListingAgent(ctx context.Context, la *models.ListingAgent) error {
	query := `
		INSERT INTO listing_agents (listing_id, agent_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (listing_id, agent_id) DO UPDATE SET role = EXCLUDED.role`

	_, err := s.pool.Exec(ctx, query, la.ListingID, la.AgentID, la.Role)
	return err
}

// =============================================================================
// Property Matches
// =============================================================================

func (s *PostgresStore) InsertPropertyMatch(ctx context.Context, pm *models.PropertyMatch) error {
	query := `
		INSERT INTO property_matches (matched_id, incoming_id, confidence, match_reasons, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (matched_id, incoming_id) DO NOTHING
		RETURNING id`

	err := s.pool.QueryRow(ctx, query,
		pm.MatchedID, pm.IncomingID, pm.Confidence, pm.MatchReasons, pm.Status, pm.CreatedAt,
	).Scan(&pm.ID)

	if err == pgx.ErrNoRows {
		return nil // conflict, no insert
	}
	return err
}

// =============================================================================
// Scrape Runs
// =============================================================================

func (s *PostgresStore) CreateScrapeRun(ctx context.Context, run *models.DomainScrapeRun) error {
	query := `
		INSERT INTO scrape_runs (source, started_at, status, listings_found, listings_new, properties_new, errors_count, error_message, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id`

	return s.pool.QueryRow(ctx, query,
		run.Source, run.StartedAt, run.Status, run.ListingsFound, run.ListingsNew, run.PropertiesNew, run.ErrorsCount, run.ErrorMessage, run.Metadata,
	).Scan(&run.ID)
}

func (s *PostgresStore) UpdateScrapeRun(ctx context.Context, run *models.DomainScrapeRun) error {
	query := `
		UPDATE scrape_runs SET
			finished_at = $2, status = $3, listings_found = $4, listings_new = $5,
			properties_new = $6, errors_count = $7, error_message = $8, metadata = $9
		WHERE id = $1`

	_, err := s.pool.Exec(ctx, query,
		run.ID, run.FinishedAt, run.Status, run.ListingsFound, run.ListingsNew, run.PropertiesNew, run.ErrorsCount, run.ErrorMessage, run.Metadata,
	)
	return err
}

// =============================================================================
// Scrape Logs
// =============================================================================

func (s *PostgresStore) CreateScrapeLog(ctx context.Context, log *models.DomainScrapeLog) error {
	query := `
		INSERT INTO scrape_logs (run_id, timestamp, level, message, source_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`

	return s.pool.QueryRow(ctx, query,
		log.RunID, log.Timestamp, log.Level, log.Message, log.SourceID,
	).Scan(&log.ID)
}

// =============================================================================
// Healthcheck Queries
// =============================================================================

func (s *PostgresStore) GetStaleActiveListings(ctx context.Context, staleDuration time.Duration, limit int) ([]models.Listing, error) {
	query := `
		SELECT id, property_id, source, external_id, url, type, status, price, currency,
			fees, property_type, beds, baths, sqft, sqft_lot, floor, stories,
			description, features, raw_data, last_seen, listed_at, delisted_at,
			enrichment_attempts, created_at, updated_at
		FROM listings
		WHERE status = 'active' AND last_seen < $1
		ORDER BY last_seen
		LIMIT $2`

	staleTime := time.Now().Add(-staleDuration)
	rows, err := s.pool.Query(ctx, query, staleTime, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var listings []models.Listing
	for rows.Next() {
		var l models.Listing
		if err := rows.Scan(
			&l.ID, &l.PropertyID, &l.Source, &l.ExternalID, &l.URL, &l.Type, &l.Status, &l.Price, &l.Currency,
			&l.Fees, &l.PropertyType, &l.Beds, &l.Baths, &l.SqFt, &l.SqFtLot, &l.Floor, &l.Stories,
			&l.Description, &l.Features, &l.RawData, &l.LastSeen, &l.ListedAt, &l.DelistedAt,
			&l.EnrichmentAttempts, &l.CreatedAt, &l.UpdatedAt,
		); err != nil {
			return nil, err
		}
		listings = append(listings, l)
	}
	return listings, rows.Err()
}
