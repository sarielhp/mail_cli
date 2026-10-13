#!/usr/bin/env bash
set -euo pipefail

# Generate the bash/zsh/fish tab-completion scripts into completions/ so that a
# package recipe can install them (contrib/aur/PKGBUILD builds them this way;
# GoReleaser used to ship them in its archives, which is gone now that releases
# are source-only) -- they are gitignored, so generate them rather than commit
# them.
#
# The scripts are produced by the binary itself (`completion <shell>`), so the
# generation needs a build. It is a host build on purpose: a cross-compiled
# target binary in dist/ cannot be executed here, and a release builds darwin
# and arm64 targets alongside linux/amd64.

cd "$(dirname "$0")/.."

out="completions"
mkdir -p "$out"

# Build to a scratch directory so the generator binary never lands in the repo
# root (where it would collide with the `mail_cli` entry in .gitignore) and is
# removed on every exit path.
scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT
bin="$scratch/mail_cli"

go build -o "$bin" .

"$bin" completion bash > "$out/mail_cli.bash"
"$bin" completion zsh  > "$out/_mail_cli"
"$bin" completion fish > "$out/mail_cli.fish"

echo "wrote shell completions to $out/"
