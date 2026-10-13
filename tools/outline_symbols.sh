#!/usr/bin/env bash
set -euo pipefail

# Ensure we are in the repository root
cd "$(dirname "$0")/.."

echo "Index of Go Types and Functions:"
echo "=================================="
grep -nH -E '^(func|type) ' *.go | sort -t: -k1,1 -k2,2n
echo "=================================="
