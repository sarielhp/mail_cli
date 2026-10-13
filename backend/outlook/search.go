package outlook

import (
	"encoding/json"
	"fmt"
	"github.com/sarielhp/mail_cli/email"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// graphSearchPageSize bounds one Graph request.
const graphSearchPageSize = 100

// SearchEmails runs a Microsoft Graph search across the mailbox.
//
// Graph's $search takes a quoted KQL string and, unlike the other endpoints
// here, refuses to be combined with $orderby -- it returns results in its own
// relevance order. Every part of the URL is escaped: an unescaped space in a
// Graph query string is answered with a bare 400, which is exactly how the
// account-wide query was broken.
func (c *client) SearchEmails(query string, limit int) ([]email.ID, error) {
	if err := c.init(); err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil, fmt.Errorf("search query is empty")
	}

	var ids []email.ID
	next := buildGraphSearchURL(trimmed, searchTop(limit, 0))

	for next != "" {
		page, following, err := c.fetchSearchPage(next)
		if err != nil {
			return nil, err
		}
		ids = append(ids, page...)
		if limit > 0 && len(ids) >= limit {
			break
		}
		if len(page) == 0 {
			break
		}
		next = following
	}

	if limit > 0 && len(ids) > limit {
		ids = ids[:limit]
	}
	return ids, nil
}

func buildGraphSearchURL(query string, top int) string {
	// The KQL string must arrive quoted, and the quotes themselves have to be
	// escaped or they terminate the query-string value.
	search := url.QueryEscape(fmt.Sprintf("%q", query))
	return fmt.Sprintf("https://graph.microsoft.com/v1.0/me/messages?$search=%s&$top=%d&$select=id", search, top)
}

func searchTop(limit, have int) int {
	if limit <= 0 {
		return graphSearchPageSize
	}
	return min(limit-have, graphSearchPageSize)
}

func (c *client) fetchSearchPage(urlStr string) (ids []email.ID, nextLink string, err error) {
	resp, err := c.client.Get(urlStr)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, "", fmt.Errorf("outlook search failed: status %d: %s",
			resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var res struct {
		Value    []struct{ ID string } `json:"value"`
		NextLink string                `json:"@odata.nextLink"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, "", fmt.Errorf("outlook search: decoding response: %w", err)
	}
	for _, m := range res.Value {
		ids = append(ids, email.ID(m.ID))
	}
	return ids, res.NextLink, nil
}
