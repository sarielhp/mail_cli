# Architecture

How `mail_cli` is put together, and why. The package inventory at the bottom is
generated; everything above it is the reasoning that a listing cannot carry.

> **This file used to be actively misleading.** It described a flat
> `package main` layout, listed about twenty-six files that no longer existed,
> documented a `MailClient` interface whose every signature had changed, and
> told the reader *"Never create subdirectories"* against a tree of thirty
> packages. Anyone following it edited files that were not there. The durable
> fix was to stop hand-maintaining the parts a tool can derive — see
> `usage/architecture.go` and `TestArchitectureInventoryIsNotStale`.

---

## The shape

Dependencies point one way, from the edges inward:

```
main                    composition root: builds everything, binds nothing to itself
 ├── cli ── tui         the two front ends
 │    └── runner        implements the interfaces cli and tui declare
 │         └── mailops  the factory: the only package that may name a backend
 │              └── mailclient   the interfaces a backend satisfies
 │                   └── backend/{gmail,outlook,jmap,mbox}
 └── report             the output port, a leaf that imports nothing of ours
```

`cli` and `runner` do not import each other. `cli` **declares** the interfaces
it needs (`MailOps`, `SpamOps`, `RuleOps`, …), `runner` happens to satisfy them,
and `main` binds the two together in `wiring.go`. Neither side has to change
when the other does, and `cli`'s tests substitute stubs without a backend
anywhere in the process.

## The four rules, and what each one prevents

These are enforced by `tools/arch_check.rb`, one of the fast checks `tools/gate` runs. They
are checks rather than conventions because each one had already been violated.

**1. Nothing below the UI prints.** Every layer under `cli`/`tui` reports through
`report.Reporter` or writes to an `io.Writer` the caller supplied. Printing
below the boundary cannot be redirected, cannot be asserted on without swapping
`os.Stdout`, and corrupts the TUI display — which is why a global called
`app.TuiActive` once existed to mute the domain layer, and why deleting it
required removing 454 `fmt.Print*` calls.

**2. Nothing below the UI imports the UI.** A backend that imports a rendering
package cannot be driven by any other front end.

**3. Only `mailops` names a concrete backend.** Choosing one is the factory's
job. Importing `gmail`, `outlook`, `jmap` or `mbox` anywhere else reintroduces
exactly the coupling `mailops` exists to remove.

**4. Nothing below `cli` reads CLI flag state.** Reading the parser's globals
makes a package impossible to call with different options from the TUI, or
twice in one test. Options travel as structs — `scan.Options`, `show.Options`.

## Two channels, not one

Status and product are different outputs, and conflating them is how `--json`
and `--quiet` become impossible to add later:

- **`report.Reporter`** carries progress, warnings and status. It can be
  silenced. `report.Discard()` in a test, a no-op under `--json`.
- **`io.Writer`** carries the thing the user asked for — the listing, the
  message, the JSON.

Suppressing progress must never suppress the answer.

## Read-only and dry-run are different promises

- **`--read-only`** forbids server mutations and any local write that would
  record something that did not happen. It deliberately permits caching:
  without downloaded bodies a message could not be displayed at all, and a
  cached body is a fact the server reported, re-derivable by deleting the cache.
- **`--dry-run`** additionally forbids the local config and cache writes, and
  reports what it would have done. It implies `--read-only`; the reverse is not
  true.

`mailclient.ReadOnlyMailClient` is the enforcement point, decorating a real
client and refusing or simulating every mutating method. **All live-account
verification goes through it**, including by hand — running the binary against
a real account to answer a question is live testing.

## Where the decorators sit

```
CheckingMailClient      cache reconciliation; owns the folder index
  └── ReadOnlyMailClient    present only under --read-only / --dry-run
        └── backend client
```

The order matters. `ReadOnlyMailClient` sits *inside*, so under `--read-only`
the delegate returns success having changed nothing; `CheckingMailClient` must
therefore not reconcile, or it would record a move that never happened.

## Testing seams

- `mailclient.MockMailClient` — records mutations, `FailOn` for one failing
  method, `FailOnMessage` for a mid-batch partial failure.
- `cli/deps_stub_test.go` — stubs for every interface `cli` declares. A question
  about which flags a command accepts is answered by
  `InitCLI(session).Execute(argv)` here, with no network at all.
- `integration/` — live suites behind `MAIL_CLI_LIVE=1`, never run by default.

---

<!-- BEGIN GENERATED: package inventory -->

*Generated from `go list -json ./...`. Do not edit by hand; run `go run . docs`*

55 packages.

| Package | Files | Purpose |
|---|---:|---|
| `.` | 5 | Command mail_cli is a command-line mail client. |
| `app` | 9 | Package app holds the small utilities every layer may use: colour styles and the [+]/[!] prefixes, the loggers, temporary directories, the version string, and ErrUsage. |
| `backend/backendtest` | 1 | Package backendtest holds the test helpers the concrete mail backends share. |
| `backend/gmail` | 21 | Package gmail implements the mail client interfaces against the Gmail REST API, including its OAuth flow. |
| `backend/jmap` | 11 | Package jmap implements the mail client interfaces against a JMAP server. |
| `backend/mbox` | 3 | Package mbox implements the mail client interfaces against a local mbox file, which makes it the backend that needs no account and no network. |
| `backend/outlook` | 11 | Package outlook implements the mail client interfaces against the Microsoft Graph API. |
| `backend/paging` | 1 | Package paging converges a batched walk over a result set that the batch function itself is shrinking. |
| `backend/political` | 1 | Package political classifies cached messages as political donation mail. |
| `backend/spamlearn` | 1 | Package spamlearn records which messages a backend has trained the spam filter on. |
| `bogofiltertest` | 1 | Package bogofiltertest installs a stand-in bogofilter executable for tests. |
| `browser` | 1 | Package browser launches a web browser on a URL or local file. |
| `cache` | 4 | Package cache is the on-disk store for everything the server told us: which messages a folder holds, which have been read or replied to, and the cached label listing. |
| `cache/cachedir` | 1 | Package cachedir names an account's cache directory. |
| `cache/dirlock` | 4 | Package dirlock serializes read-modify-write access to a cache directory. |
| `cache/folderindex` | 3 | Package folderindex maintains the local record of which message IDs are in which folder, so a listing can be drawn without asking the server. |
| `cache/msg` | 5 | Package msg stores raw message bodies on disk and the classification derived from them. |
| `cache/msgstore` | 1 | Package msgstore serves raw messages by ID, reading whatever the local cache already holds and downloading only what is missing. |
| `cache/virtual` | 2 | Package virtual stores named result sets: a list of message IDs plus the folder each came from, saved under the cache directory. |
| `calendar` | 3 | Package calendar reads the .ics attachments that arrive as meeting invitations, turning them into an ICSEvent the rest of the program can show or file. |
| `calendar/google` | 5 | Package google integrates with the Google Calendar API: adding events parsed from .ics mail attachments, checking whether they are already present, and printing a week's schedule. |
| `cfg_acc` | 2 | Package cfg_acc holds the per-account configuration types: the account itself, its routing rules, and its whitelist and blacklist. |
| `cfg_g` | 7 | Package cfg_g is the global configuration: loading and saving the file, resolving which account a command acts on, and parsing the %label references the command line takes. |
| `cfghandler` | 3 | Package cfghandler implements the config subcommands: show, set, unset and check. |
| `cli` | 50 | Package cli is the command-line front end: the command tree, the flags, and the translation from what the user typed into a call on an operation. |
| `contacts` | 7 | Package contacts holds the local address book: the contacts file and its history, importers for mutt aliases and Google Contacts, recipient harvesting from mail, and the ranked suggestions offered while composing. |
| `draft` | 1 | Package draft is the local copy of every message being composed: one draft.eml per compose session under <root>/YYYY/MM/DD/N. It knows nothing about servers. |
| `email` | 23 | Package email parses messages: headers and their encoded words, MIME parts and attachments, HTML reduced to the words it displays, and the threading of a folder into conversations. |
| `filebrowser` | 3 | Package filebrowser is the interactive directory picker used when saving an attachment, built on bubbletea like the rest of the interface. |
| `filebrowser/zonetest` | 1 | Package zonetest waits for bubblezone to record a zone a view has drawn. |
| `integration` | 1 | Package integration holds the tests that talk to a real account. |
| `interactive` | 1 | Package interactive holds the tests that drive mail_cli in a real terminal. |
| `label` | 6 | Package label is about folder names: matching one the user typed against the ones the server has, ordering a listing, and nesting it into a tree. |
| `last` | 2 | Package last implements the listing of the most recent messages across every folder of an account. |
| `llm` | 2 | Package llm summarises a message through an OpenAI-compatible chat endpoint. |
| `localdir` | 1 | Package localdir resolves the on-disk directory a subsystem keeps its own files in: <downloadDir>/<name> when a download directory is configured, and ~/.cache/mail_cli/<name> otherwise, so a subsystem still has somewhere to write when no account -- and therefore no download directory -- is selected. |
| `mailclient` | 7 | Package mailclient defines what a mail backend must be able to do, and the decorators that wrap one. |
| `mailops` | 3 | Package mailops builds backend clients: it resolves an account to its per-account config, instantiates the right backend for its type, and wraps it in the read-only and caching decorators. |
| `message_info` | 1 | Package message_info stores the cache's per-message index: where a message sits on disk, and what classification it has been given. |
| `organize` | 3 | Package organize archives mail: moving everything under a label prefix into the account's archive folder. |
| `outbox` | 1 | Package outbox provides an offline send queue: RFC822 messages spooled to disk when offline or when asynchronous queuing is requested. |
| `pgp` | 7 | Package pgp is the mail program's OpenPGP layer: it shells out to the system gpg and knows how a PGP/MIME message is shaped. |
| `report` | 1 | Package report is how the layers below the user interface say what they are doing, without deciding where it goes or what it looks like. |
| `runner` | 11 | Package runner orchestrates whole commands: the account-spanning scan, show and last, rule import and export, and the service types that implement the interfaces cli and tui declare. |
| `scan` | 10 | Package scan is the spam pass over a folder: download what is there, apply the account's routing rules, classify the remainder with bogofilter, and move what the rules and the classifier agree on. |
| `show` | 3 | Package show renders one message for reading: headers, the decoded body, attachments, and any calendar invitation it carries. |
| `sieve` | 2 | Package sieve exports an account's routing rules as a Sieve script, for a server that can filter mail without this program running. |
| `spam` | 6 | Package spam is the manual side of spam handling: marking a message, taking the mark back, and training bogofilter from either. |
| `terminal` | 1 | Package terminal holds the low-level terminal queries that several packages need and none of them owns. |
| `train` | 1 | Package train records which messages the spam filter has already been trained on. |
| `tui` | 46 | Package tui is the interactive front end, built on bubbletea. |
| `tui/render` | 5 | Package render turns the TUI's model state into styled terminal output: themes, coloured segments, and the individual lines of the folder tree and message list. |
| `ui` | 7 | Package ui formats the listings the command line prints: folder trees, label listings, message summaries, filter listings. |
| `usage` | 3 | Package usage owns the generated documentation: the per-command Markdown tree under docs/clihelp, and the package inventory inside docs/ARCHITECTURE.md. |
| `watcher` | 4 | Package watcher polls mail accounts in the background and announces what is new: a desktop notification, a user script, and a flush of the offline outbox. |

<!-- END GENERATED: package inventory -->
