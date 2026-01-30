# TCT Scrooper Architecture Plan

## Goals

- Scrape at scale with retries and partial-failure safety
- Persist a clean property timeline (listings, events, price points)
- Keep scraping fast while moving heavy work to background workers
- Stay lightweight: modular Go, not a heavyweight framework

## Invariants (Requisite Constraints)

- **System of record**: Supabase/Postgres holds all domain + operational data.
- **DB access**: direct Postgres via pgx (not REST API).
- **Always-on daemon**: one droplet runs the scraper + workers continuously.
- **At-least-once ingest**: reprocessing must be safe and idempotent.
- **Single listing fan-out**: all derived writes happen in one logical flow.
- **Media is async**: URLs first; S3 upload happens later via a worker.
- **Dedup is explicit**: property identity uses fingerprint + `property_matches`.
- **No heavy framework**: small packages with clear boundaries.

## Operational Decisions

- **ProcessListing retries**: no explicit retry loop; failures are logged and picked up by the next scrape run.
- **Dedup evaluation**: only insert `property_matches` for now; review/merge logic comes later.
- **Migration/backfill**: no migrations; when needed we drop and re-seed from fresh scrapes.
- **Retention**: old/unprocessed data can be discarded; logging is required.

## Architecture (High Level)

```
┌─────────────────────────────────────────────────────────┐
│                    Droplet ($6/mo)                      │
│                                                         │
│  ┌─────────────────────────────────────────────────┐   │
│  │                Scraper Daemon                   │   │
│  │                                                 │   │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐        │   │
│  │  │ Discovery│ │Enrichment│ │  Media   │        │   │
│  │  │ (Apify)  │ │(Webshare)│ │  Worker  │        │   │
│  │  └────┬─────┘ └────┬─────┘ └────┬─────┘        │   │
│  │       │            │            │              │   │
│  │       └────────────┴────────────┴─────────────┐│   │
│  │                    │                          ││   │
│  └────────────────────┼──────────────────────────┘│   │
│                       │                           │   │
└───────────────────────┼───────────────────────────┘   │
                        ▼                               │
                 ┌──────────────┐                       │
                 │  Supabase    │                       │
                 │  PostgreSQL  │                       │
                 └──────┬───────┘                       │
                        │                               │
           ┌────────────┴────────────┐                  │
           ▼                         ▼                  │
    Client Automations           NocoDB (later)         │
```

## Services (Light Service Layer)

- `ListingService.ProcessListing()` — fan-out to 10+ tables, idempotent.
- `MatchService.InsertPotentialMatches()` — `property_matches` (pending).
- `MediaService.Enqueue()` — create `media` rows with URL + status.
- `HealthcheckService.MarkDelisted()` — update listing status/events.

## File Structure (Modular, not heavy)

```
tct_scrooper/
├── cmd/                 # main entrypoints
├── scraper/             # adapters (Apify, browser, API)
├── services/            # use-cases / orchestration
├── storage/             # DB access (pgx)
├── workers/             # enrichment/media/healthcheck loops
├── identity/            # fingerprint + normalization
├── models/              # shared structs
├── config/              # config + sites
├── schema_v2.sql         # PostgreSQL schema
└── PLAN.md               # this file
```

## ProcessListing Flow

```
RawListing arrives
│
├─► properties           - find by fingerprint OR create
├─► property_matches     - insert potential dupes (pending)
├─► property_identifiers - add MLS as identifier
├─► listings             - create or update
├─► property_events      - "listed" / "relisted" / "price_change"
├─► price_points         - asking_sale or asking_rent
├─► property_links       - the URL
│
├─► media (N)            - create rows w/ original_url, status=pending
├─► listing_media (N)    - link each photo to listing (when ready)
│
├─► brokerages           - find or create
├─► agents               - find or create
└─► listing_agents       - link agents to listing
```

## Media Handling

- `media` doubles as a lightweight queue:
  - `original_url` set on ingest
  - `s3_key` is nullable until worker completes upload
  - `status` + `attempts` track progress/retries
- Media worker:
  - download → hash → upload S3 → update `media`
  - set `listing_media` or agent/brokerage links

## Database Schema

Full schema: `schema_v2.sql`

Core tables:
- `properties` - permanent physical entities
- `listings` - sales/rental sessions
- `property_events` - unified timeline
- `price_points` - all prices (asking, assessed, fees)
- `media` + `listing_media` - photos (URLs first, S3 later)
- `agents` + `brokerages` + `listing_agents` - realtor info
- `property_matches` - potential dedup candidates
- `scrape_runs` + `scrape_logs` - operational tracking

## Implementation Phases

### Phase 1: Database Setup
1. Create Supabase project
2. Run `schema_v2.sql` to create tables
3. Set up connection string in `.env`

### Phase 2: Core Modular Structure
1. Add `services/` with listing + media + match services
2. Add `storage/pgx` access layer (idempotent upserts)
3. Wire in `cmd/` entrypoint
4. Switch Supabase access from REST to direct pgx

### Phase 3: Listing Ingest Fan-Out
1. Implement `ListingService.ProcessListing`
2. Ensure idempotent inserts/updates
3. Test: one listing creates property + listing + event + price_point

### Phase 4: Enrichment Worker
1. Create `workers/enrichment` - Webshare client, HTML parsing
2. Add continuous queue loop
3. Test: unenriched listing gets details + photo URLs

### Phase 5: Media Worker
1. Create media worker to fetch/upload media to S3
2. Update `media` rows with `s3_key`, `content_hash`, `status`
3. Retry failures via `attempts`

### Phase 6: Healthcheck Worker
1. Add healthcheck queue loop
2. Mark listings as delisted on 404/301
3. Create "delisted" events

### Phase 7: Cleanup + TUI
1. Remove old sync logic
2. Remove SQLite domain tables (keep only if needed for TUI)
3. Update TUI to query Supabase (or deprecate)

## Testing Strategy

- **Fixture-based parsing**: JSON fixtures in `scraper/testdata/` cover Realtor.ca variations (beds like "3 + 1", missing photos, missing agents).
- **Service-layer tests**: feed `RawListing` into `ListingService.ProcessListing()` with a real Postgres test DB; verify idempotency + event creation.
- **Worker tests**: use `httptest` to mock Webshare responses; media worker uses a fake uploader for S3.
- **Mocked scrapers**: handlers return fixtures to validate end-to-end flow without hitting real APIs.

## Operational Observability

- **Logging**: write to `scrape_logs` plus stdout/stderr (systemd captures).
- **Run metrics**: `scrape_runs` for counts, error rates, and durations.
- **Run source**: store runner type in `scrape_runs.source` (e.g., `browser`, `apify:<actor>`), and attach the Apify run ID in `metadata`.
- **Queue depth**: count `media` rows by `status` to monitor backlog.

## Config (.env additions)

```env
# Supabase (replaces SQLite for domain data)
SUPABASE_DB_URL=postgres://user:pass@db.xxx.supabase.co:5432/postgres

# Webshare proxy (for enrichment + healthcheck)
WEBSHARE_PROXY_URL=http://user:pass@proxy.webshare.io:8080

# Apify (unchanged)
APIFY_API_KEY=xxx
```

## Verification

1. **Discovery**: Run scrape → check Supabase for new properties/listings
2. **Events**: Verify `property_events` has "listed" entries
3. **Price tracking**: Change a price → verify "price_change" event + new price_point
4. **Media**: Check `media.status` transitions to `uploaded`
5. **Healthcheck**: 404 a listing → verify marked delisted

## Future Additions (same droplet)

- **NocoDB** - Client data viewer ($0, Docker)
- **n8n** - Client automations ($0, Docker)
- **Uptime Kuma** - Monitoring ($0, Docker)
- **Redis** - Rate limiting/caching if needed
- **Nginx** - Reverse proxy if exposing services

## Notes

- No serverless: continuous workers need always-on daemon
- Media ingestion is separate from scrape/enrichment
- Keep code modular, but resist heavy framework overhead
