---
title: 'mail_cli msg reply'
parent: 'mail_cli msg'
---

# mail\_cli msg reply

Reply to an email with auto-quoting, Re: subject, and threading headers.

## Usage

```
mail_cli msg reply [flags] <message-id>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<message-id>` | Message ID or prefix hash of the email to reply to. |

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
| `-g, --group` | Group reply (reply to all recipients). |
| `-s, --subject <text>` | Override reply subject line. |
| `-c, --cc <address>` | Additional Cc recipient address (comma-separated). |
| `-b, --bcc <address>` | Bcc recipient (comma-separated). |
| `-a, --attach <path>` | Attachment file path (comma-separated for multiples). |
| `-i, --include <path>` | Path to file containing reply body. |
| `--body <text>` | Inline reply body text. |
| `--no-quote` | Omit quoted text from original email. |
| `--headless` | Send directly without interactive editor or TUI review. |

## Examples

- `mail_cli msg reply 18fb1234abcd` — Reply interactively to message in editor and review screen
- `mail_cli msg reply -g 18fb1234abcd` — Group reply (reply-all) interactively to all recipients
- `mail_cli msg reply --headless --body "Thanks, received!" 18fb1234abcd` — Send reply non-interactively with inline body
- `mail_cli msg reply --headless -i reply.txt 18fb1234abcd` — Send reply non-interactively reading body from file
- `mail_cli msg reply --headless 18fb1234abcd < reply.txt` — Send reply non-interactively reading body from stdin pipe
- `mail_cli msg reply --no-quote -s "New topic" 18fb1234abcd` — Reply without quoting original message with custom subject

---

[↑ mail\_cli](index.md) — [nav](nav.md)
