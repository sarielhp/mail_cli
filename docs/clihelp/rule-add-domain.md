---
title: 'mail_cli rule add-domain'
parent: 'mail_cli rule'
---

# mail\_cli rule add-domain

Add an auto-labeling rule for a sender's domain from a cached message ID.

## Usage

```
mail_cli rule add-domain <message_id> [lbl]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<message_id>` | The message ID or short ID of the email. |
| `[lbl]` | Target label (default: message's folder). |

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

- `mail_cli rule add-domain 12345 %Sort/Newsletters`
- `mail_cli rule add-domain 12345`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
