package spam

import (
	"fmt"

	"github.com/sarielhp/mail_cli/cache"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/report"
)

// Forget hands messages back to the filter: it drops the user's spam/not-spam
// decision, so the next scan judges them. Training is left alone -- what
// bogofilter learned from them stays learned -- and nothing changes on the
// server. With dryRun it only reports. It returns how many messages carried a
// decision.
func Forget(rep report.Reporter, account cachedir.Account, ids []email.ID, dryRun bool) (int, error) {
	forgotten := 0
	for _, id := range ids {
		info, err := msg.GetInfo(account, id)
		if err != nil || info.Override == msg.OverrideNone {
			continue
		}
		forgotten++
		if dryRun {
			continue
		}
		if err := msg.SetOverride(account, id, msg.OverrideNone); err != nil {
			return forgotten, fmt.Errorf("forgetting %s: %w", id, err)
		}
	}
	verb := "Forgot"
	if dryRun {
		verb = "[DRY-RUN] Would forget"
	}
	rep.Successf("%s the decision on %d of %d message(s); the filter judges them on the next scan.\n", verb, forgotten, len(ids))
	return forgotten, nil
}

// ForgetByID resolves refs, as the user typed them, against the cache and
// forgets the decision on each.
func ForgetByID(rep report.Reporter, account cachedir.Account, refs []string, dryRun bool) error {
	ids := make([]email.ID, 0, len(refs))
	for _, ref := range refs {
		id, _, err := cache.FindCachedEmailByID(account, ref)
		if err != nil {
			return err
		}
		ids = append(ids, id)
	}
	_, err := Forget(rep, account, ids, dryRun)
	return err
}

// ForgetFolder forgets the decision on every message the cache lists in
// folder. It reads the local folder index, so a folder never opened has
// nothing to forget.
func ForgetFolder(rep report.Reporter, account cachedir.Account, folder label.Folder, dryRun bool) error {
	ids, err := folderindex.IDs(account, folder.String())
	if err != nil {
		return fmt.Errorf("no cached index for folder %q: %w", folder, err)
	}
	_, err = Forget(rep, account, ids, dryRun)
	return err
}
