#!/bin/bash
# Run the TCT Scrooper TUI

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VENV_DIR="$SCRIPT_DIR/.venv"

# Check if venv exists
if [ ! -d "$VENV_DIR" ]; then
	echo "Virtual environment not found. Running setup..."
	bash "$SCRIPT_DIR/tui/setup.sh"
fi

# Activate and run
source "$VENV_DIR/bin/activate"
cd "$SCRIPT_DIR"
python run_tui.py "$@"
