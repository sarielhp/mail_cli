---
title: 'mail_cli config llm test'
parent: 'mail_cli config llm'
---

# mail\_cli config llm test

Send a harmless sample message to the configured chat endpoint and report whether the reply carries the urgency and TL;DR fields the summarizer parses. Use it to confirm an endpoint, model or reasoning-effort change before scanning real mail.

## Usage

```
mail_cli config llm test
```

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

- `mail_cli config llm test` — Check the endpoint, model and answer format

---

[↑ mail\_cli](index.md) — [nav](nav.md)
