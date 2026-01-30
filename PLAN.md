# TCT Scrooper Architecture Plan

## Decisions Made

- **Hosting**: DigitalOcean droplet (not serverless)
- **Database**: Supabase (PostgreSQL) for all data - domain + operational
- **Code structure**: Flat (not enterprise-y) - 6 files, not 20
- **Workers**: Always-on daemon with multiple responsibilities
- **Deployment**: Existing bash script + systemd
- **Client access**: Supabase integrations (Zapier, Make, n8n) + NocoDB later

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Droplet ($6/mo)                      │
│                                                         │
│  ┌─────────────────────────────────────────────────┐   │
│  │              Scraper Daemon                      │   │
│  │                                                  │   │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────────────┐│   │
│  │  │ Discovery│ │Enrichment│ │   Healthcheck    ││   │
│  │  │ (Apify)  │ │(Webshare)│ │   (Webshare)     ││   │
│  │  └────┬─────┘ └────┬─────┘ └────────┬─────────┘│   │
│  │       │            │                │          │   │
│  │       └────────────┴────────────────┘          │   │
│  │                    │                            │   │
│  └────────────────────┼────────────────────────────┘   │
│                       │                                 │
└───────────────────────┼─────────────────────────────────┘
                        │
                        ▼
              ┌─────────────────┐
              │    Supabase     │
              │   (PostgreSQL)  │
              │                 │
              │  - properties   │
              │  - listings     │
              │  - events       │
              │  - price_points │
              │  - media        │
              │  - agents       │
              │  - scrape_runs  │
              └────────┬────────┘
                       │
           ┌───────────┴───────────┐
           ▼                       ▼
    ┌─────────────┐         ┌─────────────┐
    │   Client    │         │  NocoDB     │
    │ Automations │         │ (later)     │
    │ Zapier/Make │         │             │
    └─────────────┘         └─────────────┘
```

## Daemon Responsibilities

| Worker | Trigger | Action |
|--------|---------|--------|
| **Discovery** | Cron (every 6hr per city) | Kick off Apify → process results → Supabase |
| **Enrichment** | Queue (continuous) | Pull unenriched listings → Webshare GET → parse photos/details → Supabase |
| **Healthcheck** | Queue (continuous) | Pull stale listings → Webshare GET → mark delisted if 404 |

## File Structure (Flat)

```
tct_scrooper/
├── main.go           # Entry point, config, scheduler setup
├── models.go         # All structs (Property, Listing, Event, PricePoint, Media, Agent)
├── db.go             # All Supabase/pgx operations
├── scraper.go        # Apify handler, orchestration
├── enrichment.go     # Direct HTTP scraping via Webshare, parsing
├── process.go        # ProcessListing - the fan-out logic
├── schema_v2.sql     # PostgreSQL schema (already exists)
└── PLAN.md           # This file
```

## ProcessListing Flow

One listing from Apify fans out to multiple table writes:

```
RawListing arrives
│
├─► properties         - find by fingerprint OR create
├─► property_identifiers - add MLS as identifier
├─► listings           - create or update
├─► property_events    - "listed" / "relisted" / "price_change"
├─► price_points       - asking_sale or asking_rent
├─► property_links     - the URL
│
├─► media (N)          - one per photo URL
├─► listing_media (N)  - link each photo to listing
│
├─► brokerages         - find or create
├─► agents             - find or create
└─► listing_agents     - link agents to listing
```

## Database Schema

Full schema: `schema_v2.sql`

Core tables:
- `properties` - permanent physical entities
- `listings` - sales/rental sessions
- `property_events` - unified timeline
- `price_points` - all prices (asking, assessed, fees)
- `media` + `listing_media` - photos
- `agents` + `brokerages` + `listing_agents` - realtor info
- `scrape_runs` + `scrape_logs` - operational tracking

## Implementation Phases

### Phase 1: Database Setup
1. Create Supabase project
2. Run `schema_v2.sql` to create tables
3. Set up connection string in .env

### Phase 2: Core Flat Structure
1. Create `models.go` - all domain structs
2. Create `db.go` - pgx connection + all CRUD operations
3. Modify `main.go` - init pgx pool, remove old SQLite domain stuff

### Phase 3: ProcessListing Rewrite
1. Create `process.go` - the fan-out logic
2. Wire into existing orchestrator
3. Test: one listing creates property + listing + event + price_point

### Phase 4: Enrichment Worker
1. Create `enrichment.go` - Webshare client, HTML parsing
2. Add enrichment queue loop to daemon
3. Test: unenriched listing gets photos populated

### Phase 5: Healthcheck Worker
1. Add healthcheck queue loop to daemon
2. Mark listings as delisted on 404/301
3. Create "delisted" events

### Phase 6: Cleanup
1. Remove old `storage/supabase.go` (sync logic)
2. Remove SQLite domain tables (keep only if needed for TUI)
3. Update TUI to query Supabase (or deprecate)

## Config (.env additions)

```env
# Supabase (replaces SQLite for domain data)
DATABASE_URL=postgres://user:pass@db.xxx.supabase.co:5432/postgres

# Webshare proxy (for enrichment + healthcheck)
WEBSHARE_PROXY_URL=http://user:pass@proxy.webshare.io:8080

# Apify (unchanged)
APIFY_API_KEY=xxx
```

## Verification

1. **Discovery**: Run scrape → check Supabase for new properties/listings
2. **Events**: Verify `property_events` has "listed" entries
3. **Price tracking**: Change a price → verify "price_change" event + new price_point
4. **Enrichment**: Check listing has photos populated in `media` + `listing_media`
5. **Healthcheck**: Manually 404 a listing → verify marked delisted

## Future Additions (on same droplet)

- **NocoDB** - Client data viewer ($0, Docker)
- **n8n** - Client automations ($0, Docker)
- **Uptime Kuma** - Monitoring ($0, Docker)
- **Redis** - Rate limiting, caching if needed ($0, Docker)
- **Nginx** - Reverse proxy when exposing services ($0, apt)

## Notes

- No SQLite for domain data - Supabase handles everything
- No serverless - continuous workers need always-on daemon
- No enterprise architecture - flat files, direct function calls
- Webshare handles proxies for direct HTTP (enrichment/healthcheck)
- Apify handles proxies for discovery scraping
