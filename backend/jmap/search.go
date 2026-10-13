package jmap

import (
	"fmt"
	mailemail "github.com/sarielhp/mail_cli/email"
	"strings"

	"git.sr.ht/~rockorager/go-jmap"
	jmapemail "git.sr.ht/~rockorager/go-jmap/mail/email"
)

// jmapSearchPageSize bounds one Email/query request.
const jmapSearchPageSize = 100

// SearchEmails runs a server-side JMAP search.
//
// JMAP has no query language: Email/query takes a structured filter. Rather
// than send a whole Gmail-style string as opaque free text, recognized
// qualifiers are lifted into the fields JMAP has for them, so `from:alice
// invoice` searches the From header for alice and the rest of the message for
// invoice, as a user typing it would expect.
func (c *client) SearchEmails(query string, limit int) ([]mailemail.ID, error) {
	if err := c.init(); err != nil {
		return nil, err
	}
	filter := buildJMAPFilter(query)

	var ids []mailemail.ID
	var position uint64
	for {
		want := jmapSearchPageSize
		if limit > 0 {
			want = min(limit-len(ids), jmapSearchPageSize)
		}

		page, err := c.queryEmailPage(filter, position, uint64(want))
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			break
		}
		for _, id := range page {
			ids = append(ids, mailemail.ID(id))
		}
		if limit > 0 && len(ids) >= limit {
			break
		}
		// A short page means the server has nothing more to give.
		if len(page) < want {
			break
		}
		position += uint64(len(page))
	}

	if limit > 0 && len(ids) > limit {
		ids = ids[:limit]
	}
	return ids, nil
}

func (c *client) queryEmailPage(filter *jmapemail.FilterCondition, position, limit uint64) ([]jmap.ID, error) {
	req := &jmap.Request{}
	req.Invoke(&jmapemail.Query{
		Account:  c.accID,
		Filter:   filter,
		Position: int64(position),
		Limit:    limit,
		Sort: []*jmapemail.SortComparator{
			{Property: "receivedAt", IsAscending: false},
		},
	})

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jmap search: %w", err)
	}
	for _, inv := range resp.Responses {
		if qr, ok := inv.Args.(*jmapemail.QueryResponse); ok {
			return qr.IDs, nil
		}
	}
	return nil, nil
}

// buildJMAPFilter turns a query string into a JMAP filter, lifting recognized
// `key:value` qualifiers into their own fields and leaving everything else as
// free text.
//
// A quoted value keeps its spaces, so `subject:"quarterly report"` searches for
// the phrase rather than for "quarterly" alone.
func buildJMAPFilter(query string) *jmapemail.FilterCondition {
	filter := &jmapemail.FilterCondition{}
	var freeText []string

	for _, token := range splitQueryTokens(query) {
		key, value, found := strings.Cut(token, ":")
		if !found || value == "" {
			freeText = append(freeText, token)
			continue
		}
		if !applyJMAPQualifier(filter, strings.ToLower(key), value) {
			freeText = append(freeText, token)
		}
	}

	if len(freeText) > 0 {
		filter.Text = strings.Join(freeText, " ")
	}
	return filter
}

// applyJMAPQualifier sets one recognized qualifier, reporting whether it knew
// the key. An unknown key is not an error: it falls back to free text, so a
// colon inside an ordinary search term still finds something.
func applyJMAPQualifier(filter *jmapemail.FilterCondition, key, value string) bool {
	switch key {
	case "from":
		filter.From = value
	case "to":
		filter.To = value
	case "cc":
		filter.Cc = value
	case "bcc":
		filter.Bcc = value
	case "subject":
		filter.Subject = value
	case "body":
		filter.Body = value
	case "has":
		if strings.EqualFold(value, "attachment") {
			filter.HasAttachment = true
			return true
		}
		return false
	default:
		return false
	}
	return true
}

// splitQueryTokens splits on spaces while keeping double-quoted runs together.
func splitQueryTokens(query string) []string {
	var tokens []string
	var current strings.Builder
	inQuotes := false

	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}

	for _, r := range query {
		switch {
		case r == '"':
			inQuotes = !inQuotes
		case r == ' ' && !inQuotes:
			flush()
		default:
			current.WriteRune(r)
		}
	}
	flush()
	return tokens
}
