---
title: 'mail_cli mbox merge'
parent: 'mail_cli mbox'
---

# mail\_cli mbox merge

Merge mbox file x into y (copies messages in x but not in y, appending to y).

## Usage

```
mail_cli mbox merge <x> <y>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<x>` | Source mbox file to merge from |
| `<y>` | Destination mbox file to merge into |

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

- `mail_cli mbox merge incoming.mbox archive.mbox`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
