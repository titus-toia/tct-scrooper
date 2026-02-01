#!/bin/bash
set -e

echo "Pulling latest on droplet and building..."
ssh scrooper-droplet 'cd /srv/tct_scrooper && git pull && /usr/local/go/bin/go build -o tct_scrooper . && cd tui && /usr/local/go/bin/go build -o ../tui_bin .'

echo "Restarting daemon..."
ssh scrooper-droplet 'sudo systemctl restart tct_scrooper'

echo "Done! Daemon status:"
ssh scrooper-droplet 'systemctl status tct_scrooper | head -5'
