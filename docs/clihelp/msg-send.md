---
title: 'mail_cli msg send'
parent: 'mail_cli msg'
---

# mail\_cli msg send

Send an email via configured account or standard input pipe, with optional attachments and Cc/Bcc recipients.

## Usage

```
mail_cli msg send [flags] <recipient...>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<recipient...>` | One or more recipient email addresses or contact aliases. |

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
| `-i, --include <path>` | Path to file containing email body. |
| `--body <text>` | Inline email body text. |
| `-t, --raw` | Read a full RFC822 message from stdin. |
| `-q, --queue` | Queue message in outbox without sending. |
| `--sign` | PGP-sign the message body. |
| `--encrypt` | PGP-encrypt; fails unless all recipients have keys. |
| `--pgp-key <id>` | Key to sign with (default: account key, else gpg's). |

## Examples

- `mail_cli msg send -s "Meeting notes" user@example.com < notes.txt` — Send email with body from stdin pipe
- `mail_cli msg send -s "Quick update" --body "Everything on track" boss@company.com` — Send quick email with inline body
- `mail_cli msg send -s "Report" -a report.pdf -c team@company.com boss < body.txt` — Send with attachment, CC, and alias
- `mail_cli msg send -s "Dry run test" --dry-run user@example.com < body.txt` — Preview RFC822 message without sending
- `mail_cli msg send --raw < patch.eml` — Send pre-formatted RFC822 message directly (git send-email mode)

---

[↑ mail\_cli](index.md) — [nav](nav.md)
