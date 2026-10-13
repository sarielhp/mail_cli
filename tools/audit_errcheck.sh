#!/usr/bin/env bash
set -euo pipefail

# Ensure we are in the repository root
cd "$(dirname "$0")/.."

if ! command -v errcheck &> /dev/null; then
    echo "errcheck is not installed. Install with: go install github.com/kisielk/errcheck@latest"
    exit 1
fi

echo "Running errcheck across codebase..."
if [ -f "tools/errcheck_excludes.txt" ]; then
    errcheck -exclude tools/errcheck_excludes.txt ./...
else
    errcheck ./...
fi

echo "errcheck passed: No unhandled errors found."
