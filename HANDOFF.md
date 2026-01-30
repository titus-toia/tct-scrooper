# TCT Scrooper – Execution Handoff Brief

## Goal
Implement the modular Go architecture in `PLAN.md` with async media handling, property matching, and basic observability.

## Invariants (Do Not Change)
- Supabase/Postgres is the system of record.
- One always-on daemon runs discovery + workers.
- No ProcessListing retries beyond next scheduled run.
- Media is async: store URL first, upload to S3 later.
- Dedup is via `property_matches` only; no merge logic yet.
- Drop/reseed is acceptable for migrations.
- Minimal framework: small modules only.

## File Layout
```
cmd/        scraper/       services/       storage/
workers/    models/        identity/       config/
```

## Phases (Execute in order)
1) **DB setup**: apply `schema_v2.sql` (media.s3_key nullable, status/attempts).
2) **Core structure**: add `services/` + `storage/` (pgx).
3) **Listing fan-out**: implement `ListingService.ProcessListing()` with idempotent writes.
4) **Enrichment worker**: parse listing details + photo URLs.
5) **Media worker**: download → hash → upload → update `media`.
6) **Healthcheck worker**: mark delisted + events.
7) **Cleanup**: remove old sync logic, optional TUI updates.

## Media Statuses
`pending`, `processing`, `uploaded`, `failed`.

## Tests
- JSON fixtures in `scraper/testdata/` for Realtor.ca parsing.
- Service-layer tests using a real Postgres test DB.
- Worker tests with `httptest` for Webshare and a fake S3 uploader.

## Do-Nots
- Don’t add a full framework.
- Don’t introduce a message queue yet.
- Don’t add merge logic for `property_matches`.
