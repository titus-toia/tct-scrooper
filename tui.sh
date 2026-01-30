#!/bin/bash
# Run the TCT Scrooper Go TUI

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Build if needed
if [ ! -f "tui/tui" ] || [ "tui/main.go" -nt "tui/tui" ]; then
	echo "Building..."
	cd tui && go build -o tui . && cd ..
fi

# Run
DB_PATH="${DB_PATH:-scraper.db}" LOG_PATH="${LOG_PATH:-daemon.log}" ./tui/tui "$@"
