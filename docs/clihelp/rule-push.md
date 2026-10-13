---
title: 'mail_cli rule push'
parent: 'mail_cli rule'
---

# mail\_cli rule push

Export local rules to mail server filters (e.g. Gmail filters or Sieve script).

## Usage

```
mail_cli rule push [force] [flags]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `[force]` | Overwrite existing remote filters. |

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
| `--sieve <file>` | Export rules as a Sieve script to file path |
| `-f, --force` | Force overwrite conflicting remote filters |

## Examples

- `mail_cli rule push`
- `mail_cli rule push --sieve ~/rules.sieve`

---

[↑ mail\_cli](index.md) — [nav](nav.md)
