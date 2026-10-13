---
title: 'mail_cli rule add'
parent: 'mail_cli rule'
---

# mail\_cli rule add

Add a server filter rule to auto-label emails from a sender or from a cached message ID.

## Usage

```
mail_cli rule add <email|message_id> [lbl]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<email\|message_id>` | The sender email address or cached message ID. |
| `[lbl]` | Target label (optional with message_id). |

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

- `mail_cli rule add notifications@github.com %Dev/GitHub`
- `mail_cli rule add 12345 %Dev/GitHub`
- `mail_cli rule add 12345`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
