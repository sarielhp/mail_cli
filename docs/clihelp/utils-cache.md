---
title: 'mail_cli utils cache'
has_children: true
parent: 'mail_cli utils'
---

# mail\_cli utils cache

Manage the local email download cache.

## Usage

```
mail_cli utils cache <subcommand> [args...]
```

## Subcommands

| Command | Description |
|---------|-------------|
| [prune](utils-cache-prune.md) | Prune cached emails older than N days. |
| [clear (reset)](utils-cache-clear.md) | Recreate cache directory and wipe data. |
| [repair](utils-cache-repair.md) | Check and rewrite this account's index. |

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

- `mail_cli utils cache prune`
- `mail_cli utils cache prune 7`
- `mail_cli utils cache prune --wipe`
- `mail_cli utils cache clear`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
