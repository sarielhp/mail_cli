package organize

import (
	"fmt"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"strings"

	"github.com/sarielhp/mail_cli/cache"
	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/report"
)

// All archives every folder matching sourcePrefix into target, moving each
// folder's messages on the server and in the local index. A source folder equal
// to target is skipped, and under ReadOnly it reports the moves it would make
// without performing any.
func All(rep report.Reporter, config *cfg_g.Config, client interface {
	mailclient.LabelReader
	mailclient.EmailFetcher
	mailclient.EmailWriter
	mailclient.ConfigProvider
}, sourcePrefix string, target label.Folder) error {
	// Archiving fans out over a subtree, so it asks for every folder under the
	// prefix rather than for one folder. Routing it through the single-folder
	// resolver made `archive all test` fail as ambiguous when `test/newsletter`
	// and `test/other` both existed -- which is the whole point of the command.
	matchedLabels, err := mailclient.ResolveAll(client, sourcePrefix)
	if err != nil {
		return err
	}

	for _, matchedLabel := range matchedLabels {
		if strings.EqualFold(matchedLabel.String(), target.String()) {
			if config.Verbose {
				rep.Infof("Skipping source label %s because it matches target archive folder %s.\n", matchedLabel, target)
			}
			continue
		}
		cacheDirName := cfg_g.SanitizeLabelForCache(matchedLabel.String())
		messageIDs, err := client.FetchAndDownloadEmails(matchedLabel, cacheDirName)
		if err != nil {
			return err
		}
		if len(messageIDs) == 0 {
			rep.Infof("No messages found in label %s.\n", matchedLabel)
			continue
		}
		if config.ReadOnly {
			rep.Printf("[DRY-RUN] Would archive %d message(s) to %s\n", len(messageIDs), target)
			continue
		}
		rep.Infof("Moving %d message(s) from %s to %s on server...\n", len(messageIDs), matchedLabel, target)
		if err := client.MoveEmail(messageIDs, matchedLabel, target); err != nil {
			return fmt.Errorf("failed to move messages from %s: %w", matchedLabel, err)
		}
		for _, msgID := range messageIDs {
			cd := client.Config().AccountCache
			_ = folderindex.MoveBatch(cd, []email.ID{msgID}, matchedLabel.String(), target.String())
		}
		rep.Successf("Successfully archived %d message(s) from %s to %s.\n", len(messageIDs), matchedLabel, target)
	}
	return nil
}

func archiveTargetName(client interface {
	mailclient.LabelReader
	mailclient.BackendInfo
	mailclient.ConfigProvider
}) (string, error) {
	labels, err := client.GetMatchingLabels("")
	if err != nil {
		return "", err
	}
	for _, l := range labels {
		if strings.EqualFold(l, "archive") {
			return l, nil
		}
	}
	for _, l := range labels {
		if strings.HasSuffix(strings.ToLower(l), "/archive") {
			return l, nil
		}
	}
	recFolder := client.InboxFolder().String()
	if recFolder != "" && !strings.EqualFold(recFolder, "inbox") {
		for _, l := range labels {
			if strings.EqualFold(l, recFolder) {
				return l, nil
			}
		}
		for _, l := range labels {
			if strings.HasSuffix(strings.ToLower(l), "/"+strings.ToLower(recFolder)) {
				return l, nil
			}
		}
	}
	for _, l := range labels {
		if strings.EqualFold(l, "received") {
			return l, nil
		}
	}
	for _, l := range labels {
		if strings.HasSuffix(strings.ToLower(l), "/received") {
			return l, nil
		}
	}

	// Fallback for Gmail: if it's Gmail, we can use "Archive" even if it doesn't exist on the server
	isGmail := client.BackendType() == "gmail"
	if isGmail {
		return "Archive", nil
	}

	return "", fmt.Errorf("archive target folder not found on server (neither 'Archive' nor 'Received' exists)")
}

func trashTargetName(client mailclient.LabelReader) (string, error) {
	labels, err := client.GetMatchingLabels("")
	if err != nil {
		return "", err
	}
	for _, l := range labels {
		if strings.EqualFold(l, "trash") {
			return l, nil
		}
	}
	for _, l := range labels {
		if strings.HasSuffix(strings.ToLower(l), "/trash") {
			return l, nil
		}
	}
	for _, l := range labels {
		if strings.EqualFold(l, "deleted items") {
			return l, nil
		}
	}
	for _, l := range labels {
		if strings.HasSuffix(strings.ToLower(l), "/deleted items") {
			return l, nil
		}
	}
	for _, l := range labels {
		if strings.EqualFold(l, "deleted") {
			return l, nil
		}
	}
	return "", fmt.Errorf("trash target folder not found on server (neither 'Trash' nor 'Deleted Items' exists)")
}

func draftsTargetName(client interface {
	mailclient.LabelReader
	mailclient.BackendInfo
}) (string, error) {
	labels, err := client.GetMatchingLabels("")
	if err == nil {
		for _, l := range labels {
			if strings.EqualFold(l, "drafts") || strings.EqualFold(l, "draft") {
				return l, nil
			}
		}
		for _, l := range labels {
			lower := strings.ToLower(l)
			if strings.HasSuffix(lower, "/drafts") || strings.HasSuffix(lower, "/draft") {
				return l, nil
			}
		}
	}
	if client.BackendType() == "gmail" {
		return "DRAFTS", nil
	}
	return "Drafts", nil
}

func sentTargetName(client interface {
	mailclient.LabelReader
	mailclient.BackendInfo
}) (string, error) {
	labels, err := client.GetMatchingLabels("")
	if err == nil {
		for _, l := range labels {
			if strings.EqualFold(l, "sent") || strings.EqualFold(l, "sent items") || strings.EqualFold(l, "sent messages") {
				return l, nil
			}
		}
		for _, l := range labels {
			lower := strings.ToLower(l)
			if strings.HasSuffix(lower, "/sent") || strings.HasSuffix(lower, "/sent items") || strings.HasSuffix(lower, "/sent messages") {
				return l, nil
			}
		}
	}
	if client.BackendType() == "gmail" {
		return "SENT", nil
	}
	return "Sent", nil
}

// ArchiveByID moves the message named by targetID from the folder the cache
// says holds it into target, updating the server and the local folder index. It
// errors when the ID is not in the cache, and under ReadOnly it reports the move
// without making it.
func ArchiveByID(rep report.Reporter, config *cfg_g.Config, client interface {
	mailclient.EmailWriter
	mailclient.ConfigProvider
}, target label.Folder, targetID string) error {
	msgID, folderName, err := cache.FindCachedEmailByID(client.Config().AccountCache, targetID)
	if err != nil {
		return err
	}

	if config.ReadOnly {
		rep.Printf("[DRY-RUN] Would archive message %s to %s\n", targetID, target)
		return nil
	}

	rep.Infof("Moving message %s (%s) to %s on server...\n", targetID, msgID, target)
	if err := client.MoveEmail([]email.ID{msgID}, label.FromServer(folderName), target); err != nil {
		return fmt.Errorf("failed to move message: %w", err)
	}

	cd := client.Config().AccountCache
	_ = folderindex.MoveBatch(cd, []email.ID{msgID}, folderName, target.String())

	rep.Successf("Successfully archived message %s to %s.\n", targetID, target)
	return nil
}

// ResolveArchiveTarget names the folder archived mail is filed into. The name comes from the server's
// listing, or is the backend's well-known default.
func ResolveArchiveTarget(client interface {
	mailclient.LabelReader
	mailclient.BackendInfo
	mailclient.ConfigProvider
}) (label.Folder, error) {
	name, err := archiveTargetName(client)
	if err != nil {
		return label.Folder{}, err
	}
	return label.FromServer(name), nil
}

// ResolveTrashTarget names the folder deleted mail is moved to. The name comes from the server's
// listing, or is the backend's well-known default.
func ResolveTrashTarget(client mailclient.LabelReader) (label.Folder, error) {
	name, err := trashTargetName(client)
	if err != nil {
		return label.Folder{}, err
	}
	return label.FromServer(name), nil
}

// ResolveDraftsTarget names the folder drafts are saved in. The name comes from the server's
// listing, or is the backend's well-known default.
func ResolveDraftsTarget(client interface {
	mailclient.LabelReader
	mailclient.BackendInfo
}) (label.Folder, error) {
	name, err := draftsTargetName(client)
	if err != nil {
		return label.Folder{}, err
	}
	return label.FromServer(name), nil
}

// ResolveSentTarget names the folder sent mail is kept in. The name comes from the server's
// listing, or is the backend's well-known default.
func ResolveSentTarget(client interface {
	mailclient.LabelReader
	mailclient.BackendInfo
}) (label.Folder, error) {
	name, err := sentTargetName(client)
	if err != nil {
		return label.Folder{}, err
	}
	return label.FromServer(name), nil
}

// IsAuthoredFolder reports whether folder holds mail the account's owner wrote
// -- its Sent or Drafts folder -- which no spam or political verdict applies
// to. Deciding by folder rather than by sender is deliberate: spam routinely
// forges the recipient's own address in From, so trusting the address would
// wave that mail through.
func IsAuthoredFolder(client interface {
	mailclient.LabelReader
	mailclient.BackendInfo
}, folder label.Folder) bool {
	name := folder.String()
	if sent, err := sentTargetName(client); err == nil && strings.EqualFold(sent, name) {
		return true
	}
	drafts, err := draftsTargetName(client)
	return err == nil && strings.EqualFold(drafts, name)
}
