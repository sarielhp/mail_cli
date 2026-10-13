---
title: 'mail_cli contacts'
has_children: true
---

# mail\_cli contacts

View and manage email contacts and aliases from Google and Mutt.

## Usage

```
mail_cli contacts [subcommand]
```

## Subcommands

| Command | Description |
|---------|-------------|
| [list](contacts-list.md) | List stored contacts and aliases. |
| [search](contacts-search.md) | Search contacts by name, email, or note. |
| [suggest](contacts-suggest.md) | Suggest ranked recipients from history. |
| [harvest](contacts-harvest.md) | Build suggestion index from cached mail. |
| [import-mutt](contacts-import-mutt.md) | Import contacts from a Mutt alias file. |
| [sync](contacts-sync.md) | Sync contacts from Google People API. |
| [path](contacts-path.md) | Print the file path to local contacts store. |

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

- `mail_cli contacts` — List all contacts and aliases
- `mail_cli contacts search algo` — Search contacts and groups
- `mail_cli contacts suggest alice` — Suggest ranked email recipients
- `mail_cli contacts harvest` — Harvest suggested recipients from cached mail
- `mail_cli contacts sync` — Sync contacts from Google Contacts
- `mail_cli contacts import-mutt` — Import Mutt aliases configured in config.json
- `mail_cli contacts import-mutt ~/.mutt/aliases` — Import Mutt aliases from a specific file

---

[↑ mail\_cli](index.md) — [nav](nav.md)
