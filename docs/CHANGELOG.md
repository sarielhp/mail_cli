# 📝 Changelog

All notable changes to `mail_cli` are documented in this file.

> **This file was not maintained between 0.2.20 and 0.7.0.** The entries below
> resume it. The gap is real and is not reconstructed here; `git log` is the
> record for that stretch.

## [0.7.0] - 2026-09-19

The largest release so far: a layering rewrite, a redesigned command tree, and
the whole backlog of a deep code review. 83 commits.

Read the **Breaking changes** section before upgrading. Most of it is renames
with a mechanical fix, but labels now carry a sigil, which touches nearly every
command line.

### Breaking changes

**Labels are written `%label`.** A bare folder name is no longer accepted; the
error names the spelling that would have worked.

```
scan inbox              ->  scan %inbox
show receipts 1234abcd  ->  show %receipts 1234abcd
label sort work:old     ->  label sort %work:old
```

`%` on its own means the inbox, so `mail_cli %` scans it and `mail_cli %inbox`
is the same thing. `%%foo` is the label literally named `%foo`. The sigil exists
because without it the tool has to guess whether an argument is a folder, a
message ID or an address — and guessing is what it used to do.

**Commands moved under the noun they belong to.** The old top-level spellings
are gone, not deprecated:

| was | now |
|---|---|
| `splice` | `label sort` |
| `split` | `label move` |
| `download` | `label export` |
| `upload` | `label import` |
| `unspam` | `spam unmark` |
| `whitelist` | `spam allow` |
| `blacklist` | `spam block` |
| `learning reset` | `spam reset` |
| `learn-ham` | `spam learn --ham` |
| `filter` | `rule remote` |
| `caladd` | `calendar sync` |
| `test` | `doctor` |
| `label list full ext real` | `label list --full --ext --real` |

**Kept, because they are in everyone's fingers:** `labels`, `arc`, `archive`,
`ss`, `sb`, `wlist`, `blist`, `del`, `delete`, `fix`, `ls`, and the `-1` / `-2`
account-index flags all still work.

**Flags.**

- `--move=<addr>` is now `--move-from <addr>`. `-m` / `--move` is a plain
  boolean. One string flag used to carry both a boolean and an address, with the
  absent value recorded as the literal `"true"`.
- `--ro` is removed. Use `--read-only`.
- `-p` / `--pattern` and `--fix` are no longer accepted by every command. They
  are declared by the three commands that read the pattern (`scan`, `label sort`
  and the bare `%label` form) and by `config check --fix`.
- `config set` takes exactly two arguments. The third selected an
  account-specific override and was read nowhere; use `-A` to choose the account.

**Semantics.**

- `--dry-run` and `--read-only` are different contracts and both are documented
  as such. `--read-only` forbids server mutations but still caches, because
  without downloaded bodies a message cannot be shown at all; `--dry-run` also
  forbids the local config and cache writes. `--dry-run` implies `--read-only`.
- `label move`'s wildcard-free patterns are substring matches, not exact ones —
  the same rule `label sort` and `scan` already used. `label move src invoice
  dst` previously matched only a subject *equal* to "invoice", so it silently
  moved nothing.
- `doctor` is refused under `--read-only` and `--dry-run`. It creates a label,
  inserts a message, moves it and deletes both — that is how it proves the
  credentials can write.
- Irreversible commands prompt, and require `--yes` when stdin is not a
  terminal: `spam empty`, `label rm`, `rule rm-all`, `account rm`, `cache clear`.
  A cron line that would have prompted fails loudly rather than guessing.

### Added

- **`%` label references**, with `%account:label` for cross-account work.
- **Tab completion for label arguments**, not just flag values, on fourteen
  positionals. With nothing typed it offers the `%` form.
- **`-E` / `--examples`** shows every example in the program on one page; add a
  command to narrow it.
- **`--json`** on the listings, for scripts.
- **`spam reset`, `label repair`, `rule tidy`, `msg archive`, `label archive`**
  as first-class commands.
- **`report.Reporter`**, an output port: status goes to it, a command's product
  goes to an `io.Writer` the caller supplies. Suppressing progress no longer
  suppresses the answer.
- **A generated package inventory** in `docs/ARCHITECTURE.md`, with a drift test.
- **An architecture check** in the quality gate, enforcing four layering rules.

### Fixed

**Correctness**

- **Ambiguous label prefixes resolved to a string naming no folder.** With
  folders `Work` and `Worklog`, `scan wor` passed `"wor"` to the backend: Gmail
  matched nothing and did nothing, JMAP *created a new empty mailbox* called
  `wor` and filed messages into it. There is one resolver now and it refuses to
  guess.
- **Bulk spam operations processed one page and reported success.** JMAP queried
  1000 messages with no loop; Outlook took `$top=100` and never followed
  `@odata.nextLink`. Both now converge.
- **A partly-failed batch move discarded the part that succeeded**, leaving
  messages listed in a folder they had left.
- **`--dry-run` written before the command was silently downgraded** to
  `--read-only`, so the local writes it exists to prevent went ahead.
- **A cache store that writes nothing silenced the network.** Reaching a
  non-selected account showed every folder as empty and reported success.
- **One overlong line discarded an entire mbox import.**
- **`scan %inbox` stopped clearing cached classifications**, because the check
  compared the raw argument against `"inbox"`.
- **`label rm` deleted a server label with no confirmation**, the one row of the
  confirmation matrix that had never been implemented. It also passed its
  argument to the backend unparsed, so the `%` form named a label that does not
  exist while the bare form deleted one without being checked at all.

**Security and robustness**

- **`sieve` export could be broken out of.** A rule whose sender, subject or
  label contained a newline injected script into a file the user is told to
  upload to their mail server.
- **`StripHTML` leaked the contents of an unclosed `<script>` or `<style>`** into
  the reading pane and into the text the spam classifier scores. Numeric
  character references are decoded now, so `&#118;&#105;agra` no longer hides a
  keyword from the classifier.
- **MIME nesting is depth-capped** in the two attachment walks, which had no
  limit and run automatically on every message opened or scanned.
- **`doctor` mutated the account under `--read-only`** — it builds its own
  client, so the read-only decorator never saw it.
- **A closed stdin was treated as consent to every default**, so
  `account new jmap work < /dev/null` stored the literal placeholder
  `your-jmap-api-token` as the account password.
- **Every piped answer after the first was discarded** during `account new`,
  because each prompt built its own buffered reader.

**Interface**

- **The TUI's spam key never trained the classifier**, so the action a user
  repeats most often taught it nothing while `spam mark` taught it everything.
- **Attachment previews were unbounded**: one external process per attachment,
  none cancelled when the user navigated away, and no redraw when they finished.
- **`-A nonesuch` ran against the first configured account** and exited 0. Both
  the unknown and the ambiguous case are errors now.
- **`rule list` deleted duplicate rules and saved the config** — a read command
  with a write side effect. That is `rule tidy` now.
- **A refreshed Gmail OAuth token was discarded**, so every process re-derived
  one against a provider that rate-limits refreshes.
- **An Outlook account with a lapsed token started a browser sign-in from any
  process**, including a cron scan, and then waited up to fifteen minutes for
  someone to type a code. It refuses without a terminal now and names
  `account login`.
- **An unresolvable calendar `TZID` silently read the time as UTC.** It is
  reported now, because that instant is what an event is filed at.

### Changed

- **The layering is explicit and enforced.** `main` composes; `cli` and `tui`
  declare the interfaces they need; `runner` implements them; `mailops` is the
  only package that may name a backend. 454 `fmt.Print*` calls below the UI are
  gone, along with the `app.TuiActive` global that existed to mute them.
- **`docs/ARCHITECTURE.md` is rewritten.** It described a flat `package main`
  layout and instructed the reader never to create subdirectories, against a
  tree of forty-one packages.
- Every package has a doc comment.

## [0.2.20] - 2026-06-22

### Changed
* **TUI Message Detail View:** Redesigned the detail view when pressing Enter on an email: the top bar is now always shown, the second line displays the selected index row with highlighting, headers and message body use natural text coloring (yellow labels with default foreground body on dark background), and a special `< End of message >` line with green background appears at the bottom.

## [0.2.19] - 2026-06-06

### Fixed
* **JMAP Filter Compilation Error:** Implemented the missing `Requires() []jmap.URI` method on the custom `JMAPFilterSetMethod` struct to satisfy the `jmap.Method` interface.

## [0.2.18] - 2026-06-06

### Added
* **JMAP Server-Side Rule Migration:** Implemented server-side rule migration for JMAP accounts using the JMAP Filters extension (`Filter/set` method). This uploads local routing rules to the JMAP server so they apply automatically to incoming emails.

## [0.2.17] - 2026-06-06

### Added
* **JMAP Rule Export Persistence:** Enhanced `ExportRules` for JMAP accounts to persist the `Exported` status of rules back to `config.json` and update the in-memory configuration.

## [0.2.16] - 2026-06-06

### Added
* **JMAP Rule Export Support:** Implemented `ExportRules` for JMAP accounts. Since JMAP rules are processed locally by `mail_cli` during scans, they do not need to be uploaded to the server. The command now marks local rules as exported and prints a success message instead of failing.

## [0.2.15] - 2026-06-06

### Fixed
* **Cobra Optional Flag Argument Parsing:** Added logic to `preprocessArgs` to automatically rewrite `-m <argument>` / `--move <argument>` to use the assignment operator (`-m=<argument>` / `--move=<argument>`), preventing Cobra from interpreting the flag value as a positional argument when `NoOptDefVal` is active.

## [0.2.10] - 2026-06-06

### Added
* **Unique Message Move-to-Spam:** Enabled specifying a sender email address directly to `-m` / `--move` flag (e.g. `-m spammer@example.com`). If exactly one message from this sender is found in the scanned folder, it is moved to the spam folder and removed from the remainder of the scan.

## [0.2.9] - 2026-06-06

### Added
* **Version Command:** Added an explicit `version` command to the CLI that prints the active program version.

## [0.2.8] - 2026-06-06

### Fixed
* **ProgName Auto-Selection:** Removed the legacy exclusion of `"gmail"` from the binary name/symlink auto-selection check in `config.go`, enabling correct auto-selection of the associated account when the tool is executed via a `gmail` symbolic link.

## [0.2.7] - 2026-06-06

### Added
* **Command display in account list:** Modified the `account list` command output to display the associated CLI command/alias for each account on a new line.

## [0.2.6] - 2026-06-06

### Fixed
* **JMAP Spam Learning Responses:** Updated `LearnSpam` in `jmap_client.go` to properly handle HTTP status codes 204 (ignored) and 208 (already trained) returned by the Rspamd controller. This ensures that the local metadata cache is successfully cleared and trained message IDs are marked in the database.

## [0.2.5] - 2026-06-06

### Changed
* **Documentation update:** Updated HISTORY.md for the Rspamd Bayes statistics turn.

## [0.2.4] - 2026-06-06

### Changed
* **Default Scan Limit raised to 1000:** Increased the global default scan and download limit on everything to 1000 (up from 50) for users with smaller inboxes who want to process all messages.

## [0.2.3] - 2026-06-06

### Changed
* **Increased Ham Learning Limit:** Raised the default download/scan limit specifically for the `learn_ham` subcommand to 1000 messages (up from 50) to allow more comprehensive training of the Bayes classifier.

## [0.2.2] - 2026-06-06

### Added
* **Stat Warning for Ham Learning:** Added stat warnings to main.go in verbose mode when local email cache files cannot be found.
* **Automatic versioning policy:** Added rule 6 to GEMINI.md enforcing 0.0.1 version increments on all code changes.

### Fixed
* **Ham Learning Path Bug:** Passed account-specific client configuration down to performHamLearning so Rspamd learns emails using the correct account cache directory.

## [0.2.1] - 2026-06-06

### Added
* **Fuzzy Suffix Label Matching:** Enabled specifying target folders/labels without parent and grandparent hierarchies. If a unique leaf folder matches the suffix, the CLI prints the resolved full path and executes the command as usual.

### Fixed
* **JMAP Error Handling Type Bug:** Fixed a compilation failure in `jmap_client.go` where pointer strings (`*string`) were printed directly using `%s`.

## [0.1.1] - 2026-05-30

### Added
* **Personal Whitelist System**: Implemented a local, configuration-driven whitelist (`-wlist add <email>`, `-wlist del <email>`, and `-wlist list` subcommands) stored in `config.json`.
* **Automated Ham Training**: Integrated automatic local Rspamd HAM (non-spam) learning for whitelisted emails to improve Rspamd's statistical classifier accuracy.

### Changed
* **Visual Cleanup**: Cleaned up the inbox scan (`-i`) console output by hiding the red "SPAM EMAILS DETECTED" banner block when zero spam emails are found, replacing it with a clean green success status.

---

## [1.2.0] - 2026-05-29

This major release refactors the command-line interface, implements structured JSON configuration auto-population and bootstrapping, integrates standard Linux system directories, and enhances the language validation architecture.

### Added
* **Flag `-i` (Inbox Scan)**: The default inbox scan mode has been assigned to the new `-i` flag.
* **Flag `-spam` (Spam Auditor)**: Connects securely to the Spam folder, downloads/caches spam emails, and color-prints sender and subject headers.
* **Flag `-spamdel` (Permanent Purge)**: Connects in read-write mode, tags spam messages with `\Deleted`, and expunges them, permanently purging them from Google's servers forever (bypassing Trash).
* **Length-Constrained NLP Validation**: Restored `whatlanggo` statistical NLP language detection to audit Latin-script emails. To eliminate false positives on sparse texts, it is **only run if the email body has at least 20 words**.
* **Auto-Bootstrapping Config**: If `config.json` does not exist, a credentials template is written automatically on startup.
* **Dynamic Config Auto-Population**: Automatically updates existing configuration files with any missing default properties (such as whitelisted languages, thresholds, and limits) without modifying user credentials.
* **Standard System Caching**: Caching directories default to standard Linux user caches (`~/.cache/gmail-spam-checker/`).
* **Cache Auto-Migration**: Automatically detects legacy `./downloaded_emails` configurations and migrates them to the standard `~/.cache` directory.
* **Descriptive Operational Menu**: Displays a detailed operational guide of all commands and options when the program is run with no arguments.
* **Command Validation**: Alerts the user and presents the usage guide if arguments are parsed but no action/operation mode is specified.
* **Graceful Pre-flight Stack Check**: Automatically audits local Redis and Rspamd service ports before making remote IMAP connections, failing early with detailed instructions and exact, colorized Debian/Kali startup commands (`sudo systemctl start redis-server rspamd`).
* **MIME Normalization**: Decodes malformed and non-standard UTF-8 subject headers (e.g. `=?UTF8?Q?` which are missing the hyphen).

### Changed
* **Simplified CLI**: Moved all configuration-centric command-line options (e.g., `-host`, `-user`, `-pass`, `-dir`, `-limit`, `-languages`) entirely into `config.json` to declutter the user interface.
* **Binary Renaming**: Renamed the compiled binary to `gmail_spam_checker`.
* **Usage Outputs**: Extended `flag.Usage` to explicitly output the absolute file path of the configuration file at the bottom of the CLI usage block.

### Dependencies
* Added `github.com/abadojack/whatlanggo` (v1.0.1) for statistical trigram language analysis.
* Retained `github.com/emersion/go-imap` (v1.2.1) and `github.com/fatih/color` (v1.19.0).
