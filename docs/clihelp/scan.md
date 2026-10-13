---
title: 'mail_cli scan'
---

# mail\_cli scan

Scan all folders starting with the given label prefix (case-insensitive) for spam emails using Bogofilter.

## Usage

```
mail_cli scan <lbl_prefix> [flags]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<lbl_prefix>` | Label prefix to scan (e.g. inbox) |

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
| `-p, --pattern <pattern>` | Only messages whose subject has this |
| `-m, --move` | Move identified spam emails to the Spam folder |
| `--move-from <From>` | Move the lone message from this sender to Spam |
| `--inbox-move <From>` | Move mail from this sender back to Inbox |

## Examples

- `mail_cli scan inbox`
- `mail_cli scan inbox -m`
- `mail_cli scan receipts --move-from spammer@example.com`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
