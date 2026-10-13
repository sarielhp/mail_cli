---
title: 'mail_cli tui write'
parent: 'mail_cli tui'
---

# mail\_cli tui write

Write a message in your editor, then review.

## Usage

```
mail_cli tui write [flags] [recipient...]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `[recipient...]` | Optional recipient email addresses or contact aliases. |

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
| `-s, --subject <text>` | Email subject line. |
| `-c, --cc <address>` | Carbon copy (Cc) recipient address (comma-separated). |
| `-b, --bcc <address>` | Bcc recipient (comma-separated). |
| `-a, --attach <path>` | Attach files; repeat, comma-separate, or quote a glob. |
| `-i, --include <path>` | Path to file containing initial email body. |
| `--body <text>` | Inline initial email body text. |
| `-r, --resume <id>` | Reopen a saved draft by ID (see 'drafts ls'). |

## Examples

- `mail_cli tui write` — Open interactive editor to compose a new message
- `mail_cli tui write boss@company.com -s "Status Report"` — Compose email with pre-filled recipient and subject
- `mail_cli tui write -s "Patch review" -a fix.patch dev-list` — Compose with attachment and contact alias
- `mail_cli tui write -i notes.txt colleague@example.com` — Compose starting with body from file in editor
- `mail_cli tui write -a "*.pdf" -a notes.txt boss@company.com` — Attach every PDF here, quoted so mail_cli expands it, and one more file
- `mail_cli tui write --resume 2026/09/30/125` — Reopen a saved draft on the review screen

---

[↑ mail\_cli](index.md) — [nav](nav.md)
