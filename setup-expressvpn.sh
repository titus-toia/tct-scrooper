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

# Check if ExpressVPN is already installed
if command -v expressvpnctl &> /dev/null; then
    echo "✓ ExpressVPN is already installed"
    EXPRESSVPN_INSTALLED=true
else
    EXPRESSVPN_INSTALLED=false
fi

# Check if activation code is already in environment
if [ -n "$EXPRESSVPN_ACTIVATION_CODE" ]; then
    echo "✓ ExpressVPN activation code found in environment"
    echo "Do you want to:"
    echo "  1) Use environment variable (current code)"
    echo "  2) Enter a new activation code"
    read -p "Choose [1/2]: " env_var_choice

    if [ "$env_var_choice" = "2" ]; then
        echo "Enter your new ExpressVPN activation code:"
        read -s ACTIVATION_CODE
        echo ""

        if [ -z "$ACTIVATION_CODE" ]; then
            echo "Error: Activation code cannot be empty"
            exit 1
        fi
    else
        # Use the environment variable
        ACTIVATION_CODE=$EXPRESSVPN_ACTIVATION_CODE
    fi
elif [ -f .env ]; then
    # No env var, but .env file exists
    echo "⚠ .env file already exists (but no env var detected)"
    echo "Do you want to:"
    echo "  1) Use .env file (load existing code)"
    echo "  2) Update .env with new activation code"
    read -p "Choose [1/2]: " env_choice

    if [ "$env_choice" = "2" ]; then
        echo "Enter your ExpressVPN activation code:"
        read -s ACTIVATION_CODE
        echo ""

        if [ -z "$ACTIVATION_CODE" ]; then
            echo "Error: Activation code cannot be empty"
            exit 1
        fi

        # Backup old .env
        cp .env .env.backup
        cat > .env << EOF
# ExpressVPN Configuration
EXPRESSVPN_ACTIVATION_CODE=$ACTIVATION_CODE
EXPRESSVPN_AUTOCONNECT=true
EXPRESSVPN_REGION=smart
EOF
        chmod 600 .env
        echo "✓ .env file updated (backup saved to .env.backup)"
    else
        echo "Using existing .env"
    fi
else
    # No env var, no .env file
    echo "Enter your ExpressVPN activation code:"
    read -s ACTIVATION_CODE
    echo ""

    if [ -z "$ACTIVATION_CODE" ]; then
        echo "Error: Activation code cannot be empty"
        exit 1
    fi

    cat > .env << EOF
# ExpressVPN Configuration
EXPRESSVPN_ACTIVATION_CODE=$ACTIVATION_CODE
EXPRESSVPN_AUTOCONNECT=true
EXPRESSVPN_REGION=smart
EOF
    chmod 600 .env
    echo "✓ .env file created (permissions: 600)"
fi

# Ensure .env exists with the activation code
if [ ! -f .env ]; then
    cat > .env << EOF
# ExpressVPN Configuration
EXPRESSVPN_ACTIVATION_CODE=$ACTIVATION_CODE
EXPRESSVPN_AUTOCONNECT=true
EXPRESSVPN_REGION=smart
EOF
    chmod 600 .env
    echo "✓ .env file created with activation code"
fi

# Download & Install ExpressVPN (if not already installed)
if [ "$EXPRESSVPN_INSTALLED" = false ]; then
    echo ""
    echo "Downloading ExpressVPN client..."
    EXPRESSVPN_URL="https://www.expressvpn.works/clients/linux/expressvpn-linux-universal-5.0.1.11498.run"
    EXPRESSVPN_FILE="expressvpn-linux-universal-5.0.1.11498.run"

    # Skip download if file already exists
    if [ -f "$EXPRESSVPN_FILE" ]; then
        echo "✓ $EXPRESSVPN_FILE already downloaded"
    else
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
    fi

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
else
    echo "✓ ExpressVPN already installed, skipping download/install"
fi

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

# Load .env
source .env

# Check if already activated (try to get status)
echo ""
echo "Checking activation status..."
if expressvpnctl get connectionstate &> /dev/null; then
    echo "✓ ExpressVPN is already activated"
    ALREADY_ACTIVATED=true
else
    ALREADY_ACTIVATED=false
fi

# Activate with key from .env (if not already activated)
if [ "$ALREADY_ACTIVATED" = false ]; then
    echo "Activating ExpressVPN..."
    echo "$EXPRESSVPN_ACTIVATION_CODE" > /tmp/activation.txt
    expressvpnctl login /tmp/activation.txt
    rm /tmp/activation.txt
    echo "✓ ExpressVPN activated"
else
    echo "Skipping activation (already activated)"
fi

# Set autoconnect & region
echo ""
echo "Configuring settings..."
expressvpnctl set autoconnect true
expressvpnctl set region "$EXPRESSVPN_REGION"
echo "✓ Settings configured (autoconnect: enabled, region: $EXPRESSVPN_REGION)"

# Check current connection state
echo ""
echo "Current connection status:"
CONN_STATE=$(expressvpnctl get connectionstate 2>/dev/null || echo "unknown")
echo "  State: $CONN_STATE"

# Connect if not already connected
if [ "$CONN_STATE" != "Connected" ]; then
    echo ""
    read -p "Connect to VPN now? [y/n]: " connect_choice
    if [[ "$connect_choice" =~ ^[Yy]$ ]]; then
        echo "Connecting to VPN..."
        expressvpnctl connect "$EXPRESSVPN_REGION"
        sleep 3
        echo "✓ Connected"
    fi
else
    echo "✓ Already connected to VPN"
fi

# Show final status
echo ""
echo "Final status:"
expressvpnctl status 2>/dev/null || echo "  (unable to get status)"
echo ""
if [ "$CONN_STATE" = "Connected" ] || expressvpnctl get connectionstate 2>/dev/null | grep -q Connected; then
    echo "Public IP:"
    expressvpnctl get pubip 2>/dev/null || echo "  (unable to retrieve)"
    echo ""
    echo "VPN IP:"
    expressvpnctl get vpnip 2>/dev/null || echo "  (unable to retrieve)"
fi

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
