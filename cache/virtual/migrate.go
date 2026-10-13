package virtual

import (
	"encoding/json"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"os"
	"path/filepath"
	"strings"

	"github.com/sarielhp/mail_cli/cfg_g"
)

// MigrateSanitizedNames re-saves each virtual mailbox whose on-disk file
// name no longer matches the current SanitizeLabelForCache(vm.Name) -- i.e.
// one still sitting under the pre-H4 sanitized name -- so both its file and
// its folder-index mirror move to the name only it now owns.
//
// Unlike a folder index, a virtual mailbox's own JSON records its real Name,
// so this needs no external list of names to check: it is entirely
// self-describing, and just reads every file in the virtual directory to
// find out which ones no longer live where their own Name says they should.
//
// It reports how many mailboxes it moved.
func MigrateSanitizedNames(account cachedir.Account) (int, error) {
	downloadDir := account.Path()
	dir := virtualDir(downloadDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	moved := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		didMove, err := migrateOneFile(downloadDir, dir, e.Name())
		if err != nil {
			return moved, err
		}
		if didMove {
			moved++
		}
	}
	return moved, nil
}

// migrateOneFile re-saves one virtual mailbox file under its current
// sanitized name, if its Name says it should live somewhere other than
// where it was found. Re-saving through Save also regenerates the
// folder-index mirror under the new key with the mailbox's own message IDs,
// which is enough to make it correct again -- the mirror is a derived
// index, not a second source of truth, so there is nothing to migrate there
// beyond letting Save recompute it.
func migrateOneFile(downloadDir, dir, fileName string) (bool, error) {
	data, err := os.ReadFile(filepath.Join(dir, fileName))
	if err != nil {
		return false, nil
	}
	var vm Mailbox
	if err := json.Unmarshal(data, &vm); err != nil || vm.Name == "" {
		return false, nil
	}

	wantFile := cfg_g.SanitizeLabelForCache(vm.Name) + ".json"
	if wantFile == fileName {
		return false, nil
	}
	if _, err := os.Stat(filepath.Join(dir, wantFile)); err == nil {
		// The correctly named file already exists -- a fresh `last`/`search`
		// run since upgrading, most likely. Never overwrite it with the
		// stale old one.
		return false, nil
	}

	if err := Save(cachedir.At(downloadDir), &vm); err != nil {
		return false, err
	}
	_ = os.Remove(filepath.Join(dir, fileName))
	return true, nil
}
