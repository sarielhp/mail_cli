---
title: 'mail_cli account add'
parent: 'mail_cli account'
---

# mail\_cli account add

Prompts for whatever the chosen backend needs and writes the entry to config.json. With --interactive the prompts follow the same guided flow as `account setup`: provider-specific tips, the Gmail credentials.json copy, and a connectivity check before the command returns.

## Usage

```
mail_cli account add <jmap|gmail|outlook> [name] [flags]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `<jmap\|gmail\|outlook>` | Account backend provider type. |
| `[name]` | Unique name for the account. |

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
| `--type <type>` | Account type: regular or test (default: regular) |
| `--test` | Mark as test account (shortcut for --type test) |
| `--interactive` | Guide setup with provider tips and a connection check |

## Examples

- `mail_cli account add gmail personal-gmail`
- `mail_cli account add jmap work-fastmail`
- `mail_cli account add outlook uni-outlook --test`
- `mail_cli account add gmail personal-gmail --interactive` — Guided setup with a connection check

---

[↑ mail\_cli](index.md) — [nav](nav.md)
