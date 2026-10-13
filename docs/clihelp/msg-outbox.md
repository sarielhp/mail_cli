---
title: 'mail_cli msg outbox'
has_children: true
parent: 'mail_cli msg'
---

# mail\_cli msg outbox

Inspect, transmit, or delete messages spooled to the local outbox when offline or queued via msg send --queue.

## Usage

```
mail_cli msg outbox <ls | flush | rm <id>...>
```

## Subcommands

| Command | Description |
|---------|-------------|
| [ls (list)](msg-outbox-ls.md) | List queued messages in outbox. |
| [flush](msg-outbox-flush.md) | Transmit all queued messages in outbox. |
| [rm](msg-outbox-rm.md) | Delete queued messages from outbox. |

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

- `mail_cli msg outbox ls` — List all spooled messages
- `mail_cli msg outbox flush` — Transmit all queued messages
- `mail_cli msg outbox rm 20261011-134500-0001` — Delete a queued message

---

[↑ mail\_cli](index.md) — [nav](nav.md)
