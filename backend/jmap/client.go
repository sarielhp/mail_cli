package jmap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	mailemail "github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/cache"
	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/report"

	"git.sr.ht/~rockorager/go-jmap"
	jmapMail "git.sr.ht/~rockorager/go-jmap/mail"
	"git.sr.ht/~rockorager/go-jmap/mail/email"
	"git.sr.ht/~rockorager/go-jmap/mail/identity"
	"git.sr.ht/~rockorager/go-jmap/mail/mailbox"
)

type client struct {
	config        *cfg_g.Config
	account       cfg_acc.AccountConfig
	client        *jmap.Client
	accID         jmap.ID
	mailboxID     map[string]jmap.ID
	sentMailboxID jmap.ID
	rep           report.Reporter
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
	return c.initContext(context.Background())
}

func (c *client) initContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.client != nil {
		return nil
	}

	sessionURL := c.account.SessionURL
	if sessionURL == "" {
		sessionURL = c.account.IMAPHost
	}
	if sessionURL == "" {
		return fmt.Errorf("no JMAP session URL is configured for account %s", c.account.Name)
	}

	client := &jmap.Client{
		SessionEndpoint: sessionURL,
	}
	client.WithAccessToken(c.account.Password)
	if client.HttpClient == nil {
		client.HttpClient = &http.Client{}
	}
	client.HttpClient.Timeout = 60 * time.Second
	if err := authenticateContext(ctx, client); err != nil {
		return fmt.Errorf("JMAP authentication failed for %s: %w", c.account.Username, err)
	}

	c.client = client
	c.accID = client.Session.PrimaryAccounts[jmapMail.URI]

	if err := c.loadMailboxesContext(ctx); err != nil {
		return err
	}

	return nil
}

func authenticateContext(ctx context.Context, client *jmap.Client) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	client.Lock()
	endpoint := client.SessionEndpoint
	client.Unlock()
	if endpoint == "" {
		return fmt.Errorf("no session url is set")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return err
	}

	resp, err := client.HttpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("couldn't authenticate (status %d)", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	s := &jmap.Session{}
	if err := json.Unmarshal(data, s); err != nil {
		return err
	}

	client.Lock()
	client.Session = s
	client.Unlock()

	return nil
}

func (c *client) loadMailboxes() error {
	return c.loadMailboxesContext(context.Background())
}

func (c *client) loadMailboxesContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	req := &jmap.Request{Context: ctx}
	req.Invoke(&mailbox.Get{
		Account: c.accID,
	})
	resp, err := executeJMAPOpWithRetry(ctx, "load mailboxes", func() (*jmap.Response, error) {
		return c.client.Do(req)
	})
	if err != nil {
		return fmt.Errorf("failed to retrieve mailboxes: %w", err)
	}

	var mbs []*mailbox.Mailbox
	for _, inv := range resp.Responses {
		if r, ok := inv.Args.(*mailbox.GetResponse); ok {
			mbs = r.List
		}
	}

	c.mailboxID = make(map[string]jmap.ID)
	getFullPath := mailboxFullPath(mbs)

	c.sentMailboxID = ""
	for _, mb := range mbs {
		fullPath := getFullPath(mb)
		c.mailboxID[fullPath] = mb.ID

		if strings.EqualFold(string(mb.Role), "sent") {
			c.sentMailboxID = mb.ID
		}
	}

	if c.sentMailboxID == "" {
		for _, mb := range mbs {
			nameLower := strings.ToLower(mb.Name)
			if nameLower == "sent" || nameLower == "sent items" || nameLower == "sent messages" {
				c.sentMailboxID = mb.ID
				break
			}
		}
	}

	return nil
}

func (c *client) getMailboxID(name string) (jmap.ID, error) {
	if err := c.init(); err != nil {
		return "", err
	}

	if id, ok := c.findMailboxID(name); ok {
		return id, nil
	}

	if err := c.loadMailboxes(); err == nil {
		if id, ok := c.findMailboxID(name); ok {
			return id, nil
		}
	}

	return "", fmt.Errorf("JMAP mailbox not found: %s", name)
}

func (c *client) findMailboxID(name string) (jmap.ID, bool) {
	if id, ok := c.mailboxID[name]; ok {
		return id, true
	}

	for _, k := range c.resolvePriorityKeys(name) {
		if id, ok := c.mailboxID[k]; ok {
			return id, true
		}
	}

	var matches []string
	for k := range c.mailboxID {
		if strings.EqualFold(k, name) {
			matches = append(matches, k)
		}
	}
	if len(matches) > 0 {
		return c.resolveMatchingMailbox(name, matches)
	}

	lowerSuffix := "/" + strings.ToLower(name)
	for k := range c.mailboxID {
		if strings.HasSuffix(strings.ToLower(k), lowerSuffix) {
			matches = append(matches, k)
		}
	}
	if len(matches) > 0 {
		return c.resolveMatchingMailbox(name, matches)
	}

	return "", false
}

func (c *client) resolveMatchingMailbox(name string, matches []string) (jmap.ID, bool) {
	sort.Strings(matches)
	firstID := c.mailboxID[matches[0]]

	distinct := false
	for _, m := range matches[1:] {
		if c.mailboxID[m] != firstID {
			distinct = true
			break
		}
	}
	if distinct {
		slog.Warn("getMailboxID multiple distinct mailboxes matched; selecting first",
			slog.String("lookupName", name),
			slog.String("selectedKey", matches[0]),
			slog.Any("matches", matches))
	}
	return firstID, true
}

func (c *client) resolvePriorityKeys(name string) []string {
	lowerName := strings.ToLower(name)
	inboxName := c.inboxName()
	if lowerName == "inbox" || strings.EqualFold(name, inboxName) {
		return []string{inboxName, "Inbox", "INBOX", "inbox"}
	}
	if lowerName == "spam" || strings.EqualFold(name, "junk") {
		return []string{"Spam", "spam", "SPAM", "Junk", "junk"}
	}
	return nil
}

func (c *client) getOrCreateMailbox(name string) (jmap.ID, error) {
	id, err := c.getMailboxID(name)
	if err == nil {
		return id, nil
	}

	parts := strings.Split(name, "/")
	var parentID jmap.ID

	for i := 0; i < len(parts); i++ {
		currentPath := strings.Join(parts[:i+1], "/")
		id, err = c.getMailboxID(currentPath)
		if err != nil {
			createdID, errCreate := c.createMailboxLevel(context.Background(), currentPath, parts[i], parentID)
			if errCreate != nil {
				return "", errCreate
			}

			id = createdID
			if errRefresh := c.loadMailboxes(); errRefresh != nil {
				return "", errRefresh
			}
		}
		parentID = id
	}

	return parentID, nil
}

func (c *client) EnsureLabelExists(folder label.Folder) error {
	name := folder.String()
	_, err := c.getOrCreateMailbox(name)
	return err
}

func (c *client) MarkAsRead(messageIDs []mailemail.ID) error {
	if len(messageIDs) == 0 {
		return nil
	}
	if err := c.init(); err != nil {
		return err
	}
	updates := make(map[jmap.ID]jmap.Patch)
	for _, id := range messageIDs {
		updates[jmap.ID(id)] = jmap.Patch{"keywords/$seen": true}
	}
	req := &jmap.Request{}
	req.Invoke(&email.Set{
		Account: c.accID,
		Update:  updates,
	})
	resp, err := executeJMAPOpWithRetry(context.Background(), "mark as read", func() (*jmap.Response, error) {
		return c.client.Do(req)
	})
	if err != nil {
		return err
	}
	var lastErr error
	forEachNotUpdated(resp, func(failedID jmap.ID, errSet *jmap.SetError) {
		lastErr = fmt.Errorf("failed to mark %s as read: %s", failedID, safeStr(errSet.Description))
	})
	return lastErr
}

const maxJMAPAttempts = 4

var jmapRetryBaseDelay = 500 * time.Millisecond

func isRetryableJMAPError(err error) bool {
	if err == nil {
		return false
	}
	var reqErr *jmap.RequestError
	if errors.As(err, &reqErr) {
		switch reqErr.Status {
		case http.StatusTooManyRequests,
			http.StatusServiceUnavailable,
			http.StatusBadGateway,
			http.StatusGatewayTimeout:
			return true
		default:
			return false
		}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	msg := err.Error()
	if strings.Contains(msg, "HTTP 429") || strings.Contains(msg, "HTTP 503") ||
		strings.Contains(msg, "HTTP 502") || strings.Contains(msg, "HTTP 504") {
		return true
	}
	return false
}

func computeJMAPRetryDelay(attempt int) time.Duration {
	wait := jmapRetryBaseDelay * (1 << attempt)
	half := int64(wait) / 2
	if half <= 0 {
		return wait
	}
	return time.Duration(half) + time.Duration(rand.Int64N(half))
}

func executeJMAPOpWithRetry(ctx context.Context, opName string, do func() (*jmap.Response, error)) (*jmap.Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var lastErr error
	for attempt := 0; attempt < maxJMAPAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		resp, err := do()
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !isRetryableJMAPError(err) || attempt == maxJMAPAttempts-1 {
			return nil, err
		}
		sleepDur := computeJMAPRetryDelay(attempt)
		slog.Debug("JMAP API throttled, retrying",
			slog.String("op", opName),
			slog.Int("attempt", attempt+1),
			slog.Duration("wait", sleepDur),
			slog.Any("error", err))
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(sleepDur):
		}
	}
	return nil, lastErr
}

const maxMessageDownloadBytes = 50 * 1024 * 1024 // 50MB ceiling

const maxBlobDownloadAttempts = 4

var blobRetryBaseDelay = 500 * time.Millisecond

func blobRetryDelay(resp *http.Response, attempt int) time.Duration {
	if secs, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	wait := blobRetryBaseDelay * (1 << attempt)
	half := int64(wait) / 2
	if half <= 0 {
		return wait
	}
	return time.Duration(half) + time.Duration(rand.Int64N(half))
}

func readBlobResponseBody(resp *http.Response) ([]byte, error) {
	lr := io.LimitReader(resp.Body, maxMessageDownloadBytes+1)
	data, err := io.ReadAll(lr)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxMessageDownloadBytes {
		return nil, fmt.Errorf("download exceeded maximum allowed size (%d bytes)", maxMessageDownloadBytes)
	}
	return data, nil
}

func (c *client) downloadBlob(accountID, blobID, name, contentType string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return c.downloadBlobContext(ctx, accountID, blobID, name, contentType)
}

func (c *client) downloadBlobContext(ctx context.Context, accountID, blobID, name, contentType string) ([]byte, error) {
	urlStr := c.client.Session.DownloadURL
	urlStr = strings.ReplaceAll(urlStr, "{accountId}", url.PathEscape(accountID))
	urlStr = strings.ReplaceAll(urlStr, "{blobId}", url.PathEscape(blobID))
	urlStr = strings.ReplaceAll(urlStr, "{name}", url.PathEscape(name))
	urlStr = strings.ReplaceAll(urlStr, "{type}", url.QueryEscape(contentType))

	var lastStatus int
	var lastBody string
	for attempt := 0; attempt < maxBlobDownloadAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
		if err != nil {
			return nil, err
		}
		resp, err := c.client.HttpClient.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusOK {
			defer resp.Body.Close()
			return readBlobResponseBody(resp)
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		lastStatus = resp.StatusCode
		lastBody = string(body)
		_ = resp.Body.Close()

		if (resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode != http.StatusServiceUnavailable) || attempt == maxBlobDownloadAttempts-1 {
			break
		}
		wait := blobRetryDelay(resp, attempt)
		slog.Debug("throttled downloading JMAP blob, retrying",
			slog.Int("status", resp.StatusCode),
			slog.Duration("wait", wait),
			slog.Int("attempt", attempt+1))
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}

	return nil, fmt.Errorf("server returned status %d: %s", lastStatus, lastBody)
}

func (c *client) getIdentityID() (jmap.ID, error) {
	req := &jmap.Request{}
	req.Invoke(&identity.Get{
		Account: c.accID,
	})
	resp, err := executeJMAPOpWithRetry(context.Background(), "get identity", func() (*jmap.Response, error) {
		return c.client.Do(req)
	})
	if err != nil {
		return "", err
	}

	var identityID jmap.ID
	sendAddr := c.account.GetSendAddress()
	for _, inv := range resp.Responses {
		if r, ok := inv.Args.(*identity.GetResponse); ok {
			for _, ident := range r.List {
				if strings.EqualFold(ident.Email, sendAddr) {
					identityID = ident.ID
					break
				}
			}
			if identityID == "" && len(r.List) > 0 {
				identityID = r.List[0].ID
			}
		}
	}

	if identityID == "" {
		return "", fmt.Errorf("no identity found for address %s", sendAddr)
	}
	return identityID, nil
}

func (c *client) Validate() error {
	if err := cfg_g.ValidateAccountParams(c.account); err != nil {
		return err
	}
	return c.init()
}

func (c *client) GetMatchingLabels(prefix string) ([]string, error) {
	if err := c.init(); err != nil {
		return nil, err
	}

	var result []string
	prefixLower := strings.ToLower(prefix)
	for name := range c.mailboxID {
		if strings.HasPrefix(strings.ToLower(name), prefixLower) {
			result = append(result, name)
		}
	}

	sort.Strings(result)
	return result, nil
}

func syncJMAPReadState(cacheDir cachedir.Account, list []*email.Email) error {
	err := cache.UpdateReadState(cacheDir, func(state map[mailemail.ID]bool) {
		for _, em := range list {
			state[mailemail.ID(em.ID)] = em.Keywords["$seen"]
		}
	})
	if err != nil {
		slog.Error("Failed to save read state after JMAP sync", slog.Any("error", err))
		return fmt.Errorf("failed to save read state: %w", err)
	}
	return nil
}

func (c *client) queryFolderEmailIDs(mbID jmap.ID, limit int) ([]jmap.ID, error) {
	req := &jmap.Request{}
	req.Invoke(&email.Query{
		Account: c.accID,
		Filter: &email.FilterCondition{
			InMailbox: mbID,
		},
		Limit: uint64(limit),
	})

	resp, err := executeJMAPOpWithRetry(context.Background(), "query folder email IDs", func() (*jmap.Response, error) {
		return c.client.Do(req)
	})
	if err != nil {
		return nil, err
	}

	for _, inv := range resp.Responses {
		if qr, ok := inv.Args.(*email.QueryResponse); ok {
			return qr.IDs, nil
		}
	}
	return nil, nil
}

func (c *client) fetchEmailDetails(ids []jmap.ID, properties []string) ([]*email.Email, error) {
	return c.fetchEmailDetailsContext(context.Background(), ids, properties)
}

func (c *client) fetchEmailDetailsContext(ctx context.Context, ids []jmap.ID, properties []string) ([]*email.Email, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	reqGet := &jmap.Request{Context: ctx}
	reqGet.Invoke(&email.Get{
		Account:    c.accID,
		IDs:        ids,
		Properties: properties,
	})

	respGet, err := executeJMAPOpWithRetry(ctx, "fetch email details", func() (*jmap.Response, error) {
		return c.client.Do(reqGet)
	})
	if err != nil {
		return nil, err
	}

	for _, inv := range respGet.Responses {
		if gr, ok := inv.Args.(*email.GetResponse); ok {
			return gr.List, nil
		}
	}
	return nil, fmt.Errorf("failed to retrieve email details")
}

func ensureMessagesDir(cacheDir string) error {
	if err := os.MkdirAll(filepath.Join(cacheDir, "messages"), 0700); err != nil {
		return fmt.Errorf("failed to create messages cache directory: %w", err)
	}
	return nil
}

// FetchAndDownloadEmails downloads emails from a JMAP mailbox.
func (c *client) FetchAndDownloadEmails(folder label.Folder, _ string) ([]mailemail.ID, error) {
	folderName := folder.String()
	if err := c.init(); err != nil {
		return nil, err
	}

	mbID, err := c.getMailboxID(folderName)
	if err != nil {
		return nil, err
	}

	slog.Info("FetchAndDownloadEmails: querying mailbox", slog.String("folder", folderName), slog.String("mailboxId", string(mbID)))

	ids, err := c.queryFolderEmailIDs(mbID, c.config.Limit)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		slog.Info("FetchAndDownloadEmails: JMAP query returned no results", slog.String("folder", folderName), slog.String("mailboxId", string(mbID)))
		if replaceErr := folderindex.ReplaceAll(c.config.AccountCache, folderName, []mailemail.ID{}); replaceErr != nil {
			return nil, replaceErr
		}
		return nil, nil
	}

	emailList, err := c.fetchEmailDetails(ids, []string{"id", "blobId", "keywords"})
	if err != nil {
		return nil, err
	}

	cacheDir := c.config.AccountCache
	if err := syncJMAPReadState(cacheDir, emailList); err != nil {
		return nil, err
	}

	if err := ensureMessagesDir(cacheDir.Path()); err != nil {
		return nil, err
	}
	slog.Info("Refreshing JMAP email cache", slog.Int("count", len(emailList)), slog.String("folder", folderName))

	idStrings := make([]mailemail.ID, len(emailList))
	for i, em := range emailList {
		idStrings[i] = mailemail.ID(em.ID)
	}
	if err := c.newMessageStore().Prefetch(idStrings); err != nil {
		slog.Warn("some messages in folder could not be downloaded",
			slog.String("folder", folderName), slog.Any("error", err))
	}

	if err := folderindex.ReplaceAll(cacheDir, folderName, idStrings); err != nil {
		slog.Error("Failed to save folder index", slog.Any("error", err))
	}

	return idStrings, nil
}

func (c *client) Config() *cfg_g.Config {
	return c.config
}

func (c *client) RewriteRuleLabelCasing(label string) string {
	if err := c.init(); err != nil {
		return label
	}

	jmapLabels := make(map[string]string)
	for path := range c.mailboxID {
		jmapLabels[strings.ToLower(path)] = path
	}

	parts := strings.Split(label, "/")
	for i := 1; i <= len(parts); i++ {
		prefix := strings.Join(parts[:i], "/")
		lowerPrefix := strings.ToLower(prefix)
		if exactName, ok := jmapLabels[lowerPrefix]; ok {
			exactParts := strings.Split(exactName, "/")
			for idx, val := range exactParts {
				if idx >= len(parts) {
					break
				}
				if parts[idx] != val {
					parts[idx] = val
				}
			}
		}
	}
	return strings.Join(parts, "/")
}

func (c *client) inboxName() string {
	return "Inbox"
}

func (c *client) BackendType() string { return "jmap" }

// InboxFolder is the account's inbox, as the server names it.
func (c *client) InboxFolder() label.Folder {
	return label.FromServer(c.inboxName())
}
