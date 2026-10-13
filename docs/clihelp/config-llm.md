---
title: 'mail_cli config llm'
has_children: true
parent: 'mail_cli config'
---

# mail\_cli config llm

Test the configured LLM endpoint.

## Usage

```
mail_cli config llm <subcommand>
```

## Subcommands

| Command | Description |
|---------|-------------|
| [test (check)](config-llm-test.md) | Send a sample email to the configured LLM. |

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

- `mail_cli config llm test` — Send a harmless sample and check the answer

---

[↑ mail\_cli](index.md) — [nav](nav.md)
