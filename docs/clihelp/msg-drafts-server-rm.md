---
title: 'mail_cli msg drafts server rm'
parent: 'mail_cli msg drafts server'
---

# mail\_cli msg drafts server rm

'all' moves every draft in the Drafts folder to the trash, unreadable ones included, and lists each one it moves. The trash keeps them until the server expires it, so nothing is lost at once; run with --dry-run to see the list first.

## Usage

```
mail_cli msg drafts server rm <id>... | all
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

---

[↑ mail\_cli](index.md) — [nav](nav.md)
