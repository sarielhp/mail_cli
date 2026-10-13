package gmail

import (
	"fmt"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/report"
	"sort"
	"strconv"
	"strings"
	"time"

	gmailapi "google.golang.org/api/gmail/v1"
)

// listGmailLabelsREST fetches and prints all user and system labels on Gmail.

func findSourceAndDestLabels(labels []*gmailapi.Label, oldName, newName string) (*gmailapi.Label, *gmailapi.Label) {
	var targetLabel *gmailapi.Label
	var destLabel *gmailapi.Label
	for _, l := range labels {
		if strings.EqualFold(l.Name, oldName) {
			targetLabel = l
		}
		if strings.EqualFold(l.Name, newName) {
			destLabel = l
		}
	}
	return targetLabel, destLabel
}

func migrateMessagesBetweenLabels(rep report.Reporter, srv *gmailapi.Service, sourceLabel, destLabel *gmailapi.Label) (int, error) {
	pageToken := ""
	totalMoved := 0
	isSent := sourceLabel.Id == "SENT" || strings.EqualFold(sourceLabel.Name, "SENT")
	for {
		req := srv.Users.Messages.List("me").LabelIds(sourceLabel.Id).IncludeSpamTrash(true)
		if isSent && pageToken != "" {
			req = req.PageToken(pageToken)
		}
		res, err := req.Do()
		if err != nil {
			return 0, fmt.Errorf("failed to list messages in label %q: %w", sourceLabel.Name, err)
		}
		if len(res.Messages) == 0 {
			break
		}

		var ids []email.ID
		for _, m := range res.Messages {
			ids = append(ids, email.ID(m.Id))
		}

		removeLabels := []string{}
		if !isSent {
			removeLabels = append(removeLabels, sourceLabel.Id)
		}
		err = srv.Users.Messages.BatchModify("me", &gmailapi.BatchModifyMessagesRequest{
			Ids:            email.IDStrings(ids),
			AddLabelIds:    []string{destLabel.Id},
			RemoveLabelIds: removeLabels,
		}).Do()
		if err != nil {
			return 0, fmt.Errorf("failed to move batch of messages: %w", err)
		}
		totalMoved += len(ids)
		rep.Infof("Moved %d messages to %q...\n", len(ids), destLabel.Name)

		if isSent {
			pageToken = res.NextPageToken
			if pageToken == "" {
				break
			}
		}
	}
	return totalMoved, nil
}

func mergeGmailLabels(rep report.Reporter, srv *gmailapi.Service, targetLabel, destLabel *gmailapi.Label) error {
	if targetLabel.Id == destLabel.Id {
		rep.Successf("Label is already named %q (no-op).\n", targetLabel.Name)
		return nil
	}

	rep.Infof("Destination label %q already exists (ID: %s). Migrating emails from %q to %q...\n", destLabel.Name, destLabel.Id, targetLabel.Name, destLabel.Name)

	totalMoved, err := migrateMessagesBetweenLabels(rep, srv, targetLabel, destLabel)
	if err != nil {
		return err
	}

	rep.Successf("Successfully moved %d total messages from %q to %q.\n", totalMoved, targetLabel.Name, destLabel.Name)

	rep.Infof("Deleting empty label %q...\n", targetLabel.Name)
	if err := srv.Users.Labels.Delete("me", targetLabel.Id).Do(); err != nil {
		return fmt.Errorf("failed to delete migrated label %q: %w", targetLabel.Name, err)
	}
	rep.Successf("Successfully deleted label %q.\n", targetLabel.Name)
	return nil
}

func patchGmailLabelName(rep report.Reporter, srv *gmailapi.Service, targetLabel *gmailapi.Label, newName string) error {
	rep.Infof("Found label %q (ID: %s, Type: %s). Renaming to %q...\n", targetLabel.Name, targetLabel.Id, targetLabel.Type, newName)

	_, err := srv.Users.Labels.Patch("me", targetLabel.Id, &gmailapi.Label{
		Name: newName,
	}).Do()
	if err != nil {
		return fmt.Errorf("failed to rename label: %w", err)
	}

	rep.Successf("Successfully renamed label %q to %q on Gmail!\n", targetLabel.Name, newName)
	rep.Infof("Note: All messages previously labeled with %q are now automatically labeled with %q.\n", targetLabel.Name, newName)
	return nil
}

// renameGmailLabelREST renames an existing Gmail label by name to a new name.
func renameGmailLabelREST(rep report.Reporter, config *Config, oldName, newName string) error {
	srv, err := GetGmailService(rep, config)
	if err != nil {
		return err
	}

	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	if validateErr := label.ValidateNewName(newName); validateErr != nil {
		return validateErr
	}

	rep.Infof("Searching for label %q on Gmail...\n", oldName)

	labelsRes, err := srv.Users.Labels.List("me").Do()
	if err != nil {
		return fmt.Errorf("failed to list Gmail labels: %w", err)
	}

	targetLabel, destLabel := findSourceAndDestLabels(labelsRes.Labels, oldName, newName)
	if targetLabel == nil {
		return fmt.Errorf("label %q not found on Gmail", oldName)
	}

	if strings.EqualFold(targetLabel.Type, "system") {
		return fmt.Errorf("system label %q cannot be renamed", oldName)
	}

	if destLabel != nil {
		return mergeGmailLabels(rep, srv, targetLabel, destLabel)
	}

	return patchGmailLabelName(rep, srv, targetLabel, newName)
}

// fixGmailLabelsREST checks for user labels of the form x/y where x does not exist,
// and fixes the hierarchy recursively from shallowest to deepest.
func fixGmailLabelsREST(rep report.Reporter, config *Config) error {
	srv, err := GetGmailService(rep, config)
	if err != nil {
		return err
	}

	rep.Infof("Audit started: Checking for missing parent labels on Gmail...\n")

	for {
		labelsRes, err := srv.Users.Labels.List("me").Do()
		if err != nil {
			return fmt.Errorf("failed to list Gmail labels: %w", err)
		}

		existing := make(map[string]bool)
		for _, l := range labelsRes.Labels {
			existing[l.Name] = true
		}

		type missingPrefixItem struct {
			Name  string
			Depth int
		}
		var missingPrefixes []missingPrefixItem
		seenMissing := make(map[string]bool)

		for _, l := range labelsRes.Labels {
			if strings.EqualFold(l.Type, "system") {
				continue
			}
			parts := strings.Split(l.Name, "/")
			if len(parts) <= 1 {
				continue
			}
			for i := 1; i < len(parts); i++ {
				prefix := strings.Join(parts[:i], "/")
				if !existing[prefix] {
					if !seenMissing[prefix] {
						seenMissing[prefix] = true
						missingPrefixes = append(missingPrefixes, missingPrefixItem{
							Name:  prefix,
							Depth: i,
						})
					}
				}
			}
		}

		if len(missingPrefixes) == 0 {
			rep.Successf("No missing parent labels found. Hierarchy is completely clean!\n")
			break
		}

		sort.Slice(missingPrefixes, func(i, j int) bool {
			if missingPrefixes[i].Depth != missingPrefixes[j].Depth {
				return missingPrefixes[i].Depth < missingPrefixes[j].Depth
			}
			return missingPrefixes[i].Name < missingPrefixes[j].Name
		})

		targetPrefix := missingPrefixes[0].Name
		rep.Infof("Found missing parent label %q. Launching hierarchy fix...\n", targetPrefix)

		err = fixLabelHierarchy(rep, srv, targetPrefix, labelsRes.Labels)
		if err != nil {
			return fmt.Errorf("failed to fix hierarchy for parent %q: %w", targetPrefix, err)
		}
	}

	return nil
}

// fixLabelHierarchy handles renaming P_d/* to temp names, creating P_d, and renaming them back.
func fixLabelHierarchy(rep report.Reporter, srv *gmailapi.Service, missingParent string, allLabels []*gmailapi.Label) error {
	parts := strings.Split(missingParent, "/")
	last := parts[len(parts)-1]
	parentPath := strings.Join(parts[:len(parts)-1], "/")

	timestamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	var tmpPrefix string
	if parentPath == "" {
		tmpPrefix = last + "_tmp_" + timestamp
	} else {
		tmpPrefix = parentPath + "/" + last + "_tmp_" + timestamp
	}

	var matchingLabels []*gmailapi.Label
	for _, l := range allLabels {
		if strings.HasPrefix(l.Name, missingParent+"/") {
			matchingLabels = append(matchingLabels, l)
		}
	}

	type RenameRestore struct {
		ID      string
		TmpName string
		OldName string
	}
	var restoreList []RenameRestore

	if len(matchingLabels) > 0 {
		sort.Slice(matchingLabels, func(i, j int) bool {
			return len(matchingLabels[i].Name) > len(matchingLabels[j].Name)
		})

		rep.Infof("Temporarily renaming %d child labels under %q to %q...\n", len(matchingLabels), missingParent, tmpPrefix)
		for _, l := range matchingLabels {
			suffix := strings.TrimPrefix(l.Name, missingParent+"/")
			tmpName := tmpPrefix + "/" + suffix

			_, err := srv.Users.Labels.Patch("me", l.Id, &gmailapi.Label{
				Name: tmpName,
			}).Do()
			if err != nil {
				return fmt.Errorf("failed to rename %q to %q: %w", l.Name, tmpName, err)
			}
			restoreList = append(restoreList, RenameRestore{
				ID:      l.Id,
				TmpName: tmpName,
				OldName: l.Name,
			})
		}
	}

	rep.Infof("Creating parent label %q...\n", missingParent)
	_, err := srv.Users.Labels.Create("me", &gmailapi.Label{
		Name:                  missingParent,
		LabelListVisibility:   "labelShow",
		MessageListVisibility: "show",
	}).Do()
	if err != nil {
		return fmt.Errorf("failed to create label %q: %w", missingParent, err)
	}
	rep.Successf("Parent label %q created successfully.\n", missingParent)

	if len(restoreList) > 0 {
		sort.Slice(restoreList, func(i, j int) bool {
			return len(restoreList[i].OldName) < len(restoreList[j].OldName)
		})

		rep.Infof("Restoring %d child labels back under prefix %q...\n", len(restoreList), missingParent)
		for _, r := range restoreList {
			_, err := srv.Users.Labels.Patch("me", r.ID, &gmailapi.Label{
				Name: r.OldName,
			}).Do()
			if err != nil {
				return fmt.Errorf("failed to restore label from %q to %q: %w", r.TmpName, r.OldName, err)
			}
		}
		rep.Successf("All child labels restored under %q.\n", missingParent)
	}

	return nil
}

// deleteGmailLabelREST deletes an existing custom user label on Gmail by name.
func deleteGmailLabelREST(rep report.Reporter, config *Config, labelName string) error {
	srv, err := GetGmailService(rep, config)
	if err != nil {
		return err
	}

	labelName = strings.TrimSpace(labelName)
	if labelName == "" {
		return fmt.Errorf("label name to delete cannot be empty")
	}

	rep.Infof("Searching for label %q on Gmail...\n", labelName)

	labelsRes, err := srv.Users.Labels.List("me").Do()
	if err != nil {
		return fmt.Errorf("failed to list Gmail labels: %w", err)
	}

	var targetLabel *gmailapi.Label
	for _, l := range labelsRes.Labels {
		if strings.EqualFold(l.Name, labelName) {
			targetLabel = l
			break
		}
	}

	if targetLabel == nil {
		return fmt.Errorf("label %q not found on Gmail", labelName)
	}

	if strings.EqualFold(targetLabel.Type, "system") {
		return fmt.Errorf("system label %q cannot be deleted", labelName)
	}

	rep.Infof("Found label %q (ID: %s, Type: %s). Deleting...\n", targetLabel.Name, targetLabel.Id, targetLabel.Type)

	err = srv.Users.Labels.Delete("me", targetLabel.Id).Do()
	if err != nil {
		return fmt.Errorf("failed to delete label: %w", err)
	}

	rep.Successf("Successfully deleted label %q from Gmail!\n", targetLabel.Name)
	return nil
}
