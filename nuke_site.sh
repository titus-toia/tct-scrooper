#!/bin/bash
SITE=${1:-realtor_ca}
DB=${2:-scraper.db}

echo "Nuking $SITE from $DB..."
sqlite3 "$DB" "
  DELETE FROM listing_snapshots WHERE site_id = '$SITE';
  DELETE FROM scrape_runs WHERE site_id = '$SITE';
  DELETE FROM scrape_logs WHERE site_id = '$SITE';
  DELETE FROM properties WHERE id NOT IN (SELECT DISTINCT property_id FROM listing_snapshots);
  VACUUM;
"
echo "Done. Counts:"
sqlite3 "$DB" "SELECT 'properties:', COUNT(*) FROM properties; SELECT 'snapshots:', COUNT(*) FROM listing_snapshots;"
