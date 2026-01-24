#!/bin/bash
# Setup script for TCT Scrooper TUI

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
VENV_DIR="$PROJECT_DIR/.venv"

echo "================================"
echo "TCT Scrooper TUI Setup"
echo "================================"
echo ""

# Check for python3
if ! command -v python3 &> /dev/null; then
	echo "Error: python3 not found"
	exit 1
fi

# Create venv if it doesn't exist
if [ ! -d "$VENV_DIR" ]; then
	echo "Creating virtual environment..."
	python3 -m venv "$VENV_DIR"
	echo "✓ Virtual environment created"
else
	echo "✓ Virtual environment exists"
fi

# Activate and install
echo "Installing dependencies..."
source "$VENV_DIR/bin/activate"
pip install --upgrade pip -q
pip install textual -q

echo "✓ Dependencies installed"

# Setup global commands
LOCAL_BIN="$HOME/.local/bin"
mkdir -p "$LOCAL_BIN"
ln -sf "$PROJECT_DIR/tui.sh" "$LOCAL_BIN/scrooper-top"
echo "✓ Installed 'scrooper-top' command (Python)"

# Build and install Go TUI if go is available
if command -v go &> /dev/null; then
	echo "Building Go TUI..."
	cd "$PROJECT_DIR/tui-go" && go build -o tui-go . && cd "$PROJECT_DIR"
	ln -sf "$PROJECT_DIR/tui-go.sh" "$LOCAL_BIN/scrooper-top-go"
	echo "✓ Installed 'scrooper-top-go' command (Go)"
else
	echo "⚠ Go not found, skipping Go TUI build"
fi

echo ""
echo "================================"
echo "Setup complete!"
echo "================================"
echo ""
echo "Run the TUI with:"
echo "  scrooper-top      (Python/Textual)"
echo "  scrooper-top-go   (Go/Bubble Tea)"
echo ""
