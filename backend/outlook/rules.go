package outlook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/sarielhp/mail_cli/label"
	"io"
	"mime"
	"net/http"
	"net/mail"
	"strings"

	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_acc"
	myme "github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
)

type graphRule struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Sequence    int    `json:"sequence"`
	IsEnabled   bool   `json:"isEnabled"`
	Conditions  struct {
		SenderContains  []string `json:"senderContains,omitempty"`
		SubjectContains []string `json:"subjectContains,omitempty"`
	} `json:"conditions"`
	Actions struct {
		MoveToFolder string `json:"moveToFolder,omitempty"`
	} `json:"actions"`
}

func (c *client) CheckAndApplyRules(messageIDs []myme.ID, source label.Folder, _ string) ([]myme.ID, error) {
	if len(messageIDs) == 0 {
		return messageIDs, nil
	}
	if err := c.init(); err != nil {
		return nil, err
	}

	var remainingIDs []myme.ID
	dec := new(mime.WordDecoder)

	for _, id := range messageIDs {
		data, rErr := msg.Read(c.config.AccountCache, id)
		if rErr != nil {
			remainingIDs = append(remainingIDs, id)
			continue
		}

		localEmail, errParse := mail.ReadMessage(bytes.NewReader(data))
		if errParse != nil {
			remainingIDs = append(remainingIDs, id)
			continue
		}

		fromHeader := localEmail.Header.Get("From")
		sender := myme.ParseEmailAddress(fromHeader)
		subject := myme.DecodeHeader(dec, localEmail.Header.Get("Subject"))

		if matchedRule := cfg_acc.MatchRules(c.config.Rules, sender, subject); matchedRule != nil {
			if c.config.Verbose {
				c.rep.Infof("Outlook Rule match: moving %s to %q\n", id, matchedRule.Label)
			}
			errMove := c.MoveEmail([]myme.ID{id}, source, label.FromServer(matchedRule.Label))
			if errMove != nil {
				remainingIDs = append(remainingIDs, id)
			}
		} else {
			remainingIDs = append(remainingIDs, id)
		}
	}

	return remainingIDs, nil
}

func (c *client) GetFilters() (mailclient.FilterListing, error) {
	listing := mailclient.FilterListing{Title: "REMOTE OUTLOOK FILTERS"}

	if err := c.init(); err != nil {
		return listing, err
	}

	resp, err := c.client.Get("https://graph.microsoft.com/v1.0/me/mailFolders/inbox/messagerules")
	if err != nil {
		return listing, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return listing, fmt.Errorf("list filters failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	var res struct {
		Value []graphRule `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return listing, err
	}

	for _, r := range res.Value {
		status := "disabled"
		if r.IsEnabled {
			status = "enabled"
		}
		var conds []string
		if len(r.Conditions.SenderContains) > 0 {
			conds = append(conds, "From: "+strings.Join(r.Conditions.SenderContains, ", "))
		}
		if len(r.Conditions.SubjectContains) > 0 {
			conds = append(conds, "Subject: "+strings.Join(r.Conditions.SubjectContains, ", "))
		}
		match := strings.Join(conds, " AND ")
		if match == "" {
			match = "No conditions"
		}
		listing.Filters = append(listing.Filters, mailclient.Filter{
			ID:          r.DisplayName,
			RemoteID:    r.ID,
			Status:      status,
			Match:       match,
			Action:      "Move to folder ID " + r.Actions.MoveToFolder,
			Sender:      strings.Join(r.Conditions.SenderContains, ", "),
			Subject:     strings.Join(r.Conditions.SubjectContains, ", "),
			TargetLabel: r.Actions.MoveToFolder,
		})
	}
	return listing, nil
}

func (c *client) ExportRules() error {
	if err := c.init(); err != nil {
		return err
	}

	resp, err := c.client.Get("https://graph.microsoft.com/v1.0/me/mailFolders/inbox/messagerules")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b := readErrorBody(resp.Body)
		return fmt.Errorf("fetching rules failed: status %d: %s", resp.StatusCode, b)
	}

	var res struct {
		Value []graphRule `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return fmt.Errorf("decoding rules failed: %w", err)
	}

	if err := c.deleteExistingMailCliRules(res.Value); err != nil {
		return err
	}

	for i, rule := range c.config.Rules {
		if rule.Exported && !c.config.ForceLearn {
			continue
		}
		if err := c.exportSingleRule(rule, i); err != nil {
			return err
		}
	}

	c.rep.Successf("Successfully exported local rules to Outlook.\n")
	return nil
}

func (c *client) deleteExistingMailCliRules(rules []graphRule) error {
	for _, r := range rules {
		if !strings.HasPrefix(r.DisplayName, "mail_cli_rule_") {
			continue
		}
		url := fmt.Sprintf("https://graph.microsoft.com/v1.0/me/mailFolders/inbox/messagerules/%s", r.ID)
		req, err := http.NewRequest(http.MethodDelete, url, nil)
		if err != nil {
			return err
		}
		dResp, dErr := c.client.Do(req)
		if dErr != nil {
			return fmt.Errorf("deleting rule %s failed: %w", r.ID, dErr)
		}
		if dResp.StatusCode != http.StatusNoContent && dResp.StatusCode != http.StatusOK {
			b := readErrorBody(dResp.Body)
			_ = dResp.Body.Close()
			return fmt.Errorf("deleting rule %s failed: status %d: %s", r.ID, dResp.StatusCode, b)
		}
		_ = dResp.Body.Close()
	}
	return nil
}

func (c *client) exportSingleRule(rule cfg_acc.Rule, index int) error {
	destFolderID, err := c.getFolderID(rule.Label)
	if err != nil {
		_ = c.EnsureLabelExists(label.FromServer(rule.Label))
		destFolderID, _ = c.getFolderID(rule.Label)
	}
	if destFolderID == "" {
		return nil
	}

	payload := map[string]interface{}{
		"displayName": fmt.Sprintf("mail_cli_rule_%d", index+1),
		"sequence":    index + 1,
		"isEnabled":   true,
		"actions": map[string]interface{}{
			"moveToFolder": destFolderID,
		},
	}

	conditions := make(map[string]interface{})
	if rule.Sender != "" {
		conditions["senderContains"] = []string{rule.Sender}
	} else if rule.Subject != "" {
		conditions["subjectContains"] = []string{rule.Subject}
	}
	payload["conditions"] = conditions

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal rule %d: %w", index+1, err)
	}

	cResp, cErr := c.client.Post("https://graph.microsoft.com/v1.0/me/mailFolders/inbox/messagerules", "application/json", bytes.NewReader(bodyBytes))
	if cErr != nil {
		return fmt.Errorf("failed to export rule %d: %w", index+1, cErr)
	}
	if cResp.StatusCode != http.StatusCreated && cResp.StatusCode != http.StatusOK {
		b := readErrorBody(cResp.Body)
		_ = cResp.Body.Close()
		return fmt.Errorf("failed to export rule %d: status %d: %s", index+1, cResp.StatusCode, b)
	}
	_ = cResp.Body.Close()
	return nil
}

func (c *client) DeleteFilter(id string) error {
	if err := c.init(); err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodDelete, "https://graph.microsoft.com/v1.0/me/mailFolders/inbox/messagerules/"+id, nil)
	if err != nil {
		return err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		b := readErrorBody(resp.Body)
		return fmt.Errorf("failed to delete Outlook rule %s: status %d: %s", id, resp.StatusCode, b)
	}
	return nil
}
