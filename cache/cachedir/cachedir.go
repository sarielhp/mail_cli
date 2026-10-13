// Package cachedir names an account's cache directory.
//
// The cache has two levels: a root (the configured download_dir, holding
// drafts, mbox views and the contacts history) and one directory per account
// beneath it (messages, folder indexes, read state, training records). Both
// used to be the same string field, Config.DownloadDir, which the client
// factory rewrote from the root to the account directory in the configs it
// built. Code holding the session's config instead of a client's read the
// root where it meant the account -- spam learn --ham found no messages,
// trained nothing, and recorded every one as trained.
//
// Account is a distinct type with an unexported path. The per-account cache
// functions take one, so passing the root is a compile error, and the only
// ways to make one are ForAccount, which joins the root and account name the
// one way the program does, and At, for a directory that is not under the
// root (an mbox view, a test).
//
// The package imports nothing from the program, so every layer may use it.
package cachedir

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Account is one account's cache directory.
type Account struct{ path string }

// ForAccount is the cache directory for the account named name under root.
// An account with no name, or one whose name sanitizes to nothing, uses
// "default".
func ForAccount(root, name string) Account {
	dir := SanitizeName(name)
	if dir == "" {
		dir = "default"
	}
	return Account{path: filepath.Join(root, dir)}
}

// At is an account cache at an explicit path, for a cache that does not live
// under the root: an mbox view's, or a test's temporary directory.
func At(path string) Account { return Account{path: path} }

// Path is the directory. It is empty for the zero Account, which the cache
// functions treat as "no cache" -- never as the current directory.
func (a Account) Path() string { return a.path }

// IsZero reports an Account that was never set: a config the client factory
// did not build, such as the session's own.
func (a Account) IsZero() bool { return a.path == "" }

// String is the path, for messages and logs.
func (a Account) String() string { return a.path }

// SanitizeName maps a label or account name to a filesystem-safe
// directory/file name segment. It governs account configuration directories,
// per-account cache directories, folder indexes and virtual mailboxes, so
// it must be injective: two distinct inputs must never produce the same
// output, or their cache entries -- and for account directories, their
// credentials -- collide on one path with the later write winning.
//
// A rune outside the safe set [A-Za-z0-9_-] is escaped as "~<hex>~" rather
// than flattened to a shared '_', so "corp.com" and "corp_com" no longer
// collapse onto the same account directory and distinct Hebrew folder names
// no longer collapse onto the same index file. '~' itself falls outside the
// safe set and is therefore always escaped too, which keeps every escape
// sequence unambiguous: a literal '~' never appears in the output outside of
// one, so the string can always be parsed back into the rune sequence that
// produced it.
func SanitizeName(label string) string {
	var sb strings.Builder
	for _, r := range label {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			sb.WriteRune(r)
			continue
		}
		fmt.Fprintf(&sb, "~%x~", r)
	}
	return strings.ToLower(sb.String())
}
