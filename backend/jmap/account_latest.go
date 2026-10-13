package jmap

import (
	mailemail "github.com/sarielhp/mail_cli/email"
	"log/slog"

	"github.com/sarielhp/mail_cli/cfg_acc"

	"git.sr.ht/~rockorager/go-jmap"
	"git.sr.ht/~rockorager/go-jmap/mail/email"
)

func (c *client) queryLatestEmailIDs(limit int) ([]jmap.ID, error) {
	req := &jmap.Request{}
	req.Invoke(&email.Query{
		Account: c.accID,
		Sort: []*email.SortComparator{
			{
				Property:    "receivedAt",
				IsAscending: false,
			},
		},
		Limit: uint64(limit),
	})

	resp, err := c.client.Do(req)
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

func buildAccountFolderRefs(list []*email.Email, idToName map[jmap.ID]string) []cfg_acc.MessageFolderRef {
	refs := make([]cfg_acc.MessageFolderRef, len(list))
	for i, em := range list {
		folder := "Inbox"
		for mbID := range em.MailboxIDs {
			if name, ok := idToName[mbID]; ok && name != "" {
				folder = name
				break
			}
		}
		refs[i] = cfg_acc.MessageFolderRef{
			MessageID: mailemail.ID(em.ID),
			Folder:    folder,
		}
	}
	return refs
}

// FetchLatestAccountEmails queries Fastmail/JMAP account-wide for the latest N emails.
func (c *client) FetchLatestAccountEmails(limit int) ([]cfg_acc.MessageFolderRef, error) {
	if err := c.init(); err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 100
	}

	idToName := make(map[jmap.ID]string, len(c.mailboxID))
	for name, id := range c.mailboxID {
		idToName[id] = name
	}

	ids, err := c.queryLatestEmailIDs(limit)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}

	emailList, err := c.fetchEmailDetails(ids, []string{"id", "blobId", "mailboxIds", "keywords"})
	if err != nil {
		return nil, err
	}
	if len(emailList) == 0 {
		return nil, nil
	}

	cacheDir := c.config.AccountCache
	if err := ensureMessagesDir(cacheDir.Path()); err != nil {
		slog.Warn("failed to create messages cache directory", slog.Any("error", err))
	}
	_ = syncJMAPReadState(cacheDir, emailList)

	idStrings := make([]mailemail.ID, len(emailList))
	for i, em := range emailList {
		idStrings[i] = mailemail.ID(em.ID)
	}
	// A download failure is logged rather than fatal: the caller wants the list
	// of recent messages, and one that could not be cached is still worth
	// listing.
	if err := c.newMessageStore().Prefetch(idStrings); err != nil {
		slog.Warn("some recent messages could not be downloaded", slog.Any("error", err))
	}

	return buildAccountFolderRefs(emailList, idToName), nil
}
