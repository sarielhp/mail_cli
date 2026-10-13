---
title: 'mail_cli msg pipe'
parent: 'mail_cli msg'
---

# mail\_cli msg pipe

Fetch the specified message and pipe its contents (raw RFC822 by default, or body text with --body, or headers with --headers-only) into the standard input of an external command.

## Usage

```
mail_cli msg pipe [flags] <message-id> <command...>
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<message-id>` | Message ID or prefix hash of the email to pipe. |
| `<command...>` | Command and arguments to execute. |

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
| `--body` | Pipe only the extracted body text instead of raw RFC822. |
| `--headers-only` | Pipe only the RFC822 headers. |

## Examples

- `mail_cli msg pipe 18fb1234 git am` — Pipe email directly to git am
- `mail_cli msg pipe --body 18fb1234 -- wc -w` — Count words in message body
- `mail_cli msg pipe 18fb1234 'xclip -selection clipboard'` — Copy raw RFC822 email to system clipboard

---

[↑ mail\_cli](index.md) — [nav](nav.md)
