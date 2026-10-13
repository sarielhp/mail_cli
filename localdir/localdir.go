// Package localdir resolves the on-disk directory a subsystem keeps its own
// files in: <downloadDir>/<name> when a download directory is configured, and
// ~/.cache/mail_cli/<name> otherwise, so a subsystem still has somewhere to
// write when no account -- and therefore no download directory -- is selected.
package localdir

import (
	"os"
	"path/filepath"
)

// Path returns the directory named name below downloadDir. With no download
// directory it falls back to ~/.cache/mail_cli/name, or a temp directory when
// the home directory cannot be determined.
func Path(downloadDir, name string) string {
	if downloadDir != "" {
		return filepath.Join(downloadDir, name)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "mail_cli", name)
	}
	return filepath.Join(home, ".cache", "mail_cli", name)
}
