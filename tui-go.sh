#!/bin/bash
# Run the TCT Scrooper Go TUI

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Build if needed
if [ ! -f "tui-go/tui-go" ] || [ "tui-go/main.go" -nt "tui-go/tui-go" ]; then
	echo "Building..."
	cd tui-go && go build -o tui-go . && cd ..
fi

# Run
DB_PATH="${DB_PATH:-scraper.db}" LOG_PATH="${LOG_PATH:-daemon.log}" ./tui-go/tui-go "$@"
