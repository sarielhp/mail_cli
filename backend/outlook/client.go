package outlook

import (
	"fmt"
	"github.com/sarielhp/mail_cli/label"
	"io"
	"net/http"
	"strings"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/report"
)

const maxErrorBodyBytes = 4096

func readErrorBody(r io.Reader) string {
	b, _ := io.ReadAll(io.LimitReader(r, maxErrorBodyBytes))
	return string(b)
}

type client struct {
	config  *cfg_g.Config
	account cfg_acc.AccountConfig
	client  *http.Client
	rep     report.Reporter
}

// NewClient builds a client that reports progress through rep. A nil rep
// discards, which suits a caller that has nowhere to show it.
func NewClient(acc cfg_acc.AccountConfig, config *cfg_g.Config, rep report.Reporter) mailclient.MailClient {
	return &client{
		config:  config,
		account: acc,
		rep:     report.Or(rep),
	}
}

func (c *client) init() error {
	if c.client != nil {
		return nil
	}
	c.client = getOutlookClient(c.config, c.account.Name, c.rep)
	return nil
}

func (c *client) Validate() error {
	if err := cfg_g.ValidateAccountParams(c.account); err != nil {
		return err
	}
	if err := c.init(); err != nil {
		return err
	}

	resp, err := c.client.Get("https://graph.microsoft.com/v1.0/me/mailFolders")
	if err != nil {
		return fmt.Errorf("outlook validation request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("outlook API returned status code %d", resp.StatusCode)
	}

	return nil
}

func (c *client) Config() *cfg_g.Config {
	return c.config
}

func (c *client) inboxName() string {
	return "Inbox"
}

func (c *client) RewriteRuleLabelCasing(label string) string {
	if strings.EqualFold(label, "inbox") {
		return "Inbox"
	}
	return label
}

func (c *client) BackendType() string { return "outlook" }

// InboxFolder is the account's inbox, as the server names it.
func (c *client) InboxFolder() label.Folder {
	return label.FromServer(c.inboxName())
}
