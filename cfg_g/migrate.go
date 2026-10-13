package cfg_g

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// LegacySanitizeLabelForCache is SanitizeLabelForCache's pre-H4 behavior:
// every rune outside [A-Za-z0-9_-] flattened to a shared '_', instead of
// escaped injectively. It collided distinct names -- "corp.com" and
// "corp_com" both sanitized to "corp_com" -- which is the bug H4 fixed.
//
// It is frozen here, used only by the directory-migration helpers below and
// their counterparts in cache/folderindex and cache/virtual, so a directory
// or file written under the old scheme can still be found by the name it
// used to have and renamed forward to the new, injective one. Nothing else
// should call it: it is deliberately the broken mapping, kept only to locate
// its own wreckage.
func LegacySanitizeLabelForCache(label string) string {
	var sb strings.Builder
	for _, r := range label {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('_')
		}
	}
	return strings.ToLower(sb.String())
}

// MigrateDirSanitization renames <baseDir>/<old-sanitized(name)> to
// <baseDir>/<new-sanitized(name)>, if the old one exists and the new one
// does not. It is a one-time, best-effort step for H4: fixing
// SanitizeLabelForCache to stop colliding distinct names onto the same path
// means a name that used to share a path with something else now resolves
// to one only it owns, and whatever was cached under the old, shared path
// has to move there to still be found -- otherwise every account or folder
// whose name held a character outside [A-Za-z0-9_-] would look freshly
// empty the first time it is read after upgrading.
//
// Two names could have collided under the old scheme -- that collision is
// H4's whole point -- so at most one of them still has anything under the
// shared old path (whichever wrote it last). This can only move that
// survivor to the name being resolved right now; it cannot recover what the
// collision already overwrote for the other name, because that data no
// longer exists anywhere to recover.
//
// This runs even under --read-only/--dry-run. It moves data, it does not
// create or discard any: the account or folder it moves is exactly as
// re-derivable by re-fetching as any other cache entry, which is what
// --read-only already exempts caching from.
func MigrateDirSanitization(baseDir, name string) {
	oldKey := LegacySanitizeLabelForCache(name)
	newKey := SanitizeLabelForCache(name)
	if oldKey == newKey {
		return
	}
	renameIfOldExistsAndNewDoesNot(filepath.Join(baseDir, oldKey), filepath.Join(baseDir, newKey))
}

// MigrateFileSanitization renames oldPath to newPath if oldPath exists and
// newPath does not. It is MigrateDirSanitization's counterpart for a caller
// whose sanitized name is embedded in a filename (e.g.
// "token_<sanitized>.json" or a hash-suffixed variant of one) rather than
// being a directory of its own -- the caller computes both paths with
// whatever template it uses and hands them here to do the actual move.
func MigrateFileSanitization(oldPath, newPath string) {
	if oldPath == newPath {
		return
	}
	renameIfOldExistsAndNewDoesNot(oldPath, newPath)
}

func renameIfOldExistsAndNewDoesNot(oldPath, newPath string) {
	if _, err := os.Stat(newPath); err == nil {
		return
	}
	if _, err := os.Stat(oldPath); err != nil {
		return
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		slog.Warn("failed to migrate cache path to the new sanitization scheme",
			slog.String("from", oldPath), slog.String("to", newPath), slog.Any("error", err))
		return
	}
	slog.Info("migrated cache path to the new sanitization scheme",
		slog.String("from", oldPath), slog.String("to", newPath))
}
