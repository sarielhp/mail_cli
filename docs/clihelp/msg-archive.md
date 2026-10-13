---
title: 'mail_cli msg archive'
parent: 'mail_cli msg'
---

# mail\_cli msg archive

Archive one or more messages by ID.

## Usage

```
mail_cli msg archive <message-id...>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<message-id...>` | One or more message IDs (short 8-character or full). |

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

- `mail_cli msg archive abc123de` — Archive one message
- `mail_cli msg archive abc123de 9f8e7d6c` — Archive several at once

---

[↑ mail\_cli](index.md) — [nav](nav.md)
