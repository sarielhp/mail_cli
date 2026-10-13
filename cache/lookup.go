package cache

import (
	"encoding/json"
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"os"
	"path/filepath"
	"strings"

	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/email"
)

// FindCachedEmailByID resolves ref -- a message ID as the user typed it, full
// or short -- to a cached message, and names the folder index holding it.
func FindCachedEmailByID(account cachedir.Account, ref string) (email.ID, string, error) {
	downloadDir := account.Path()
	// A full ID is looked for first, through the whole cache, so a short ID
	// that collides with some other message's never shadows an exact match.
	ref = strings.TrimSpace(ref)
	var foundID email.ID
	msg.ForEachID(account, func(id email.ID) bool {
		if strings.EqualFold(string(id), ref) {
			foundID = id
			return true
		}
		return false
	})

	if foundID == "" {
		msg.ForEachID(account, func(id email.ID) bool {
			if email.ComputeShortID(id).Matches(ref) {
				foundID = id
				return true
			}
			return false
		})
	}

	if foundID == "" {
		return "", "", fmt.Errorf("no cached email found with ID %q", ref)
	}

	idxDir := filepath.Join(downloadDir, "indexes")
	entries, err := os.ReadDir(idxDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			data, rErr := os.ReadFile(filepath.Join(idxDir, e.Name()))
			if rErr != nil {
				continue
			}
			var ids []email.ID
			if err := json.Unmarshal(data, &ids); err != nil {
				continue
			}
			for _, id := range ids {
				if id == foundID {
					return foundID, strings.TrimSuffix(e.Name(), ".json"), nil
				}
			}
		}
	}

	return foundID, "", nil
}
