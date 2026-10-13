---
title: 'mail_cli utils completion keys'
parent: 'mail_cli utils completion'
---

# mail\_cli utils completion keys

Print shell key bindings (Alt-H explains the command line)

## Usage

```
mail_cli utils completion keys [<shell>]
```

## Parameters

| Parameter | Description |
|-----------|-------------|
| `[<shell>]` | Shell type ('bash', 'zsh', or 'fish'; defaults to current shell) |

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

- `mail_cli utils completion keys bash` — Print the Bash key bindings

## Why This Is Separate

Every shell loads a completion script lazily, on the first completion of the command, so a key binding written there would not exist until <Tab> had already been pressed once. Source this from your shell's rc file instead; the first line of the output says how.

---

[↑ mail\_cli](index.md) — [nav](nav.md)
