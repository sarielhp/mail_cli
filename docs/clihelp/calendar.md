---
title: 'mail_cli calendar'
has_children: true
---

# mail\_cli calendar

Parse iCalendar (.ics) attachments from emails and synchronize events with Google Calendar or inspect upcoming agenda schedules.

## Usage

```
mail_cli calendar <subcommand> [args...]
```

## Subcommands

| Command | Description |
|---------|-------------|
| [add](calendar-add.md) | Add event from email's .ics attachment. |
| [week](calendar-week.md) | Show upcoming calendar events. |
| [add-all (addall)](calendar-add-all.md) | Scan inbox and add all .ics events. |
| [sync](calendar-sync.md) | Scan inbox and sync all .ics events. |

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

- `mail_cli calendar week`
- `mail_cli calendar add 1234abcd`
- `mail_cli calendar add-all`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
