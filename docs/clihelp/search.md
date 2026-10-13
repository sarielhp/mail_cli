---
title: 'mail_cli search'
---

# mail\_cli search

Search email history or export to mbox.

## Usage

```
mail_cli search <query> [flags]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<query>` | Query string (free text or search filters). |

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
| `-o, --output <file.mbox>` | Write matches to an mbox file |
| `-n, --limit <num>` | Maximum number of matches to retrieve (default: 50) |
| `--all` | Retrieve every match, however many there are |
| `-r, --reverse` | List newest first instead of oldest first |

## Examples

- `mail_cli search "taxes 2025"`
- `mail_cli search from:alice@example.com -n 25`
- `mail_cli -A work search "from:manager has:attachment"`
- `mail_cli search from:lawyer@firm.com --all -o legal.mbox` — Export every match to an archive

---

[↑ mail\_cli](index.md) — [nav](nav.md)
