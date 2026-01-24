#!/bin/bash
set -e

echo "=== TCT Scrooper Droplet Setup ==="

# Update system
apt-get update && apt-get upgrade -y

# Go (use latest stable)
GO_VERSION="1.22.4"
if ! command -v go &> /dev/null; then
	wget -q "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz"
	rm -rf /usr/local/go && tar -C /usr/local -xzf "go${GO_VERSION}.linux-amd64.tar.gz"
	rm "go${GO_VERSION}.linux-amd64.tar.gz"
	echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile
	export PATH=$PATH:/usr/local/go/bin
fi
go version

# Chromium + dependencies for headless browser (rod)
apt-get install -y \
	chromium-browser \
	libnss3 \
	libatk1.0-0 \
	libatk-bridge2.0-0 \
	libcups2 \
	libdrm2 \
	libxcomposite1 \
	libxdamage1 \
	libxrandr2 \
	libgbm1 \
	libasound2 \
	libpangocairo-1.0-0 \
	libgtk-3-0 \
	fonts-liberation \
	fonts-noto-color-emoji

# SQLite
apt-get install -y sqlite3 libsqlite3-dev

# Build tools (for cgo/sqlite)
apt-get install -y build-essential

# ExpressVPN (optional - download from expressvpn.com)
# wget https://www.expressvpn.works/clients/linux/expressvpn_3.x.x-1_amd64.deb
# dpkg -i expressvpn_*.deb
# expressvpn activate

echo "=== Setup complete ==="
echo "Next steps:"
echo "1. Clone repo and cd into it"
echo "2. Copy .env file with credentials"
echo "3. go build -o tct_scrooper"
echo "4. ./tct_scrooper"
