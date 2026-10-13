---
title: 'mail_cli mbox'
has_children: true
---

# mail\_cli mbox

Inspect, validate and merge local mbox files.

## Usage

```
mail_cli mbox <subcommand> [args...]
```

## Subcommands

| Command | Description |
|---------|-------------|
| [check](mbox-check.md) | Validate mbox file or directory. |
| [merge](mbox-merge.md) | Merge mbox file x into y. |
| [info](mbox-info.md) | Display message count and file size. |
| [view](mbox-view.md) | Browse a local mbox or .eml file in the TUI. |

## Flags

| Flag | Description |
|------|-------------|
| `-A, --account <name>` | Target account name |
| `--account-index N` | Target account by config index (also -N) |
| `--read-only` | Block server writes (cache still updates) |
| `--dry-run` | Report changes only; implies --read-only |
| `-v, --verbose` | Enable verbose output |
| `--json` | Emit result as JSON on stdout |
| `--logappend` | Append to existing logs instead of truncating |
| `--version` | Print the version number and exit |

## Examples

- `mail_cli mbox check archive.mbox` — Validate a single mbox file
- `mail_cli mbox check ~/archive/` — Validate all mbox files in a directory
- `mail_cli mbox merge new.mbox archive.mbox` — Merge new.mbox into archive.mbox deduplicating messages
- `mail_cli mbox info archive.mbox` — Show message count, file size, and attachment statistics
- `mail_cli mbox view archive.mbox` — Open and browse an mbox file in the TUI (read-only)

---

[↑ mail\_cli](index.md) — [nav](nav.md)
