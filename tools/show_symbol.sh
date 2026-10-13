#!/usr/bin/env bash
set -euo pipefail

# Ensure we are in the repository root
cd "$(dirname "$0")/.."

if [ $# -lt 1 ]; then
    echo "Usage: $0 <symbol_name>" >&2
    exit 1
fi

SYMBOL="$1"

# Setup excludes for ctags
EXCLUDE_ARGS=""
if [ -d "old_code" ]; then
    EXCLUDE_ARGS="$EXCLUDE_ARGS --exclude=old_code"
fi
if [ -d ".aider.tags.cache.v4" ]; then
    EXCLUDE_ARGS="$EXCLUDE_ARGS --exclude=.aider.tags.cache.v4"
fi

# Run ctags and query for matching symbol name using jq
MATCHES=$(ctags $EXCLUDE_ARGS --fields=+ne --output-format=json -R . | jq -c "select(.name == \"$SYMBOL\")")

if [ -z "$MATCHES" ]; then
    echo "Symbol '$SYMBOL' not found."
    exit 0
fi

# Iterate over matches
while IFS= read -r match; do
    file=$(echo "$match" | jq -r '.path')
    start=$(echo "$match" | jq -r '.line')
    end=$(echo "$match" | jq -r '.end // empty')
    kind=$(echo "$match" | jq -r '.kind')
    scope=$(echo "$match" | jq -r '.scope // ""')
    
    # If end is empty, default to start line
    if [ -z "$end" ]; then
        end=$start
    fi
    
    # If it is a single-line type/var/const but we want some context, let's output a single line
    # For multiline blocks, we output the full range
    echo "================================================================================"
    if [ -n "$scope" ]; then
        echo "File: $file | Symbol: $SYMBOL ($kind in $scope) | Lines: $start-$end"
    else
        echo "File: $file | Symbol: $SYMBOL ($kind) | Lines: $start-$end"
    fi
    echo "--------------------------------------------------------------------------------"
    
    # Print the lines of code using head and tail
    head -n "$end" "$file" | tail -n "+$start"
    echo ""
done <<< "$MATCHES"
