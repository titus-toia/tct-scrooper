# ExpressVPN Setup & Deployment

## Quick Start (Local/WSL)

**IMPORTANT:** Do NOT use `sudo` with this script. ExpressVPN doesn't work properly with sudo.

### Option 1: As actual root user (easiest for first setup)

```bash
chmod +x setup-expressvpn.sh
su -
bash setup-expressvpn.sh
```

### Option 2: As normal user (recommended for ongoing use)

First time only - add yourself to expressvpn group:
```bash
sudo usermod -aG expressvpn $USER
newgrp expressvpn
```

Then run script:
```bash
chmod +x setup-expressvpn.sh
bash setup-expressvpn.sh
```

The script will:
1. ✓ Ask for your ExpressVPN activation code
2. ✓ Create `.env` file with your key
3. ✓ Download the ExpressVPN client
4. ✓ Install it
5. ✓ Activate with your key
6. ✓ Enable background mode
7. ✓ Set autoconnect
8. ✓ Connect to VPN

## Droplet Deployment

For a fresh Ubuntu/Debian droplet (logged in as root):

```bash
# SSH into droplet as root
ssh root@your-droplet-ip

# Clone/download your project
git clone <your-repo> /opt/scraper
cd /opt/scraper

# Run setup (as root, NOT with sudo)
bash setup-expressvpn.sh
```

Or create a droplet initialization script (run as root on fresh droplet):

```bash
#!/bin/bash
# deploy.sh - Run as root on fresh droplet

# Update system
apt update && apt upgrade -y

# Install dependencies
apt install -y curl wget git

# Clone project
git clone <your-repo> /opt/scraper
cd /opt/scraper

# Copy .env (you'll provide this securely via scp)
# Example: scp .env root@droplet:/opt/scraper/

# Run setup (as root, NOT with sudo)
bash setup-expressvpn.sh
```

## Manual Setup (if script fails)

Run as root (not with sudo):

```bash
# 1. Download
curl -O https://www.expressvpn.works/clients/linux/expressvpn-linux-universal-5.0.1.11498.run

# 2. Install (as root, not sudo)
bash expressvpn-linux-universal-5.0.1.11498.run

# 3. Activate
source .env
echo "$EXPRESSVPN_ACTIVATION_CODE" > /tmp/activation.txt
expressvpnctl login /tmp/activation.txt
rm /tmp/activation.txt

# 4. Enable background & autoconnect
expressvpnctl background enable
expressvpnctl set autoconnect true

# 5. Connect
expressvpnctl connect smart
```

## Verify Installation

```bash
# Check daemon is running
sudo systemctl status expressvpn-service

# Check connection status
expressvpnctl status

# View public IP (should be VPN IP)
expressvpnctl get pubip

# View VPN IP
expressvpnctl get vpnip
```

## Useful Commands

```bash
# Check regions
expressvpnctl get regions

# Connect to specific region
expressvpnctl connect "usa-new-york"

# Disconnect
expressvpnctl disconnect

# Disable autoconnect
expressvpnctl set autoconnect false

# Monitor connection state
expressvpnctl monitor connectionstate
```

## Troubleshooting

**"expressvpnctl: command not found"**
```bash
ln -s /opt/expressvpn/bin/expressvpnctl /usr/local/bin/expressvpnctl
```

**"Background mode not enabled"**
```bash
sudo expressvpnctl background enable
```

**Connection drops**
```bash
# Check daemon status
sudo systemctl restart expressvpn-service

# Reconnect
expressvpnctl connect smart
```

## Security Notes

- `.env` contains your activation code - keep it secure!
- Set permissions: `chmod 600 .env`
- Don't commit `.env` to git (add to `.gitignore`)
- Use `.env.example` as template
- On droplet, only root needs access to `.env`

## Next Steps

Once ExpressVPN is running:
1. Build the scraper (Go application)
2. Build the TUI admin interface (Textual/Python)
3. Set up Supabase for data storage
4. Configure cron/systemd for scheduled jobs
