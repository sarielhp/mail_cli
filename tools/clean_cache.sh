#!/usr/bin/env bash
set -euo pipefail

# Ensure we are in the repository root
cd "$(dirname "$0")/.."

# Check if mail_cli binary exists, compile if missing
if [ ! -f "./mail_cli" ]; then
    make
fi

echo "Cleaning local email cache using built-in cache command..."
./mail_cli cache prune --wipe
