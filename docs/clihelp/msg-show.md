---
title: 'mail_cli msg show'
parent: 'mail_cli msg'
---

# mail\_cli msg show

Show emails by label prefix or message ID.

## Usage

```
mail_cli msg show <lbl_prefix|message-id> [message-id] [flags]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<lbl_prefix\|message-id>` | Folder prefix, or a message ID on its own |
| `[message-id]` | Message ID or prefix within the folder |

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
| `-w, --web` | Open the HTML body of the email in your configured browser |
| `--raw` | Output verbatim unformatted RFC822 message to stdout |

## Examples

- `mail_cli msg show receipts`
- `mail_cli msg show receipts 1234abcd`
- `mail_cli msg show receipts 1234abcd --web`
- `mail_cli msg show 1234abcd --raw` — Output raw RFC822 message for piping
- `mail_cli msg show 19f3a8216d52dfc6` — Show a message by ID alone, downloading it if needed

## JSON output

With --json a folder listing is one JSON array with an element per matched folder in every account -- {account, folder, message_ids, total} -- even when only one folder matches, so the output always parses. A message shown by ID is a single JSON object.

---

[↑ mail\_cli](index.md) — [nav](nav.md)
