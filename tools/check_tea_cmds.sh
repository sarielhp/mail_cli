#!/usr/bin/env bash
# Flag model field access inside a Bubble Tea command closure.
#
# Bubble Tea runs every tea.Cmd on its own goroutine, concurrently with the next
# Update call. Touching tuiModel fields inside `return func() tea.Msg { ... }`
# therefore races the Update goroutine. Writing a field can abort the process
# with a non-recoverable "concurrent map writes" fatal error; reading one picks
# up whatever the value happened to be when the command ran, which is how an
# archive could be sent with the wrong source label.
#
# Commands must take what they need as parameters, resolved on the Update
# goroutine, and return results as a tea.Msg.
#
# A small allowlist covers fields set once at construction and never reassigned.
set -euo pipefail
cd "$(dirname "$0")/.."

python3 - "$@" <<'PY'
import glob, re, sys

ALLOW = {"client", "bk", "cfg", "theme", "isReadOnly", "Config"}
OPEN = re.compile(r'return func\(\) tea\.Msg \{')
FIELD = re.compile(r'\bm\.([A-Za-z_]\w*)')

problems = []
for path in sorted(glob.glob("tui/*.go")):
    if path.endswith("_test.go"):
        continue
    lines = open(path).read().split("\n")
    depth = None
    for i, line in enumerate(lines, 1):
        if depth is None:
            if OPEN.search(line):
                depth = line.count("{") - line.count("}")
            continue
        for name in FIELD.findall(line):
            if name not in ALLOW:
                problems.append((path, i, name, line.strip()))
        depth += line.count("{") - line.count("}")
        if depth <= 0:
            depth = None

if problems:
    print("Model field access inside tea.Cmd closures:")
    print("-" * 60)
    for path, line, name, text in problems:
        print(f"\033[31m[!] {path}:{line}: m.{name}\033[0m")
        print(f"        {text}")
    print("-" * 60)
    print(f"{len(problems)} occurrence(s). Pass the value in as a parameter instead.")
    sys.exit(1)

print("No model field access inside tea.Cmd closures.")
PY
