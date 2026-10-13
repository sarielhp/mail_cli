package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/mailclient"
)

// mockMailClient implementing MailClient just for GetMatchingLabels
type mockMailClient struct {
	mailclient.MailClient
	matchingLabels []string
	cfg            *cfg_g.Config
}

// InboxFolder is required because label resolution now maps the name "inbox"
// to the backend's own inbox folder. The embedded interface is nil, so an
// unimplemented method is a panic rather than a compile error -- which is what
// this mock traded for not having to implement all of MailClient.
func (m *mockMailClient) InboxFolder() label.Folder { return label.FromServer("INBOX") }

func (m *mockMailClient) GetMatchingLabels(prefix string) ([]string, error) {
	if prefix == "" {
		return m.matchingLabels, nil
	}
	var res []string
	for _, l := range m.matchingLabels {
		if strings.HasPrefix(l, prefix) {
			res = append(res, l)
		}
	}
	return res, nil
}

func (m *mockMailClient) Config() *cfg_g.Config {
	return m.cfg
}

func TestRulesAccountLevelAndResolution(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	// Set up initial config
	fc := &cfg_g.FileConfig{
		Accounts: &[]cfg_acc.AccountConfig{
			{
				Name:           "default",
				Type:           "gmail",
				Username:       "test@gmail.com",
				Password:       "pass",
				IMAPHost:       "imap.gmail.com",
				SpamFolder:     "Spam",
				ReceivedFolder: "INBOX",
				SpamLearn:      "Spam",
				Rules:          []cfg_acc.Rule{},
			},
		},
	}
	data, err := json.Marshal(fc)
	if err != nil {
		t.Fatalf("failed to marshal setup config: %v", err)
	}
	if writeErr := os.WriteFile(configPath, data, 0600); writeErr != nil {
		t.Fatalf("failed to write setup config: %v", writeErr)
	}

	config := &cfg_g.Config{SelectedAccount: "default", Paths: cfg_g.Paths{ConfigDir: tempDir}}

	mockClient := &mockMailClient{
		matchingLabels: []string{"INBOX", "Work/Projects/Antigravity", "Spam"},
	}

	// Both streams go to buffers: what the command reports about its work is
	// status, and status goes to Err.
	var out, status bytes.Buffer
	session := &Session{
		Config: config,
		Out:    &out,
		Err:    &status,
		Deps: Deps{
			Clients: stubClients{
				ForSelectedFn: func(_ *cfg_g.Config) (mailclient.MailClient, error) {
					return mockClient, nil
				},
			},
		},
	}

	// 1. Add rule with unresolved label suffix, check that it resolves and prints.
	// Resolution happens where the argument is read, as for every label argument.
	label, err := ruleLabelArg(session, []string{"%antigravity"}, 0)
	if err != nil {
		t.Fatalf("ruleLabelArg failed: %v", err)
	}
	err = addRuleToConfig(session, "spammy@sender.com", label, "")

	if err != nil {
		t.Fatalf("addRuleToConfig failed: %v", err)
	}

	outStr := out.String() + status.String()

	// Verify resolution output
	// ResolveLabel prints "[*] Using label: Work/Projects/Antigravity"
	if !strings.Contains(outStr, "Using label: Work/Projects/Antigravity") && !strings.Contains(outStr, "Work/Projects/Antigravity") {
		t.Errorf("output did not show using/adding Work/Projects/Antigravity: %s", outStr)
	}

	// Load config and check that the rule is under default account, not global
	loadedFc, err := cfg_g.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("failed to load saved config: %v", err)
	}

	if loadedFc.Rules != nil && len(*loadedFc.Rules) > 0 {
		t.Errorf("expected global rules to be nil, got: %v", loadedFc.Rules)
	}

	accs := *loadedFc.Accounts
	if len(accs) == 0 {
		t.Fatal("no accounts in saved config")
	}
	rules := accs[0].Rules
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule in account rules, got %d", len(rules))
	}
	if rules[0].Sender != "spammy@sender.com" {
		t.Errorf("expected sender 'spammy@sender.com', got '%s'", rules[0].Sender)
	}
	if rules[0].Label != "Work/Projects/Antigravity" {
		t.Errorf("expected resolved label 'Work/Projects/Antigravity', got '%s'", rules[0].Label)
	}

	// 2. Test listRules. The listing is the command's product, so it lands on
	// Out; nothing about it should need Err.
	out.Reset()
	status.Reset()
	if listErr := listRules(session); listErr != nil {
		t.Fatalf("listRules failed: %v", listErr)
	}
	outStr = out.String()
	if !strings.Contains(outStr, "spammy@sender.com -> Work/Projects/Antigravity") && !strings.Contains(outStr, "spammy@sender.com") {
		t.Errorf("listRules output did not contain rule. Output: %s", outStr)
	}

	// 2b. Test updating an existing rule (overwriting / rewriting)
	// First manually set Exported = true on the loaded config rule
	loadedFc, err = cfg_g.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("failed to load saved config: %v", err)
	}
	(*loadedFc.Accounts)[0].Rules[0].Exported = true
	data, _ = json.Marshal(loadedFc)
	_ = os.WriteFile(configPath, data, 0600)

	// Now add rule again but with a different label (e.g. INBOX)
	err = addRuleToConfig(session, "spammy@sender.com", "INBOX", "")
	if err != nil {
		t.Fatalf("updating rule failed: %v", err)
	}

	// Load and verify that the label is updated and Exported is set back to false
	loadedFc, err = cfg_g.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("failed to load config after update: %v", err)
	}
	updatedRules := (*loadedFc.Accounts)[0].Rules
	if len(updatedRules) != 1 {
		t.Fatalf("expected exactly 1 rule, got %d", len(updatedRules))
	}
	if updatedRules[0].Label != "INBOX" {
		t.Errorf("expected updated label to be 'INBOX', got '%s'", updatedRules[0].Label)
	}
	if updatedRules[0].Exported {
		t.Error("expected Exported to be reset to false after label update")
	}

	// 2c. Test deduplication when listing rules
	// Manually inject duplicate rules into config.json
	loadedFc, err = cfg_g.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("failed to load saved config: %v", err)
	}
	(*loadedFc.Accounts)[0].Rules = []cfg_acc.Rule{
		{Sender: "duplicate@sender.com", Label: "Label1"},
		{Sender: "other@sender.com", Label: "Label2"},
		{Sender: "duplicate@sender.com", Label: "Label3"}, // duplicate, keeping this one (last)
	}
	data, _ = json.Marshal(loadedFc)
	_ = os.WriteFile(configPath, data, 0600)

	// Listing must report duplicates without modifying anything: a read command
	// with a hidden write was the original defect.
	out.Reset()
	status.Reset()
	if listErr := listRules(session); listErr != nil {
		t.Fatalf("listRules with duplicates failed: %v", listErr)
	}
	// The warning is status, not part of the listing.
	outStr = status.String()

	if !strings.Contains(outStr, "duplicate rule(s) present") {
		t.Errorf("expected a duplicate-rule warning, got: %s", outStr)
	}
	if untouched, errLoad := cfg_g.LoadConfigFile(configPath); errLoad != nil {
		t.Fatalf("failed to reload config after listRules: %v", errLoad)
	} else if got := len((*untouched.Accounts)[0].Rules); got != 3 {
		t.Errorf("listRules must not modify the config; rule count changed from 3 to %d", got)
	}

	// 'rule tidy' is what performs the deduplication.
	out.Reset()
	status.Reset()
	if tidyErr := tidyRules(session); tidyErr != nil {
		t.Fatalf("tidyRules failed: %v", tidyErr)
	}
	if reported := out.String() + status.String(); !strings.Contains(reported, "Duplicate rule for 'duplicate@sender.com'") {
		t.Errorf("expected tidyRules to report the duplicate, got: %s", reported)
	}

	// Load and verify that only the last rule was kept and duplicates deleted
	loadedFc, err = cfg_g.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("failed to load config after deduplication: %v", err)
	}
	cleanedRules := (*loadedFc.Accounts)[0].Rules
	if len(cleanedRules) != 2 {
		t.Fatalf("expected 2 rules after deduplication, got %d", len(cleanedRules))
	}
	if cleanedRules[0].Sender != "other@sender.com" || cleanedRules[1].Sender != "duplicate@sender.com" {
		t.Errorf("unexpected rules: %+v", cleanedRules)
	}
	if cleanedRules[1].Label != "Label3" {
		t.Errorf("expected duplicate rule kept to have label 'Label3', got '%s'", cleanedRules[1].Label)
	}

	// 3. Delete rules
	err = deleteRuleFromConfig(session, "duplicate@sender.com", strings.NewReader(""))
	if err != nil {
		t.Fatalf("delete duplicate rule failed: %v", err)
	}
	err = deleteRuleFromConfig(session, "other@sender.com", strings.NewReader(""))
	if err != nil {
		t.Fatalf("delete other rule failed: %v", err)
	}

	loadedFc, err = cfg_g.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if len((*loadedFc.Accounts)[0].Rules) != 0 {
		t.Errorf("expected 0 rules after deletion, got %d", len((*loadedFc.Accounts)[0].Rules))
	}
}

func TestDeleteRuleByIndex(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	fc := &cfg_g.FileConfig{
		Accounts: &[]cfg_acc.AccountConfig{
			{
				Name: "default",
				Type: "gmail",
				Rules: []cfg_acc.Rule{
					{Sender: "rule1@sender.com", Label: "Label1"},
					{Sender: "rule2@sender.com", Label: "Label2", Exported: true},
					{Sender: "rule3@sender.com", Label: "Label3"},
				},
			},
		},
	}
	data, _ := json.Marshal(fc)
	_ = os.WriteFile(configPath, data, 0600)

	config := &cfg_g.Config{SelectedAccount: "default", Paths: cfg_g.Paths{ConfigDir: tempDir}}
	session := &Session{
		Config: config,
	}

	// Test cancellation (inputting "n")
	err := deleteRuleFromConfig(session, "3", strings.NewReader("n\n"))
	if err != nil {
		t.Fatalf("unexpected error on cancel: %v", err)
	}

	// Verify not deleted
	loadedFc, _ := cfg_g.LoadConfigFile(configPath)
	if len((*loadedFc.Accounts)[0].Rules) != 3 {
		t.Errorf("expected 3 rules, got %d", len((*loadedFc.Accounts)[0].Rules))
	}

	// Test confirm deletion of Rule 3 (which is index 2)
	err = deleteRuleFromConfig(session, "3", strings.NewReader("y\n"))
	if err != nil {
		t.Fatalf("delete rule by index failed: %v", err)
	}

	// Verify deleted
	loadedFc, _ = cfg_g.LoadConfigFile(configPath)
	rules := (*loadedFc.Accounts)[0].Rules
	if len(rules) != 2 {
		t.Errorf("expected 2 rules, got %d", len(rules))
	}
	if rules[0].Sender != "rule1@sender.com" || rules[1].Sender != "rule2@sender.com" {
		t.Errorf("unexpected remaining rules: %+v", rules)
	}
}

func TestDeleteInternalRuleByEmail(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	fc := &cfg_g.FileConfig{
		Accounts: &[]cfg_acc.AccountConfig{
			{
				Name: "default",
				Type: "gmail",
				Rules: []cfg_acc.Rule{
					{Sender: "foo@bar.com", Label: "Label1", Internal: true},
					{Sender: "John Doe <trigger@example.com>", Label: "Label2", Internal: true},
				},
			},
		},
	}
	data, _ := json.Marshal(fc)
	_ = os.WriteFile(configPath, data, 0600)

	config := &cfg_g.Config{SelectedAccount: "default", Paths: cfg_g.Paths{ConfigDir: tempDir}}
	session := &Session{
		Config: config,
	}

	// Delete internal rule by email address trigger
	err := deleteRuleFromConfig(session, "trigger@example.com", strings.NewReader(""))
	if err != nil {
		t.Fatalf("unexpected error deleting rule by email: %v", err)
	}

	loadedFc, _ := cfg_g.LoadConfigFile(configPath)
	rules := (*loadedFc.Accounts)[0].Rules
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule remaining, got %d", len(rules))
	}
	if rules[0].Sender != "foo@bar.com" {
		t.Errorf("expected remaining rule for foo@bar.com, got %s", rules[0].Sender)
	}
}

func TestRuleAddInvalidEmail(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	fc := &cfg_g.FileConfig{
		Accounts: &[]cfg_acc.AccountConfig{
			{
				Name: "default",
				Type: "gmail",
			},
		},
	}
	data, _ := json.Marshal(fc)
	_ = os.WriteFile(configPath, data, 0600)

	config := &cfg_g.Config{SelectedAccount: "default", Paths: cfg_g.Paths{ConfigDir: tempDir}}
	session := &Session{
		Config: config,
	}

	err := addRuleToConfig(session, "invalid-email", "Label", "")
	if err == nil {
		t.Fatalf("expected error adding invalid email, got nil")
	}
	if !strings.Contains(err.Error(), "not a valid email address") {
		t.Errorf("expected invalid email error message, got: %v", err)
	}
}

func TestCanonicalRuleAndCalCommands(t *testing.T) {
	session := &Session{Config: &cfg_g.Config{}}
	delCmd := newRuleDelCmd(session)
	if len(delCmd.Aliases) != 1 || delCmd.Aliases[0] != "del" {
		t.Errorf("expected newRuleDelCmd aliases to be ['del'], got %v", delCmd.Aliases)
	}

	delAllCmd := newRuleDeleteAllCmd(session)
	if len(delAllCmd.Aliases) != 0 {
		t.Errorf("expected newRuleDeleteAllCmd aliases to be empty, got %v", delAllCmd.Aliases)
	}

	addTitleCmd := newRuleAddByTitleCmd(session)
	if len(addTitleCmd.Aliases) != 0 {
		t.Errorf("expected newRuleAddByTitleCmd aliases to be empty, got %v", addTitleCmd.Aliases)
	}

	// Verify hidden subcommands for backward compatibility
	ruleCmd := RuleCmd(session)
	hiddenSubcmds := make(map[string]bool)
	for _, sub := range ruleCmd.Subcommands {
		if sub.Hidden {
			hiddenSubcmds[sub.Name] = true
		}
	}
	for _, expectedHidden := range []string{"add-by-title", "delete-all", "del-all", "delete"} {
		if !hiddenSubcmds[expectedHidden] {
			t.Errorf("expected hidden backward-compat subcommand %q in RuleCmd", expectedHidden)
		}
	}
}

func TestRuleCmd_ExamplesUseCanonicalSubcommands(t *testing.T) {
	session := &Session{Config: &cfg_g.Config{}}
	ruleCmd := RuleCmd(session)
	subcmdMap := make(map[string]bool)
	for _, sub := range ruleCmd.Subcommands {
		subcmdMap[sub.Name] = true
		for _, alias := range sub.Aliases {
			subcmdMap[alias] = true
		}
	}

	for _, ex := range ruleCmd.Examples {
		parts := strings.Fields(ex.Line)
		if len(parts) >= 3 && parts[0] == "mail_cli" && parts[1] == "rule" {
			sub := parts[2]
			if !subcmdMap[sub] {
				t.Errorf("example %q uses non-existent or invalid subcommand %q", ex.Line, sub)
			}
		}
	}
}

func setupRemoteRuleTest(t *testing.T, localRules []cfg_acc.Rule, remoteFilters []mailclient.Filter) (*Session, *mailclient.MockMailClient, string) {
	t.Helper()
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	fc := &cfg_g.FileConfig{
		Accounts: &[]cfg_acc.AccountConfig{
			{
				Name:           "default",
				Type:           "gmail",
				Username:       "test@gmail.com",
				Password:       "pass",
				IMAPHost:       "imap.gmail.com",
				SpamFolder:     "Spam",
				ReceivedFolder: "INBOX",
				Rules:          localRules,
			},
		},
	}
	data, err := json.Marshal(fc)
	if err != nil {
		t.Fatalf("failed to marshal setup config: %v", err)
	}
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatalf("failed to write setup config: %v", err)
	}

	config := &cfg_g.Config{SelectedAccount: "default", Paths: cfg_g.Paths{ConfigDir: tempDir}}
	mockClient := &mailclient.MockMailClient{
		Filters: mailclient.FilterListing{
			Title:   "Remote Filters",
			Filters: remoteFilters,
		},
	}

	var out, status bytes.Buffer
	session := &Session{
		Config: config,
		Out:    &out,
		Err:    &status,
		Deps: Deps{
			Clients: stubClients{
				ForSelectedFn: func(_ *cfg_g.Config) (mailclient.MailClient, error) {
					return mockClient, nil
				},
			},
		},
	}
	return session, mockClient, configPath
}

func TestRuleRemoteRm_ByIndex(t *testing.T) {
	localRules := []cfg_acc.Rule{
		{Sender: "alerts@github.com", Label: "Dev/GitHub", Exported: true},
	}
	remoteFilters := []mailclient.Filter{
		{
			ID:          "Filter ID: AN63b...",
			RemoteID:    "AN63b_full_123",
			Sender:      "alerts@github.com",
			Action:      "Apply Label: Dev/GitHub",
			TargetLabel: "Dev/GitHub",
		},
	}
	session, mock, _ := setupRemoteRuleTest(t, localRules, remoteFilters)

	err := deleteRemoteRule(session, "1", true, false)
	if err != nil {
		t.Fatalf("deleteRemoteRule failed: %v", err)
	}

	if len(mock.DeletedFilters) != 1 || mock.DeletedFilters[0] != "AN63b_full_123" {
		t.Errorf("expected deleted filter AN63b_full_123, got: %v", mock.DeletedFilters)
	}

	resolved, err := cfg_g.Resolve(session.Config)
	if err != nil {
		t.Fatalf("failed to resolve config: %v", err)
	}
	if len(resolved.Account.Rules) != 0 {
		t.Errorf("expected matching local rule to be removed, got: %v", resolved.Account.Rules)
	}
}

func TestRuleRemoteRm_KeepLocal(t *testing.T) {
	localRules := []cfg_acc.Rule{
		{Sender: "alerts@github.com", Label: "Dev/GitHub", Exported: true},
	}
	remoteFilters := []mailclient.Filter{
		{
			ID:          "Filter ID: AN63b...",
			RemoteID:    "AN63b_full_123",
			Sender:      "alerts@github.com",
			TargetLabel: "Dev/GitHub",
		},
	}
	session, mock, _ := setupRemoteRuleTest(t, localRules, remoteFilters)

	err := deleteRemoteRule(session, "1", true, true)
	if err != nil {
		t.Fatalf("deleteRemoteRule failed: %v", err)
	}

	if len(mock.DeletedFilters) != 1 || mock.DeletedFilters[0] != "AN63b_full_123" {
		t.Errorf("expected deleted filter AN63b_full_123, got: %v", mock.DeletedFilters)
	}

	resolved, err := cfg_g.Resolve(session.Config)
	if err != nil {
		t.Fatalf("failed to resolve config: %v", err)
	}
	if len(resolved.Account.Rules) != 1 {
		t.Fatalf("expected local rule preserved, got: %v", resolved.Account.Rules)
	}
	if resolved.Account.Rules[0].Exported {
		t.Errorf("expected Exported to be false when --keep-local used")
	}
}

func TestRuleRemoteRm_ByPrefixAndQuery(t *testing.T) {
	localRules := []cfg_acc.Rule{}
	remoteFilters := []mailclient.Filter{
		{
			ID:       "Filter ID: AN63b...",
			RemoteID: "AN63b9f4812",
			Sender:   "bot@service.com",
		},
	}
	session, mock, _ := setupRemoteRuleTest(t, localRules, remoteFilters)

	err := deleteRemoteRule(session, "AN63b", true, false)
	if err != nil {
		t.Fatalf("delete by prefix failed: %v", err)
	}
	if len(mock.DeletedFilters) != 1 || mock.DeletedFilters[0] != "AN63b9f4812" {
		t.Errorf("unexpected deleted filter: %v", mock.DeletedFilters)
	}

	mock.DeletedFilters = nil
	err = deleteRemoteRule(session, "bot@service.com", true, false)
	if err != nil {
		t.Fatalf("delete by sender query failed: %v", err)
	}
	if len(mock.DeletedFilters) != 1 || mock.DeletedFilters[0] != "AN63b9f4812" {
		t.Errorf("unexpected deleted filter: %v", mock.DeletedFilters)
	}
}

func TestRuleRemoteRm_AmbiguousAndNotFound(t *testing.T) {
	localRules := []cfg_acc.Rule{}
	remoteFilters := []mailclient.Filter{
		{RemoteID: "AN63b_alpha", Sender: "dupe@test.com"},
		{RemoteID: "AN63b_beta", Sender: "dupe@test.com"},
	}
	session, _, _ := setupRemoteRuleTest(t, localRules, remoteFilters)

	// Ambiguous prefix
	err := deleteRemoteRule(session, "AN63b", true, false)
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("expected ambiguous error for prefix, got: %v", err)
	}

	// Ambiguous sender query
	err = deleteRemoteRule(session, "dupe@test.com", true, false)
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("expected ambiguous error for sender query, got: %v", err)
	}

	// Not found
	err = deleteRemoteRule(session, "nonexistent@test.com", true, false)
	if err == nil || !strings.Contains(err.Error(), "no remote filter found") {
		t.Errorf("expected not found error, got: %v", err)
	}
}

func TestRuleRemoteRm_DryRun(t *testing.T) {
	localRules := []cfg_acc.Rule{
		{Sender: "alerts@github.com", Label: "Dev/GitHub", Exported: true},
	}
	remoteFilters := []mailclient.Filter{
		{
			ID:          "Filter ID: AN63b...",
			RemoteID:    "AN63b_full_123",
			Sender:      "alerts@github.com",
			TargetLabel: "Dev/GitHub",
		},
	}
	session, mock, _ := setupRemoteRuleTest(t, localRules, remoteFilters)
	session.Config.DryRun = true

	err := deleteRemoteRule(session, "1", true, false)
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}

	if len(mock.DeletedFilters) != 0 {
		t.Errorf("expected no server deletion in dry run mode, got: %v", mock.DeletedFilters)
	}

	resolved, err := cfg_g.Resolve(session.Config)
	if err != nil {
		t.Fatalf("failed to resolve config: %v", err)
	}
	if len(resolved.Account.Rules) != 1 {
		t.Errorf("expected local rule untouched in dry run mode, got: %v", resolved.Account.Rules)
	}
}
