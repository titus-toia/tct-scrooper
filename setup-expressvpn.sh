#!/bin/bash

# ExpressVPN Setup Script
# Sets up ExpressVPN with activation key from user input
# Creates .env, downloads client, installs, and configures

set -e

echo "================================"
echo "ExpressVPN Setup Script"
echo "================================"
echo ""

# Check for proper execution context
# ExpressVPN doesn't work well with sudo - needs either:
# 1. Actual root user (logged in as root)
# 2. Normal user (will use group permissions)

if [[ -n "$SUDO_USER" ]]; then
   echo "❌ Error: Do not run this script with sudo!"
   echo ""
   echo "ExpressVPN requires either:"
   echo "  1. Actual root user:"
   echo "     su - && bash setup-expressvpn.sh"
   echo ""
   echo "  2. Normal user (recommended):"
   echo "     bash setup-expressvpn.sh"
   echo ""
   echo "Using 'sudo bash' causes permission issues with ExpressVPN daemon."
   exit 1
fi

if [[ $EUID -ne 0 ]] && ! groups | grep -q expressvpn; then
   echo "❌ Error: Must run as root OR be in expressvpn group"
   echo ""
   echo "Option 1 - Run as root:"
   echo "  su - && bash setup-expressvpn.sh"
   echo ""
   echo "Option 2 - Add yourself to expressvpn group (first install only):"
   echo "  sudo usermod -aG expressvpn \$USER"
   echo "  newgrp expressvpn"
   echo "  bash setup-expressvpn.sh"
   exit 1
fi

# Get activation code from user
echo "Enter your ExpressVPN activation code:"
read -s ACTIVATION_CODE
echo ""

if [ -z "$ACTIVATION_CODE" ]; then
    echo "Error: Activation code cannot be empty"
    exit 1
fi

# Create .env file
echo "Creating .env file..."
cat > .env << EOF
# ExpressVPN Configuration
EXPRESSVPN_ACTIVATION_CODE=$ACTIVATION_CODE
EXPRESSVPN_AUTOCONNECT=true
EXPRESSVPN_REGION=smart
EOF
chmod 600 .env
echo "✓ .env file created (permissions: 600)"

# Download ExpressVPN
echo ""
echo "Downloading ExpressVPN client..."
EXPRESSVPN_URL="https://www.expressvpn.works/clients/linux/expressvpn-linux-universal-5.0.1.11498.run"
EXPRESSVPN_FILE="expressvpn-linux-universal-5.0.1.11498.run"

if command -v curl &> /dev/null; then
    curl -O "$EXPRESSVPN_URL"
elif command -v wget &> /dev/null; then
    wget "$EXPRESSVPN_URL"
else
    echo "Error: curl or wget not found"
    exit 1
fi

if [ ! -f "$EXPRESSVPN_FILE" ]; then
    echo "Error: Failed to download ExpressVPN"
    exit 1
fi

echo "✓ Downloaded $EXPRESSVPN_FILE"

# Install ExpressVPN
echo ""
echo "Installing ExpressVPN..."
chmod +x "$EXPRESSVPN_FILE"
./"$EXPRESSVPN_FILE"

if [ $? -ne 0 ]; then
    echo "Error: ExpressVPN installation failed"
    exit 1
fi

echo "✓ ExpressVPN installed"

# Add to PATH if not already
echo ""
echo "Setting up PATH..."
if ! command -v expressvpnctl &> /dev/null; then
    ln -sf /opt/expressvpn/bin/expressvpnctl /usr/local/bin/expressvpnctl
    echo "✓ expressvpnctl linked to /usr/local/bin"
else
    echo "✓ expressvpnctl already in PATH"
fi

# Enable background mode
echo ""
echo "Enabling background mode..."
expressvpnctl background enable
echo "✓ Background mode enabled"

# Activate with key from .env
echo ""
echo "Activating ExpressVPN..."
source .env
echo "$EXPRESSVPN_ACTIVATION_CODE" > /tmp/activation.txt
expressvpnctl login /tmp/activation.txt
rm /tmp/activation.txt
echo "✓ ExpressVPN activated"

# Set autoconnect
echo ""
echo "Configuring autoconnect..."
expressvpnctl set autoconnect true
expressvpnctl set region "$EXPRESSVPN_REGION"
echo "✓ Autoconnect enabled (region: $EXPRESSVPN_REGION)"

# Connect to VPN
echo ""
echo "Connecting to VPN..."
expressvpnctl connect "$EXPRESSVPN_REGION"
sleep 3

# Check status
echo ""
echo "Checking connection..."
expressvpnctl status
echo ""
expressvpnctl get pubip
echo ""
expressvpnctl get vpnip

echo ""
echo "================================"
echo "✓ Setup Complete!"
echo "================================"
echo ""
echo "Your VPN is now:"
echo "  - Activated"
echo "  - Connected"
echo "  - Set to autoconnect on boot"
echo ""
echo "Useful commands:"
echo "  expressvpnctl status              # Check connection status"
echo "  expressvpnctl connect <region>    # Connect to specific region"
echo "  expressvpnctl disconnect          # Disconnect VPN"
echo "  expressvpnctl get regions         # List available regions"
echo ""
