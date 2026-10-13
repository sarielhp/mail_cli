package jmap

import (
	"bytes"
	"fmt"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_acc"
	myme "github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/mailclient"
	"mime"
	"net/mail"
	"strings"

	"git.sr.ht/~rockorager/go-jmap"
	"git.sr.ht/~rockorager/go-jmap/mail/email"
	"git.sr.ht/~rockorager/go-jmap/mail/emailsubmission"
)

// forEachNotUpdated iterates resp.Responses for *email.SetResponse and calls fn
// for each entry in NotUpdated.
func forEachNotUpdated(resp *jmap.Response, fn func(id jmap.ID, errSet *jmap.SetError)) {
	forEachSetError(resp, notUpdatedErrors, fn)
}

// forEachNotDestroyedEmail iterates resp.Responses for *email.SetResponse and calls fn
// for each entry in NotDestroyed.
func forEachNotDestroyedEmail(resp *jmap.Response, fn func(id jmap.ID, errSet *jmap.SetError)) {
	forEachSetError(resp, notDestroyedErrors, fn)
}

func notUpdatedErrors(r *email.SetResponse) map[jmap.ID]*jmap.SetError { return r.NotUpdated }

func notDestroyedErrors(r *email.SetResponse) map[jmap.ID]*jmap.SetError { return r.NotDestroyed }

// forEachSetError calls fn for every entry of the per-ID error map that errors
// selects from a SetResponse, ignoring responses that are not a SetResponse.
func forEachSetError(resp *jmap.Response, errors func(*email.SetResponse) map[jmap.ID]*jmap.SetError, fn func(id jmap.ID, errSet *jmap.SetError)) {
	for _, inv := range resp.Responses {
		r, ok := inv.Args.(*email.SetResponse)
		if !ok {
			continue
		}
		for id, errSet := range errors(r) {
			fn(id, errSet)
		}
	}
}

type candidateInfo struct {
	rule    cfg_acc.Rule
	sender  string
	subject string
}

func (c *client) CheckAndApplyRules(messageIDs []myme.ID, _ label.Folder, _ string) ([]myme.ID, error) {
	if len(messageIDs) == 0 {
		return messageIDs, nil
	}
	if err := c.init(); err != nil {
		return nil, err
	}

	candidates, matchedIDs, remainingIDs := c.loadCandidates(messageIDs)
	if len(matchedIDs) == 0 {
		return remainingIDs, nil
	}

	currentMailboxIDs, err := c.fetchCurrentMailboxIDs(toJmapIDs(matchedIDs))
	if err != nil {
		remainingIDs = append(remainingIDs, matchedIDs...)
		return remainingIDs, err
	}

	return c.applyRuleMoves(matchedIDs, candidates, currentMailboxIDs, remainingIDs), nil
}

func (c *client) loadCandidates(messageIDs []myme.ID) (map[myme.ID]candidateInfo, []myme.ID, []myme.ID) {
	dec := new(mime.WordDecoder)
	candidates := make(map[myme.ID]candidateInfo)
	var matchedIDs, remainingIDs []myme.ID

	for _, id := range messageIDs {
		data, rErr := msg.Read(c.config.AccountCache, id)
		if rErr != nil {
			remainingIDs = append(remainingIDs, id)
			continue
		}
		parsed, errParse := mail.ReadMessage(bytes.NewReader(data))
		if errParse != nil {
			remainingIDs = append(remainingIDs, id)
			continue
		}
		fromHeader := parsed.Header.Get("From")
		sender := myme.ParseEmailAddress(fromHeader)
		subject := myme.DecodeHeader(dec, parsed.Header.Get("Subject"))

		if matchedRule := cfg_acc.MatchRules(c.config.Rules, sender, subject); matchedRule != nil {
			candidates[id] = candidateInfo{rule: *matchedRule, sender: sender, subject: subject}
			matchedIDs = append(matchedIDs, id)
		} else {
			remainingIDs = append(remainingIDs, id)
		}
	}
	return candidates, matchedIDs, remainingIDs
}

func toJmapIDs(ids []myme.ID) []jmap.ID {
	out := make([]jmap.ID, len(ids))
	for i, id := range ids {
		out[i] = jmap.ID(id)
	}
	return out
}

func (c *client) fetchCurrentMailboxIDs(jmapIDs []jmap.ID) (map[string]map[string]bool, error) {
	req := &jmap.Request{}
	req.Invoke(&email.Get{
		Account:    c.accID,
		IDs:        jmapIDs,
		Properties: []string{"mailboxIds"},
	})
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}

	var getResp *email.GetResponse
	for _, inv := range resp.Responses {
		if gr, ok := inv.Args.(*email.GetResponse); ok {
			getResp = gr
		}
	}

	out := make(map[string]map[string]bool)
	if getResp == nil {
		return out, nil
	}
	for _, em := range getResp.List {
		mbs := make(map[string]bool, len(em.MailboxIDs))
		for mbID := range em.MailboxIDs {
			mbs[string(mbID)] = true
		}
		out[string(em.ID)] = mbs
	}
	return out, nil
}

func (c *client) applyRuleMoves(matchedIDs []myme.ID, candidates map[myme.ID]candidateInfo, currentMailboxIDs map[string]map[string]bool, remainingIDs []myme.ID) []myme.ID {
	for _, id := range matchedIDs {
		remainingIDs = c.applySingleRuleMove(id, candidates[id], currentMailboxIDs[string(id)], remainingIDs)
	}
	return remainingIDs
}

func (c *client) applySingleRuleMove(id myme.ID, candidate candidateInfo, mbs map[string]bool, remainingIDs []myme.ID) []myme.ID {
	targetID, err := c.getOrCreateMailbox(candidate.rule.Label)
	if err != nil {
		if c.config.Verbose {
			c.rep.Warnf("    JMAP Rule: Failed to resolve/create mailbox %q: %v\n", candidate.rule.Label, err)
		}
		return append(remainingIDs, id)
	}

	if len(mbs) == 1 && mbs[string(targetID)] {
		if c.config.Verbose {
			c.rep.Infof("JMAP Rule match has 0 effect (already exclusively in %q) for message %s. Keeping message as-is.\n", candidate.rule.Label, id)
		}
		return append(remainingIDs, id)
	}

	req := &jmap.Request{}
	req.Invoke(&email.Set{
		Account: c.accID,
		Update: map[jmap.ID]jmap.Patch{
			jmap.ID(id): {"mailboxIds": map[string]bool{string(targetID): true}},
		},
	})
	resp, err := c.client.Do(req)
	if err != nil {
		if c.config.Verbose {
			c.rep.Warnf("    JMAP Rule: Move failed for message %s: %v\n", id, err)
		}
		return append(remainingIDs, id)
	}

	isSuccess := true
	forEachNotUpdated(resp, func(failedID jmap.ID, errSet *jmap.SetError) {
		if failedID == jmap.ID(id) {
			isSuccess = false
			if c.config.Verbose {
				c.rep.Warnf("    JMAP Rule: Move failed: %s\n", safeStr(errSet.Description))
			}
		}
	})

	if isSuccess {
		ruleDesc := candidate.rule.Sender
		if candidate.rule.Subject != "" {
			ruleDesc = fmt.Sprintf("subject: %s*", candidate.rule.Subject)
		}
		c.rep.Successf("JMAP Rule Applied: Message %s (%q) matching '%s' moved to %q\n", id, candidate.subject, ruleDesc, candidate.rule.Label)
		return remainingIDs
	}
	return append(remainingIDs, id)
}

// setMailboxOperation is the shared helper for MoveEmail and CopyEmail.
// buildPatch receives the resolved destID and returns the patch to apply.
func (c *client) setMailboxOperation(messageIDs []myme.ID, destLabelName string, buildPatch func(jmap.ID) jmap.Patch) error {
	if len(messageIDs) == 0 {
		return nil
	}
	if err := c.init(); err != nil {
		return err
	}

	destID, err := c.getOrCreateMailbox(destLabelName)
	if err != nil {
		return err
	}

	updates := make(map[jmap.ID]jmap.Patch, len(messageIDs))
	patch := buildPatch(destID)
	for _, id := range messageIDs {
		updates[jmap.ID(id)] = patch
	}

	req := &jmap.Request{}
	req.Invoke(&email.Set{
		Account: c.accID,
		Update:  updates,
	})
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}

	// Email/set answers per message, so a batch can partly succeed. Naming only
	// the first failure told the caller an error happened and nothing about
	// which messages it happened to -- and the caller then skipped local
	// reconciliation for the whole batch, including the messages that moved.
	var opErr error
	var failed []myme.ID
	forEachNotUpdated(resp, func(id jmap.ID, errSet *jmap.SetError) {
		msg := fmt.Sprintf("failed to move message %s to JMAP mailbox %s: %s (type: %s)", id, destLabelName, safeStr(errSet.Description), errSet.Type)
		c.rep.Errorf("    %s\n", msg)
		failed = append(failed, myme.ID(id))
		if opErr == nil {
			opErr = fmt.Errorf("%s", msg)
		}
	})
	if opErr == nil {
		return nil
	}
	return &mailclient.PartialMoveError{Failed: failed, Err: opErr}
}

// buildMovePatch constructs a JMAP patch that adds destID and removes sourceID
// without destroying other mailbox memberships.
func buildMovePatch(destID, sourceID jmap.ID) jmap.Patch {
	patch := jmap.Patch{"mailboxIds/" + string(destID): true}
	if sourceID != "" && sourceID != destID {
		patch["mailboxIds/"+string(sourceID)] = nil
	}
	return patch
}

func (c *client) MoveEmail(messageIDs []myme.ID, source label.Folder, dest label.Folder) error {
	sourceLabelName := source.String()
	destLabelName := dest.String()
	if len(messageIDs) == 0 {
		return nil
	}
	if strings.EqualFold(sourceLabelName, destLabelName) {
		if c.config.Verbose {
			c.rep.Printf("    [Debug] MoveEmail: source and destination are the same (%q). Skipping.\n", destLabelName)
		}
		return nil
	}
	if err := c.init(); err != nil {
		return err
	}
	var sourceID jmap.ID
	if sourceLabelName != "" {
		sid, err := c.getMailboxID(sourceLabelName)
		if err != nil {
			return err
		}
		sourceID = sid
	}
	return c.setMailboxOperation(messageIDs, destLabelName, func(destID jmap.ID) jmap.Patch {
		return buildMovePatch(destID, sourceID)
	})
}

func (c *client) CopyEmail(messageIDs []myme.ID, _ label.Folder, dest label.Folder) error {
	destLabelName := dest.String()
	if len(messageIDs) == 0 {
		return nil
	}
	return c.setMailboxOperation(messageIDs, destLabelName, func(destID jmap.ID) jmap.Patch {
		return jmap.Patch{"mailboxIds/" + string(destID): true}
	})
}

func (c *client) ReportSpam(messageIDs []myme.ID, _ label.Folder) error {
	if len(messageIDs) == 0 {
		return nil
	}
	if err := c.init(); err != nil {
		return err
	}

	spamFolder := c.account.SpamFolder
	if spamFolder == "" {
		spamFolder = "Spam"
	}

	spamID, err := c.getMailboxID(spamFolder)
	if err != nil {
		return err
	}

	updates := make(map[jmap.ID]jmap.Patch, len(messageIDs))
	for _, id := range messageIDs {
		updates[jmap.ID(id)] = jmap.Patch{
			"mailboxIds": map[string]bool{string(spamID): true},
		}
	}

	req := &jmap.Request{}
	req.Invoke(&email.Set{
		Account: c.accID,
		Update:  updates,
	})
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}

	forEachNotUpdated(resp, func(id jmap.ID, errSet *jmap.SetError) {
		c.rep.Errorf("    Failed to move message %s to JMAP Spam: %s (type: %s)\n", id, safeStr(errSet.Description), errSet.Type)
	})
	return nil
}

func (c *client) MoveToInbox(messageIDs []myme.ID, source label.Folder) error {
	sourceLabelName := source.String()
	if len(messageIDs) == 0 {
		return nil
	}
	inboxName := c.inboxName()
	if strings.EqualFold(sourceLabelName, inboxName) {
		if c.config.Verbose {
			c.rep.Printf("    [Debug] MoveToInbox: message already in Inbox. Skipping.\n")
		}
		return nil
	}
	if err := c.init(); err != nil {
		return err
	}

	inboxID, err := c.getMailboxID(inboxName)
	if err != nil {
		return err
	}

	unspamID, errUnspam := c.resolveUnspamMailbox()
	if errUnspam != nil {
		c.rep.Warnf("    Warning: failed to get or create JMAP mailbox %q: %v\n", c.config.UnspamLearn, errUnspam)
	}

	updates := make(map[jmap.ID]jmap.Patch, len(messageIDs))
	for _, id := range messageIDs {
		mailboxIDs := map[string]bool{string(inboxID): true}
		if unspamID != "" {
			mailboxIDs[string(unspamID)] = true
		}
		updates[jmap.ID(id)] = jmap.Patch{"mailboxIds": mailboxIDs}
	}

	req := &jmap.Request{}
	req.Invoke(&email.Set{
		Account: c.accID,
		Update:  updates,
	})
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}

	forEachNotUpdated(resp, func(id jmap.ID, errSet *jmap.SetError) {
		c.rep.Errorf("    Failed to move message %s to JMAP Inbox: %s (type: %s)\n", id, safeStr(errSet.Description), errSet.Type)
	})
	return nil
}

func (c *client) resolveUnspamMailbox() (jmap.ID, error) {
	if c.config.UnspamLearn == "" {
		return "", nil
	}
	return c.getOrCreateMailbox(c.config.UnspamLearn)
}

// DeleteMessages destroys messages (Email/set destroy); JMAP has no trash
// of its own to pass them through.
func (c *client) DeleteMessages(messageIDs []myme.ID, _ label.Folder) error {
	if len(messageIDs) == 0 {
		return nil
	}
	if err := c.init(); err != nil {
		return err
	}
	ids := make([]jmap.ID, len(messageIDs))
	for i, id := range messageIDs {
		ids[i] = jmap.ID(id)
	}
	req := &jmap.Request{}
	req.Invoke(&email.Set{Account: c.accID, Destroy: ids})
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("JMAP destroy request failed: %w", err)
	}
	var firstErr error
	forEachNotDestroyedEmail(resp, func(id jmap.ID, errSet *jmap.SetError) {
		if firstErr == nil {
			firstErr = fmt.Errorf("JMAP could not destroy %s: %s", id, safeStr(errSet.Description))
		}
	})
	return firstErr
}

func (c *client) UploadRawEmail(rawBytes []byte, target label.Folder) (myme.ID, error) {
	targetLabel := target.String()
	if err := c.init(); err != nil {
		return "", err
	}
	if err := c.EnsureLabelExists(target); err != nil {
		return "", err
	}

	destMailboxID, err := c.getMailboxID(targetLabel)
	if err != nil {
		return "", err
	}

	uploadResp, err := c.client.Upload(c.accID, bytes.NewReader(rawBytes))
	if err != nil {
		return "", fmt.Errorf("failed to upload email blob to JMAP: %w", err)
	}

	req := &jmap.Request{}
	req.Invoke(&email.Import{
		Account: c.accID,
		Emails: map[string]*email.EmailImport{
			"import1": {
				BlobID:     uploadResp.ID,
				MailboxIDs: map[jmap.ID]bool{destMailboxID: true},
			},
		},
	})
	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed JMAP import request: %w", err)
	}

	for _, inv := range resp.Responses {
		r, ok := inv.Args.(*email.ImportResponse)
		if !ok {
			continue
		}
		id, err := c.handleImportResponse(r, rawBytes, destMailboxID)
		if err != nil {
			return "", err
		}
		if id != "" {
			return myme.ID(id), nil
		}
	}
	return "", fmt.Errorf("import completed without assigned ID")
}

func (c *client) handleImportResponse(r *email.ImportResponse, rawBytes []byte, destMailboxID jmap.ID) (string, error) {
	if len(r.Created) > 0 && r.Created["import1"] != nil {
		return string(r.Created["import1"].ID), nil
	}
	for id, jErr := range r.NotCreated {
		if jErr.Type == "alreadyExists" {
			linkedID, linkErr := c.linkExistingEmailToMailbox(rawBytes, destMailboxID)
			if linkErr != nil {
				return "", fmt.Errorf("email already exists in account, but failed to link: %w", linkErr)
			}
			return linkedID, nil
		}
		desc := ""
		if jErr.Description != nil {
			desc = *jErr.Description
		}
		return "", fmt.Errorf("failed to import email %s: %s (%s)", id, jErr.Type, desc)
	}
	return "", nil
}

func (c *client) linkExistingEmailToMailbox(rawBytes []byte, destMailboxID jmap.ID) (string, error) {
	m, err := mail.ReadMessage(bytes.NewReader(rawBytes))
	if err != nil {
		return "", err
	}
	messageID := strings.TrimSpace(m.Header.Get("Message-ID"))
	if messageID == "" {
		return "", nil
	}

	req := &jmap.Request{}
	req.Invoke(&email.Query{
		Account: c.accID,
		Filter: &email.FilterCondition{
			Header: []string{"Message-ID", messageID},
		},
		Limit: 1,
	})

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}

	var queryResp *email.QueryResponse
	for _, inv := range resp.Responses {
		if qr, ok := inv.Args.(*email.QueryResponse); ok {
			queryResp = qr
			break
		}
	}

	if queryResp == nil || len(queryResp.IDs) == 0 {
		return "", nil
	}

	targetID := queryResp.IDs[0]
	setReq := &jmap.Request{}
	setReq.Invoke(&email.Set{
		Account: c.accID,
		Update: map[jmap.ID]jmap.Patch{
			targetID: {"mailboxIds/" + string(destMailboxID): true},
		},
	})
	setResp, err := c.client.Do(setReq)
	if err != nil {
		return "", err
	}
	var setErr error
	forEachNotUpdated(setResp, func(id jmap.ID, errSet *jmap.SetError) {
		if id == targetID && errSet != nil {
			desc := errSet.Type
			if errSet.Description != nil {
				desc = *errSet.Description
			}
			setErr = fmt.Errorf("failed to patch mailbox: %s", desc)
		}
	})
	if setErr != nil {
		return "", setErr
	}
	return string(targetID), nil
}

func (c *client) resolveFallbackMailbox() jmap.ID {
	for _, name := range []string{"inbox", "inbox folder"} {
		if id, exists := c.mailboxID[name]; exists {
			return id
		}
	}
	for _, id := range c.mailboxID {
		return id
	}
	return ""
}

func (c *client) SendEmail(rawBytes []byte) error {
	if err := c.init(); err != nil {
		return err
	}

	identityID, err := c.getIdentityID()
	if err != nil {
		return err
	}

	rcptTo, err := c.parseRecipients(rawBytes)
	if err != nil {
		return err
	}
	if len(rcptTo) == 0 {
		return fmt.Errorf("no recipients found in email")
	}

	cleanBytes := stripBccHeader(rawBytes)
	createdEmailID, err := c.uploadAndImportEmail(cleanBytes)
	if err != nil {
		return err
	}

	return c.submitEmail(createdEmailID, identityID, rcptTo)
}

func (c *client) uploadAndImportEmail(rawBytes []byte) (jmap.ID, error) {
	uploadResp, err := c.client.Upload(c.accID, bytes.NewReader(rawBytes))
	if err != nil {
		return "", fmt.Errorf("failed JMAP upload: %w", err)
	}

	targetMailboxID := c.sentMailboxID
	if targetMailboxID == "" {
		targetMailboxID = c.resolveFallbackMailbox()
	}
	if targetMailboxID == "" {
		return "", fmt.Errorf("failed to submit email: no mailboxes available on JMAP server")
	}

	req := &jmap.Request{}
	req.Invoke(&email.Import{
		Account: c.accID,
		Emails: map[string]*email.EmailImport{
			"import1": {
				BlobID:     uploadResp.ID,
				MailboxIDs: map[jmap.ID]bool{targetMailboxID: true},
			},
		},
	})
	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed JMAP import request: %w", err)
	}

	var createdEmailID jmap.ID
	for _, inv := range resp.Responses {
		r, ok := inv.Args.(*email.ImportResponse)
		if !ok {
			continue
		}
		if created, ok := r.Created["import1"]; ok {
			createdEmailID = created.ID
		}
		for id, jErr := range r.NotCreated {
			desc := ""
			if jErr.Description != nil {
				desc = *jErr.Description
			}
			return "", fmt.Errorf("failed to import email %s: %s (%s)", id, jErr.Type, desc)
		}
	}
	if createdEmailID == "" {
		return "", fmt.Errorf("failed to retrieve created email ID from JMAP import")
	}
	return createdEmailID, nil
}

func (c *client) parseRecipients(rawBytes []byte) ([]*emailsubmission.Address, error) {
	parsedMsg, err := mail.ReadMessage(bytes.NewReader(rawBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to parse email headers for submission: %w", err)
	}

	var rcptTo []*emailsubmission.Address
	seen := make(map[string]bool)
	for _, hdrName := range []string{"To", "Cc", "Bcc"} {
		hdrVal := parsedMsg.Header.Get(hdrName)
		if hdrVal == "" {
			continue
		}
		addrs, err := mail.ParseAddressList(hdrVal)
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			addrLower := strings.ToLower(addr.Address)
			if !seen[addrLower] {
				seen[addrLower] = true
				rcptTo = append(rcptTo, &emailsubmission.Address{Email: addr.Address})
			}
		}
	}
	return rcptTo, nil
}

func stripBccHeader(rawBytes []byte) []byte {
	headerEnd := bytes.Index(rawBytes, []byte("\r\n\r\n"))
	sep := "\r\n"
	sepLen := 4
	if headerEnd == -1 {
		headerEnd = bytes.Index(rawBytes, []byte("\n\n"))
		sep = "\n"
		sepLen = 2
	}
	if headerEnd == -1 {
		return rawBytes
	}

	headerBytes := rawBytes[:headerEnd]
	bodyBytes := rawBytes[headerEnd+sepLen:]
	lines := bytes.Split(headerBytes, []byte("\n"))
	var filtered [][]byte
	inBcc := false
	for _, l := range lines {
		trimmed := bytes.TrimRight(l, "\r")
		if inBcc && len(trimmed) > 0 && (trimmed[0] == ' ' || trimmed[0] == '\t') {
			continue
		}
		inBcc = false
		if bytes.HasPrefix(bytes.ToLower(trimmed), []byte("bcc:")) {
			inBcc = true
			continue
		}
		filtered = append(filtered, trimmed)
	}

	var buf bytes.Buffer
	for _, l := range filtered {
		buf.Write(l)
		buf.WriteString(sep)
	}
	buf.WriteString(sep)
	buf.Write(bodyBytes)
	return buf.Bytes()
}

func (c *client) submitEmail(createdEmailID, identityID jmap.ID, rcptTo []*emailsubmission.Address) error {
	req := &jmap.Request{}
	req.Invoke(&emailsubmission.Set{
		Account: c.accID,
		Create: map[jmap.ID]*emailsubmission.EmailSubmission{
			"sub1": {
				EmailID:    createdEmailID,
				IdentityID: identityID,
				Envelope: &emailsubmission.Envelope{
					MailFrom: &emailsubmission.Address{Email: c.account.GetSendAddress()},
					RcptTo:   rcptTo,
				},
			},
		},
	})
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed JMAP email submission: %w", err)
	}

	for _, inv := range resp.Responses {
		r, ok := inv.Args.(*emailsubmission.SetResponse)
		if !ok || len(r.NotCreated) == 0 {
			continue
		}
		for id, jErr := range r.NotCreated {
			desc := ""
			if jErr.Description != nil {
				desc = *jErr.Description
			}
			return fmt.Errorf("failed to submit email %s: %s (%s)", id, jErr.Type, desc)
		}
	}
	return nil
}
