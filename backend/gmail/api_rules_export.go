package gmail

import (
	"fmt"
	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/report"
	"strings"

	gmailapi "google.golang.org/api/gmail/v1"
)

func matchCasedLabel(parts []string, gmailLabels map[string]string) ([]string, bool) {
	rewritten := false
	result := make([]string, len(parts))
	copy(result, parts)
	for j := 1; j <= len(result); j++ {
		prefix := strings.Join(result[:j], "/")
		exactName, ok := gmailLabels[strings.ToLower(prefix)]
		if !ok {
			continue
		}
		exactParts := strings.Split(exactName, "/")
		for idx, val := range exactParts {
			if idx < len(result) && result[idx] != val {
				result[idx] = val
				rewritten = true
			}
		}
	}
	return result, rewritten
}

func syncRuleLabelCasing(rep report.Reporter, config *Config, gmailLabels map[string]string) {
	configRulesUpdated := false
	for i, rule := range config.Rules {
		parts := strings.Split(rule.Label, "/")
		newParts, rewritten := matchCasedLabel(parts, gmailLabels)
		if !rewritten {
			continue
		}
		newLabel := strings.Join(newParts, "/")
		if newLabel != rule.Label {
			rep.Infof("Casing mismatch detected for rule '%s': Rewriting local config label %q to %q to match existing Gmail folders.\n", rule.Sender, rule.Label, newLabel)
			config.Rules[i].Label = newLabel
			configRulesUpdated = true
		}
	}

	if configRulesUpdated {
		rep.Infof("Saving updated rule labels with corrected casing to config.json...\n")
		if err := updateLocalRulesInConfig(config, config.Rules); err != nil {
			rep.Warnf("Warning: Failed to save corrected rules to config.json: %v\n", err)
		} else {
			rep.Successf("Successfully saved updated rules to config.json\n")
		}
	}
}

func findExistingRuleFilter(filters []*gmailapi.Filter, r rule) *gmailapi.Filter {
	cleanSubject := strings.ToLower(strings.TrimSpace(r.Subject))
	cleanSender := strings.ToLower(strings.TrimSpace(r.Sender))
	if cleanSubject == "" && cleanSender == "" {
		return nil
	}

	for _, f := range filters {
		if f.Criteria == nil {
			continue
		}
		matchSubject := cleanSubject == "" || strings.ToLower(strings.TrimSpace(f.Criteria.Subject)) == cleanSubject
		matchSender := cleanSender == "" || strings.ToLower(strings.TrimSpace(f.Criteria.From)) == cleanSender
		if matchSubject && matchSender {
			return f
		}
	}
	return nil
}

func buildRuleFilter(rule rule, labelID string) *gmailapi.Filter {
	criteria := &gmailapi.FilterCriteria{}
	if rule.Subject != "" {
		criteria.Subject = rule.Subject
	}
	if rule.Sender != "" {
		criteria.From = rule.Sender
	}
	return &gmailapi.Filter{
		Criteria: criteria,
		Action: &gmailapi.FilterAction{
			AddLabelIds:    []string{labelID},
			RemoveLabelIds: []string{"INBOX"},
		},
	}
}

func filterHasLabel(action *gmailapi.FilterAction, labelID string) bool {
	if action == nil {
		return false
	}
	for _, addID := range action.AddLabelIds {
		if addID == labelID {
			return true
		}
	}
	return false
}

func forceOverwriteRemoteFilter(rep report.Reporter, srv *gmailapi.Service, config *Config, rule *rule, ruleDesc string, existingID string, labelID string) (bool, error) {
	rep.Infof("Conflict found for '%s'. Force overwriting existing remote filter...\n", ruleDesc)
	if errDel := srv.Users.Settings.Filters.Delete("me", existingID).Do(); errDel != nil {
		rep.Warnf("Warning: Failed to delete existing remote filter on Gmail: %v\n", errDel)
		rule.Exported = false
		return false, nil
	}
	filter := buildRuleFilter(*rule, labelID)
	_, errCreate := srv.Users.Settings.Filters.Create("me", filter).Do()
	if errCreate != nil {
		if IsInsufficientScopeError(errCreate) {
			return false, HandleScopeError(rep, config, errCreate)
		}
		rep.Warnf("Warning: Failed to recreate filter for '%s' on Gmail: %v\n", ruleDesc, errCreate)
		rule.Exported = false
		return false, nil
	}
	rep.Successf("Successfully overwrote Gmail server filter for '%s' -> label %q (archives automatically)\n", ruleDesc, rule.Label)
	rule.Exported = true
	return true, nil
}

func exportSingleRule(rep report.Reporter, srv *gmailapi.Service, config *Config, rule *rule, labelNameToID, labelIDToName map[string]string, filters []*gmailapi.Filter) (bool, bool, error) {
	ruleDesc := rule.Sender
	if rule.Subject != "" {
		ruleDesc = fmt.Sprintf("subject:%s", rule.Subject)
	}

	if rule.Exported && !config.ForceLearn {
		if config.Verbose {
			rep.Successf("    rule for '%s' -> label %q is already marked as exported. Skipping.\n", ruleDesc, rule.Label)
		}
		return false, true, nil
	}

	labelID, errLabel := ensureLabelHierarchyREST(rep, srv, rule.Label, labelNameToID, labelIDToName)
	if errLabel != nil {
		if IsInsufficientScopeError(errLabel) {
			return false, false, HandleScopeError(rep, config, errLabel)
		}
		rep.Warnf("Warning: Failed to ensure label %q for rule '%s': %v\n", rule.Label, ruleDesc, errLabel)
		rule.Exported = false
		return false, false, nil
	}

	existing := findExistingRuleFilter(filters, *rule)
	if existing != nil {
		if filterHasLabel(existing.Action, labelID) {
			if config.Verbose {
				rep.Successf("    rule for '%s' -> label %q already exists on Gmail. Skipping.\n", ruleDesc, rule.Label)
			}
			rule.Exported = true
			return false, true, nil
		}
		if config.ForceLearn {
			overwritten, err := forceOverwriteRemoteFilter(rep, srv, config, rule, ruleDesc, existing.Id, labelID)
			if err != nil {
				return false, false, err
			}
			if overwritten {
				return true, false, nil
			}
		}
		remoteActionDesc := describeFilterAction(existing.Action, labelIDToName)
		rep.Warnf("Filter for '%s' already exists on Gmail with different action (%s). Skipping to avoid conflict.\n", ruleDesc, remoteActionDesc)
		rep.Printf("    -> Remote filter action: %s\n", remoteActionDesc)
		rep.Printf("    -> Local rule suggests:  Add label %q and archive (remove INBOX)\n", rule.Label)
		rep.Printf("    -> To overwrite conflicting remote filters, run 'mail_cli rule export force' or use '--force'.\n")
		rule.Exported = false
		return false, true, nil
	}

	filter := buildRuleFilter(*rule, labelID)
	_, errCreate := srv.Users.Settings.Filters.Create("me", filter).Do()
	if errCreate != nil {
		if IsInsufficientScopeError(errCreate) {
			return false, false, HandleScopeError(rep, config, errCreate)
		}
		rep.Warnf("Warning: Failed to create filter for '%s' on Gmail: %v\n", ruleDesc, errCreate)
		rule.Exported = false
		return false, false, nil
	}

	rep.Successf("Successfully created Gmail server filter for '%s' -> label %q (archives automatically)\n", ruleDesc, rule.Label)
	rule.Exported = true
	return true, false, nil
}

// exportRulesToGmailREST exports local rules from config.json to Gmail filters.
func exportRulesToGmailREST(rep report.Reporter, config *Config) error {
	if len(config.Rules) == 0 {
		rep.Infof("No rules defined in config.json. Nothing to export.\n")
		return nil
	}

	srv, err := GetGmailService(rep, config)
	if err != nil {
		return err
	}

	rep.Infof("Fetching existing labels from Gmail...\n")
	labelsRes, err := srv.Users.Labels.List("me").Do()
	if err != nil {
		return HandleScopeError(rep, config, fmt.Errorf("failed to list Gmail labels: %w", err))
	}

	gmailLabels := make(map[string]string)
	labelNameToID := make(map[string]string)
	labelIDToName := make(map[string]string)
	for _, l := range labelsRes.Labels {
		lowerName := strings.ToLower(l.Name)
		gmailLabels[lowerName] = l.Name
		labelNameToID[lowerName] = l.Id
		labelIDToName[l.Id] = l.Name
	}

	syncRuleLabelCasing(rep, config, gmailLabels)

	rep.Infof("Fetching existing filters from Gmail...\n")
	filtersRes, err := srv.Users.Settings.Filters.List("me").Do()
	if err != nil {
		return HandleScopeError(rep, config, fmt.Errorf("failed to list Gmail filters: %w", err))
	}

	rep.Infof("Processing %d local rules...\n", len(config.Rules))
	createdCount := 0
	skippedCount := 0
	configRulesStatusChanged := false

	for i := range config.Rules {
		rule := &config.Rules[i]
		if rule.Internal || (rule.Sender == "" && rule.Subject == "") || rule.Label == "" {
			continue
		}
		origExported := rule.Exported
		created, skipped, err := exportSingleRule(rep, srv, config, rule, labelNameToID, labelIDToName, filtersRes.Filter)
		if err != nil {
			return err
		}
		if rule.Exported != origExported {
			configRulesStatusChanged = true
		}
		if created {
			createdCount++
		}
		if skipped {
			skippedCount++
		}
	}

	if configRulesStatusChanged {
		rep.Infof("Saving updated rule export statuses to config.json...\n")
		if errSave := updateLocalRulesInConfig(config, config.Rules); errSave != nil {
			rep.Warnf("Warning: Failed to save updated rules to config.json: %v\n", errSave)
		} else {
			rep.Successf("Successfully saved updated rules to config.json\n")
		}
	}

	rep.Printf("\n%s Export finished. Created %d new filters, skipped %d rules.\n", app.PrefixSuccess, createdCount, skippedCount)
	return nil
}

func describeFilterAction(action *gmailapi.FilterAction, labelIDToName map[string]string) string {
	if action == nil {
		return "no action defined"
	}
	var parts []string

	if len(action.AddLabelIds) > 0 {
		parts = append(parts, fmt.Sprintf("Add label(s): %s", strings.Join(formatFilterActionLabels(action.AddLabelIds, labelIDToName), ", ")))
	}

	if len(action.RemoveLabelIds) > 0 {
		parts = append(parts, fmt.Sprintf("Remove label(s): %s", strings.Join(formatFilterActionLabels(action.RemoveLabelIds, labelIDToName), ", ")))
	}

	if action.Forward != "" {
		parts = append(parts, fmt.Sprintf("Forward to: %s", action.Forward))
	}

	if len(parts) == 0 {
		return "other action"
	}
	return strings.Join(parts, "; ")
}

// updateLocalRulesInConfig saves the rules slice back to ~/.config/mail_cli/config.json
