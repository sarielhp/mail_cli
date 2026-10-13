---
title: 'mail_cli mbox view'
parent: 'mail_cli mbox'
---

# mail\_cli mbox view

Browse a local mbox or .eml file in the TUI.

## Usage

```
mail_cli mbox view <file>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<file>` | A local mbox file, or a single message ending in .eml |

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

- `mail_cli mbox view archive.mbox`
- `mail_cli mbox view ~/mail/backup.mbox`
- `mail_cli mbox view message.eml` — Open one saved message

---

[↑ mail\_cli](index.md) — [nav](nav.md)
