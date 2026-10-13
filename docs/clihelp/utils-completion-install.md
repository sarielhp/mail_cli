---
title: 'mail_cli utils completion install'
parent: 'mail_cli utils completion'
---

# mail\_cli utils completion install

Set up tab completion, the Alt-H key binding and the man page

## Usage

```
mail_cli utils completion install [--completion-no-keys] [--completion-no-man] [<shell>]
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
| `--completion-no-keys` | Install tab completion only, leaving Alt-H alone |
| `--completion-no-man` | Skip the manual page |

## Examples

- `mail_cli utils completion install` — Set up the active shell
- `mail_cli utils completion install --completion-no-keys zsh` — Set up Zsh completion without the Alt-H binding

## What It Writes

One generated file under this application's configuration directory, one permanent line in the shell's startup file that sources it, and a manual page under the user's data directory. The line never changes; the generated file is rewritten whenever the application is upgraded. On fish nothing shared is touched at all, because conf.d is a drop-in directory. Run 'completion uninstall' to remove all of it.

---

[↑ mail\_cli](index.md) — [nav](nav.md)
