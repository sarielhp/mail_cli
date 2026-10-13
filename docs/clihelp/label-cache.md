---
title: 'mail_cli label cache'
has_children: true
parent: 'mail_cli label'
---

# mail\_cli label cache

Manage the local labels cache: fetch and refresh cached label/folder data from the server.

## Usage

```
mail_cli label cache <subcommand>
```

## Subcommands

| Command | Description |
|---------|-------------|
| [update (up, sync)](label-cache-update.md) | Refresh local cache from server. |

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

- `mail_cli label cache update`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
