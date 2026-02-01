#!/bin/bash
cd "$(dirname "${BASH_SOURCE[0]}")"
DB_PATH="${DB_PATH:-scraper.db}" LOG_PATH="${LOG_PATH:-daemon.log}" ./tui_bin "$@"
