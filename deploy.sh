#!/bin/bash
set -e

echo "Building binaries..."
go build -o tct_scrooper .
cd tui && go build -o tui_bin . && cd ..

echo "Deploying to droplet..."
scp tct_scrooper scrooper-droplet:/srv/tct_scrooper/tct_scrooper
scp tui/tui_bin scrooper-droplet:/srv/tct_scrooper/tui_bin

echo "Restarting daemon..."
ssh scrooper-droplet 'sudo systemctl restart tct_scrooper'

echo "Updating git on droplet..."
ssh scrooper-droplet 'cd /srv/tct_scrooper && git pull'

echo "Done! Daemon status:"
ssh scrooper-droplet 'systemctl status tct_scrooper | head -5'
