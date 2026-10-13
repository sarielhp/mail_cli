---
title: 'mail_cli account'
has_children: true
---

# mail\_cli account

Start with account setup for guided onboarding, or account new to add an account explicitly. Use account login to renew credentials and account migrate to copy configuration to another machine over SSH.

## Usage

```
mail_cli account <subcommand> [args...]
```

## Subcommands

| Command | Description |
|---------|-------------|
| [list](account-list.md) | List configured accounts. |
| [add (new)](account-add.md) | Add a new account template to config.json. |
| [link (associate)](account-link.md) | Link program name for auto-selection. |
| [test](account-test.md) | Check an account's config and connection. |
| [calendar (cal)](account-calendar.md) | Show or set the calendar manager account. |
| [login](account-login.md) | OAuth login for Gmail or Outlook. |
| [rename](account-rename.md) | Rename an existing account's display name. |
| [user-name](account-user-name.md) | Set the name shown in outgoing mail. |
| [user-email](account-user-email.md) | Set the address outgoing mail is sent from. |
| [rm (delete, del)](account-rm.md) | Delete an account and its credentials. |
| [setup](account-setup.md) | Configure an email account interactively. |
| [migrate](account-migrate.md) | Copy config and credentials via SSH. |

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

- `mail_cli account list`
- `mail_cli account new personal-gmail`
- `mail_cli account test`
- `mail_cli account rename old-gmail new-gmail`
- `mail_cli account rm temp-account`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
