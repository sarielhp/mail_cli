package folderindex

import (
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"os"
	"path/filepath"

	"github.com/sarielhp/mail_cli/cfg_g"
)

// MigrateSanitizedNames renames a folder's index files from the pre-H4
// sanitized key (cfg_g.LegacySanitizeLabelForCache) to the current injective
// one, for every name in folderNames whose two sanitizations differ and
// whose old-named files still exist. folderNames is normally an account's
// cached label listing (label.Item.FullName): folderindex has no
// registry of every folder name it has ever indexed, unlike cache/virtual,
// whose files are self-describing, so the caller has to supply what it
// knows.
//
// It reports how many folders it moved. A folder whose old and new
// sanitizations already agree, or that has nothing cached under the old
// name, costs one or two os.Stat calls and nothing else.
func MigrateSanitizedNames(account cachedir.Account, folderNames []string) (int, error) {
	downloadDir := account.Path()
	defer guard(downloadDir)()

	moved := 0
	for _, name := range folderNames {
		oldKey := cfg_g.LegacySanitizeLabelForCache(name)
		newKey := cfg_g.SanitizeLabelForCache(name)
		if oldKey == newKey {
			continue
		}
		didMove, err := renameIndexFiles(downloadDir, oldKey, newKey)
		if err != nil {
			return moved, err
		}
		if didMove {
			moved++
		}
	}
	return moved, nil
}

// renameIndexFiles moves the base and deltas files for one folder from
// oldKey to newKey, skipping either half that has nothing to move or that
// the new key already owns -- migrating must never overwrite content a
// fresh, correctly named index already has.
func renameIndexFiles(downloadDir, oldKey, newKey string) (bool, error) {
	oldBase := filepath.Join(downloadDir, "indexes", oldKey+".json")
	newBase := filepath.Join(downloadDir, "indexes", newKey+".json")
	baseMoved, err := renameIfSourceExistsAndTargetDoesNot(oldBase, newBase)
	if err != nil {
		return baseMoved, err
	}

	oldDeltas := filepath.Join(downloadDir, "indexes", oldKey+".deltas.jsonl")
	newDeltas := filepath.Join(downloadDir, "indexes", newKey+".deltas.jsonl")
	deltasMoved, err := renameIfSourceExistsAndTargetDoesNot(oldDeltas, newDeltas)
	if err != nil {
		return baseMoved || deltasMoved, err
	}

	return baseMoved || deltasMoved, nil
}

func renameIfSourceExistsAndTargetDoesNot(oldPath, newPath string) (bool, error) {
	if _, err := os.Stat(newPath); err == nil {
		return false, nil
	}
	if _, err := os.Stat(oldPath); err != nil {
		return false, nil
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return false, err
	}
	return true, nil
}
