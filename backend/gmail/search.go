package gmail

import (
	"fmt"
	"github.com/sarielhp/mail_cli/email"

	gmailapi "google.golang.org/api/gmail/v1"
)

// gmailSearchPageSize is the largest page Gmail's list endpoint will return.
const gmailSearchPageSize = 100

// SearchEmails runs a Gmail search. The query is passed through untouched, so
// the whole Gmail query language -- from:, has:attachment, older_than:, and the
// rest -- works exactly as it does in the web interface.
func (c *client) SearchEmails(query string, limit int) ([]email.ID, error) {
	srv, err := GetGmailService(c.rep, c.config)
	if err != nil {
		return nil, err
	}
	return searchMessageIDs(srv, query, limit)
}

func searchMessageIDs(srv *gmailapi.Service, query string, limit int) ([]email.ID, error) {
	var ids []email.ID
	pageToken := ""

	for {
		call := srv.Users.Messages.List("me").Q(query).IncludeSpamTrash(true)
		call = call.MaxResults(int64(searchPageSize(limit, len(ids))))
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		res, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("gmail search %q: %w", query, err)
		}
		for _, m := range res.Messages {
			ids = append(ids, email.ID(m.Id))
		}

		if res.NextPageToken == "" {
			break
		}
		if limit > 0 && len(ids) >= limit {
			break
		}
		pageToken = res.NextPageToken
	}

	if limit > 0 && len(ids) > limit {
		ids = ids[:limit]
	}
	return ids, nil
}

// searchPageSize asks for only as many results as are still wanted, so a
// search for five matches does not pull a hundred.
func searchPageSize(limit, have int) int {
	if limit <= 0 {
		return gmailSearchPageSize
	}
	return min(limit-have, gmailSearchPageSize)
}
