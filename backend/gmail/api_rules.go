package gmail

import (
	"bytes"
	"fmt"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/report"
	"mime"
	"net/mail"
	"strings"

	gmailapi "google.golang.org/api/gmail/v1"
)

// checkAndApplyRulesREST checks rules against downloaded emails and applies labels via the Gmail API.
func matchRulesForCachedMessages(rep report.Reporter, messageIDs []email.ID, config *Config) (map[rule][]email.ID, map[email.ID]bool) {
	dec := new(mime.WordDecoder)
	ruleMatches := make(map[rule][]email.ID)
	matchedIDs := make(map[email.ID]bool)

	for _, id := range messageIDs {
		rawBytes, err := msg.Read(config.AccountCache, id)
		if err != nil {
			if config.Verbose {
				rep.Warnf("Warning: Failed to open cached email for rule checking (ID %s): %v\n", id, err)
			}
			continue
		}
		mailMsg, err := mail.ReadMessage(bytes.NewReader(rawBytes))
		if err != nil {
			if config.Verbose {
				rep.Warnf("Warning: Failed to parse cached email headers for rule checking (ID %s): %v\n", id, err)
			}
			continue
		}

		sender := email.ParseEmailAddress(mailMsg.Header.Get("From"))
		subject := email.DecodeHeader(dec, mailMsg.Header.Get("Subject"))

		if rule := cfg_acc.MatchRules(config.Rules, sender, subject); rule != nil {
			ruleMatches[*rule] = append(ruleMatches[*rule], id)
			matchedIDs[id] = true
			if config.Verbose {
				rep.Infof("rule Match (ID %s, Subject: %q) matches rule -> label %q\n", id, subject, rule.Label)
			}
		}
	}
	return ruleMatches, matchedIDs
}

func resolveSourceLabelID(sourceLabelName string, labelNameToID map[string]string) (string, error) {
	if strings.EqualFold(sourceLabelName, "inbox") {
		return "INBOX", nil
	}
	if id, ok := labelNameToID[strings.ToLower(sourceLabelName)]; ok {
		return id, nil
	}
	return "", fmt.Errorf("source label %q not found on Gmail", sourceLabelName)
}

func buildRemovableLabels(sourceLabelID string, labelNameToID map[string]string, targetLabelID string) []string {
	removeLabels := []string{sourceLabelID}
	if sourceLabelID == "INBOX" {
		if recID, ok := labelNameToID["received"]; ok {
			removeLabels = append(removeLabels, recID)
		}
	} else {
		removeLabels = append(removeLabels, "INBOX")
	}

	var finalRemoveLabels []string
	for _, rl := range removeLabels {
		if rl != targetLabelID {
			finalRemoveLabels = append(finalRemoveLabels, rl)
		}
	}
	return finalRemoveLabels
}

func filterAffectedRuleIDs(rep report.Reporter, srv *gmailapi.Service, ids []email.ID,
	targetLabelID string, removeLabels []string, matchedIDs map[email.ID]bool,
	config *Config, ruleLabel string,
) []email.ID {
	var affectedIDs []email.ID
	for _, id := range ids {
		msgMeta, errMeta := srv.Users.Messages.Get("me", string(id)).Format("minimal").Do()
		if errMeta != nil {
			affectedIDs = append(affectedIDs, id)
			continue
		}

		hasTargetLabel := false
		for _, lid := range msgMeta.LabelIds {
			if lid == targetLabelID {
				hasTargetLabel = true
				break
			}
		}

		hasLabelsToRemove := false
		for _, rlid := range removeLabels {
			for _, lid := range msgMeta.LabelIds {
				if lid == rlid {
					hasLabelsToRemove = true
					break
				}
			}
			if hasLabelsToRemove {
				break
			}
		}

		if !hasTargetLabel || hasLabelsToRemove {
			affectedIDs = append(affectedIDs, id)
			continue
		}
		matchedIDs[id] = false
		if config.Verbose {
			rep.Infof("rule match has 0 effect (already labeled %q and not in removable labels) for ID %s. Keeping message as-is.\n", ruleLabel, id)
		}
	}
	return affectedIDs
}

func applyBatchModifyRule(rep report.Reporter, srv *gmailapi.Service, rule rule, affectedIDs []string, targetLabelID string, removeLabels []string, sourceLabelName string) error {
	ruleDesc := rule.Sender
	if rule.Subject != "" {
		ruleDesc = fmt.Sprintf("subject: %s*", rule.Subject)
	}

	if rule.Exported {
		rep.Warnf("Warning: Applying local rule for '%s' -> %q, which has already been exported to Gmail server filters!\n", ruleDesc, rule.Label)
	}
	rep.Successf("Applying rule: labeling %d emails matching '%s' with %q and archiving them from %s\n", len(affectedIDs), ruleDesc, rule.Label, sourceLabelName)

	err := srv.Users.Messages.BatchModify("me", &gmailapi.BatchModifyMessagesRequest{
		Ids:            affectedIDs,
		AddLabelIds:    []string{targetLabelID},
		RemoveLabelIds: removeLabels,
	}).Do()
	if err != nil {
		return fmt.Errorf("failed to batch modify messages for rule %q: %w", ruleDesc, err)
	}
	return nil
}

// checkAndApplyRulesREST checks rules against downloaded emails and applies labels via the Gmail API.
func checkAndApplyRulesREST(rep report.Reporter, messageIDs []email.ID, config *Config, sourceLabelName string, _ string) ([]email.ID, error) {
	if len(messageIDs) == 0 || len(config.Rules) == 0 {
		return messageIDs, nil
	}

	ruleMatches, matchedIDs := matchRulesForCachedMessages(rep, messageIDs, config)
	if len(matchedIDs) == 0 {
		return messageIDs, nil
	}

	srv, err := GetGmailService(rep, config)
	if err != nil {
		return messageIDs, err
	}

	labelsRes, err := srv.Users.Labels.List("me").Do()
	if err != nil {
		return messageIDs, fmt.Errorf("failed to list Gmail labels: %w", err)
	}

	labelNameToID := make(map[string]string)
	labelIDToName := make(map[string]string)
	for _, l := range labelsRes.Labels {
		labelNameToID[strings.ToLower(l.Name)] = l.Id
		labelIDToName[l.Id] = l.Name
	}

	sourceLabelID, err := resolveSourceLabelID(sourceLabelName, labelNameToID)
	if err != nil {
		return messageIDs, err
	}

	for rule, ids := range ruleMatches {
		targetLabelID, err := ensureLabelHierarchyREST(rep, srv, rule.Label, labelNameToID, labelIDToName)
		if err != nil {
			return messageIDs, err
		}

		finalRemoveLabels := buildRemovableLabels(sourceLabelID, labelNameToID, targetLabelID)
		affectedIDs := filterAffectedRuleIDs(rep, srv, ids, targetLabelID, finalRemoveLabels, matchedIDs, config, rule.Label)
		if len(affectedIDs) == 0 {
			continue
		}

		if err := applyBatchModifyRule(rep, srv, rule, email.IDStrings(affectedIDs), targetLabelID, finalRemoveLabels, sourceLabelName); err != nil {
			return messageIDs, err
		}
	}

	var remainingIDs []email.ID
	for _, id := range messageIDs {
		if !matchedIDs[id] {
			remainingIDs = append(remainingIDs, id)
		}
	}

	return remainingIDs, nil
}

// exportRulesToGmailREST exports local rules from config.json to Gmail filters.
func updateLocalRulesInConfig(config *Config, rules []rule) error {
	resolved, err := cfg_g.Resolve(config)
	if err != nil {
		return err
	}
	resolved.Account.Rules = rules
	return cfg_g.SaveConfigFile(resolved.Path, resolved.File)
}

// findMinimumUniquePrefixLength finds the smallest prefix length n >= 1
// such that all prefixes of length n of the given IDs are unique.
func findMinimumUniquePrefixLength(ids []string) int {
	if len(ids) <= 1 {
		return 1
	}

	maxLen := 0
	for _, id := range ids {
		if len(id) > maxLen {
			maxLen = len(id)
		}
	}

	for n := 1; n <= maxLen; n++ {
		seen := make(map[string]bool)
		unique := true
		for _, id := range ids {
			pref := id
			if len(id) > n {
				pref = id[:n]
			}
			if seen[pref] {
				unique = false
				break
			}
			seen[pref] = true
		}
		if unique {
			return n
		}
	}
	return maxLen
}

func formatFilterCriteria(f *gmailapi.Filter) string {
	if f.Criteria == nil {
		return "Any message"
	}
	var critParts []string
	if f.Criteria.From != "" {
		critParts = append(critParts, fmt.Sprintf("From: %s", f.Criteria.From))
	}
	if f.Criteria.To != "" {
		critParts = append(critParts, fmt.Sprintf("To: %s", f.Criteria.To))
	}
	if f.Criteria.Subject != "" {
		critParts = append(critParts, fmt.Sprintf("Subject: %s", f.Criteria.Subject))
	}
	if f.Criteria.Query != "" {
		critParts = append(critParts, fmt.Sprintf("Has words: %s", f.Criteria.Query))
	}
	if f.Criteria.NegatedQuery != "" {
		critParts = append(critParts, fmt.Sprintf("Doesn't have: %s", f.Criteria.NegatedQuery))
	}
	if len(critParts) == 0 {
		return "Any message"
	}
	return strings.Join(critParts, ", ")
}

func formatFilterActionLabels(labelIDs []string, labelIDToName map[string]string) []string {
	var names []string
	for _, id := range labelIDs {
		if name, ok := labelIDToName[id]; ok {
			names = append(names, name)
			continue
		}
		names = append(names, id)
	}
	return names
}

func formatFilterActions(f *gmailapi.Filter, labelIDToName map[string]string) string {
	if f.Action == nil {
		return "No action defined"
	}
	var actions []string
	if len(f.Action.AddLabelIds) > 0 {
		adds := formatFilterActionLabels(f.Action.AddLabelIds, labelIDToName)
		actions = append(actions, fmt.Sprintf("Add labels: %s", strings.Join(adds, ", ")))
	}
	if len(f.Action.RemoveLabelIds) > 0 {
		removes := formatFilterActionLabels(f.Action.RemoveLabelIds, labelIDToName)
		actions = append(actions, fmt.Sprintf("Remove labels: %s", strings.Join(removes, ", ")))
	}
	if f.Action.Forward != "" {
		actions = append(actions, fmt.Sprintf("Forward to: %s", f.Action.Forward))
	}
	if len(actions) == 0 {
		return "No action defined"
	}
	return strings.Join(actions, "; ")
}

// gmailFilters fetches the remote filters as data. It used to print them, so
// the listing layout lived in the backend and each backend had its own.
func gmailFilters(rep report.Reporter, config *Config) (mailclient.FilterListing, error) {
	listing := mailclient.FilterListing{Title: "REMOTE GMAIL FILTERS"}

	srv, err := GetGmailService(rep, config)
	if err != nil {
		return listing, err
	}

	labelsRes, err := srv.Users.Labels.List("me").Do()
	if err != nil {
		return listing, HandleScopeError(rep, config, fmt.Errorf("failed to list Gmail labels: %w", err))
	}
	labelIDToName := make(map[string]string, len(labelsRes.Labels))
	for _, l := range labelsRes.Labels {
		labelIDToName[l.Id] = l.Name
	}

	filtersRes, err := srv.Users.Settings.Filters.List("me").Do()
	if err != nil {
		return listing, HandleScopeError(rep, config, fmt.Errorf("failed to list Gmail filters: %w", err))
	}

	var ids []string
	for _, f := range filtersRes.Filter {
		ids = append(ids, f.Id)
	}
	prefixLen := findMinimumUniquePrefixLength(ids)

	for _, f := range filtersRes.Filter {
		displayID := f.Id
		if len(displayID) > prefixLen {
			displayID = displayID[:prefixLen] + "..."
		}
		var sender, subject, targetLabel string
		if f.Criteria != nil {
			sender = f.Criteria.From
			subject = f.Criteria.Subject
		}
		if f.Action != nil && len(f.Action.AddLabelIds) > 0 {
			targetLabel = labelIDToName[f.Action.AddLabelIds[0]]
		}
		listing.Filters = append(listing.Filters, mailclient.Filter{
			ID:          "Filter ID: " + displayID,
			RemoteID:    f.Id,
			Match:       formatFilterCriteria(f),
			Action:      formatFilterActions(f, labelIDToName),
			Sender:      sender,
			Subject:     subject,
			TargetLabel: targetLabel,
		})
	}
	return listing, nil
}

// deleteGmailFilterREST deletes a remote filter from Gmail by filter ID.
func deleteGmailFilterREST(rep report.Reporter, config *Config, filterID string) error {
	srv, err := GetGmailService(rep, config)
	if err != nil {
		return err
	}
	if err := srv.Users.Settings.Filters.Delete("me", filterID).Do(); err != nil {
		if IsInsufficientScopeError(err) {
			return HandleScopeError(rep, config, err)
		}
		return fmt.Errorf("failed to delete Gmail filter %s: %w", filterID, err)
	}
	return nil
}
