---
title: 'mail_cli utils cache repair'
parent: 'mail_cli utils cache'
---

# mail\_cli utils cache repair

Check the account's message index against the cached message files and rewrite it: one current-format record per message, a record for every file, and none for a missing file unless it holds your spam/not-spam decision, which a re-download then gets back. Damaged training ledgers are reported but never rewritten. Only local files are touched.

## Usage

```
mail_cli utils cache repair
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

- `mail_cli --dry-run utils cache repair` — Report what a repair would change
- `mail_cli utils cache repair`

## Reporting only

With --dry-run or --read-only it reports what it would change and changes nothing.

---

[↑ mail\_cli](index.md) — [nav](nav.md)
