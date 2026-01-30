#!/bin/bash
set -e

echo "=== TCT Scrooper Droplet Setup ==="

# Config
APP_USER="${APP_USER:-titus}"
APP_DIR="/home/$APP_USER/projects/tct_scrooper"
GO_VERSION="1.23.0"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

log() { echo -e "${GREEN}[+]${NC} $1"; }
err() { echo -e "${RED}[!]${NC} $1"; exit 1; }

# Must run as root
[[ $EUID -ne 0 ]] && err "Run as root: sudo bash $0"

# Create user if needed
if ! id "$APP_USER" &>/dev/null; then
	log "Creating user $APP_USER..."
	useradd -m -s /bin/bash "$APP_USER"
fi

log "Updating package lists..."
apt-get update

if [[ "${ALSO_UPGRADE_DROPLET:-}" == "true" ]]; then
	log "Upgrading system packages..."
	apt-get upgrade -y
fi

log "Installing dependencies..."
apt-get install -y \
	build-essential \
	git \
	sqlite3 \
	postgresql-client \
	curl \
	wget \
	unzip \
	jq

# Install Go
if ! command -v go &>/dev/null || [[ "$(go version)" != *"$GO_VERSION"* ]]; then
	log "Installing Go $GO_VERSION..."
	cd /tmp
	wget -q "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz"
	rm -rf /usr/local/go
	tar -C /usr/local -xzf "go${GO_VERSION}.linux-amd64.tar.gz"
	rm "go${GO_VERSION}.linux-amd64.tar.gz"
fi

# Set up Go path for user
if ! grep -q 'export PATH=.*go/bin' /home/$APP_USER/.bashrc; then
	echo 'export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin' >> /home/$APP_USER/.bashrc
fi

log "Go version: $(/usr/local/go/bin/go version)"

# Clone or update repo
if [[ ! -d "$APP_DIR" ]]; then
	log "Cloning repository..."
	mkdir -p "$(dirname $APP_DIR)"
	git clone https://github.com/your-org/tct_scrooper.git "$APP_DIR"
	chown -R $APP_USER:$APP_USER "$(dirname $APP_DIR)"
else
	log "Repository exists, pulling latest..."
	cd "$APP_DIR"
	sudo -u $APP_USER git pull
fi

cd "$APP_DIR"

# Create .env if not exists
if [[ ! -f .env ]]; then
	log "Creating .env from example..."
	cp .env.example .env
	echo ""
	echo "!!! IMPORTANT: Edit $APP_DIR/.env with your credentials !!!"
	echo ""
fi

# Build Go binary
log "Building scrooper..."
sudo -u $APP_USER /usr/local/go/bin/go build -o tct_scrooper

# Build Go TUI
log "Building TUI..."
cd "$APP_DIR/tui"
sudo -u $APP_USER /usr/local/go/bin/go build -o tui

# Install systemd service
log "Installing systemd service..."
cat > /etc/systemd/system/tct_scrooper.service << EOF
[Unit]
Description=TCT Scrooper Daemon
After=network.target

[Service]
Type=simple
User=$APP_USER
WorkingDirectory=$APP_DIR
ExecStart=$APP_DIR/tct_scrooper
Restart=always
RestartSec=5
Environment=PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable tct_scrooper

# Restart if already running (to pick up new binary)
if systemctl is-active --quiet tct_scrooper; then
	log "Restarting service to pick up changes..."
	systemctl restart tct_scrooper
fi

# Create convenience scripts
log "Creating convenience scripts..."

cat > /usr/local/bin/scrooper << EOF
#!/bin/bash
case "\$1" in
  start|stop|restart)
    sudo systemctl \$1 tct_scrooper
    ;;
  status)
    systemctl is-active --quiet tct_scrooper && echo "running" || echo "stopped"
    ;;
  logs)
    journalctl -u tct_scrooper -f
    ;;
  tui|top)
    cd $APP_DIR && ./tui/tui
    ;;
  *)
    echo "TCT Scrooper - Property scraper daemon"
    echo ""
    echo "Usage: scrooper <command>"
    echo ""
    echo "Commands:"
    echo "  start    Start the daemon"
    echo "  stop     Stop the daemon"
    echo "  restart  Restart the daemon"
    echo "  status   Check daemon status"
    echo "  logs     Tail daemon logs (Ctrl+C to exit)"
    echo "  tui      Launch TUI"
    echo "  top      Launch TUI (alias)"
    ;;
esac
EOF
chmod +x /usr/local/bin/scrooper

echo ""
log "=== Setup Complete ==="
echo ""
echo "Run 'scrooper' for available commands."
echo ""
echo "Next steps:"
echo "  1. Edit $APP_DIR/.env with your credentials"
echo "  2. Run: scrooper start"
echo "  3. Run: scrooper logs"
echo ""
