#!/usr/bin/env bash
set -euo pipefail

echo "=== Installing Go analysis tools ==="

tools=(
  "honnef.co/go/tools/cmd/staticcheck@latest"
  "golang.org/x/vuln/cmd/govulncheck@latest"
  "github.com/fzipp/gocyclo/cmd/gocyclo@latest"
  "github.com/securego/gosec/v2/cmd/gosec@latest"
  "golang.org/x/tools/cmd/deadcode@latest"
)

for tool in "${tools[@]}"; do
  name=$(basename "$tool" | sed 's/@.*//')
  echo "  Installing $name..."
  go install "$tool"
done

echo ""
echo "=== Verifying ==="
for tool in staticcheck govulncheck gocyclo gosec deadcode; do
  if command -v "$tool" &>/dev/null; then
    echo "  ✓ $tool installed ($(command -v "$tool"))"
  else
    echo "  ✗ $tool not found in PATH"
  fi
done

echo ""
echo "Done. Run deadcode with: deadcode ./..."