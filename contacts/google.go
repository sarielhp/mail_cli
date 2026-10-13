package contacts

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/api/people/v1"
)

// FetchGoogleContacts fetches all user contacts and labels from Google People API.
func FetchGoogleContacts(ctx context.Context, client *http.Client, accountName string) ([]Contact, error) {
	if client == nil {
		return nil, fmt.Errorf("http client is nil")
	}

	srv, err := people.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to initialize Google People client: %w", err)
	}

	groupMap := fetchGroupMap(ctx, srv)
	return fetchAllConnections(ctx, srv, accountName, groupMap)
}

func fetchGroupMap(ctx context.Context, srv *people.Service) map[string]string {
	groupMap := make(map[string]string)
	call := srv.ContactGroups.List().Context(ctx)
	res, err := call.Do()
	if err != nil {
		return groupMap
	}

	for _, g := range res.ContactGroups {
		if g == nil {
			continue
		}
		name := g.FormattedName
		if name == "" {
			name = g.Name
		}
		if g.GroupType == "SYSTEM_CONTACT_GROUP" && (name == "Contacts" || name == "myContacts") {
			continue
		}
		if name != "" {
			groupMap[g.ResourceName] = name
		}
	}
	return groupMap
}

func fetchAllConnections(ctx context.Context, srv *people.Service, accountName string, groupMap map[string]string) ([]Contact, error) {
	var contacts []Contact
	pageToken := ""
	source := fmt.Sprintf("google:%s", accountName)

	for {
		req := srv.People.Connections.List("people/me").
			Context(ctx).
			PersonFields("names,emailAddresses,nicknames,memberships").
			PageSize(1000)

		if pageToken != "" {
			req = req.PageToken(pageToken)
		}

		res, err := req.Do()
		if err != nil {
			return nil, checkScopeError(err, accountName)
		}

		for _, p := range res.Connections {
			if p == nil {
				continue
			}
			c := personToContact(p, source, groupMap)
			if len(c.Emails) > 0 || c.Name != "" {
				contacts = append(contacts, c)
			}
		}

		pageToken = res.NextPageToken
		if pageToken == "" {
			break
		}
	}

	return contacts, nil
}

func personToContact(p *people.Person, source string, groupMap map[string]string) Contact {
	name := extractPersonName(p)
	emails := extractPersonEmails(p)
	nicknames := extractPersonNicknames(p)
	groups := extractPersonGroups(p, groupMap)

	id := p.ResourceName
	if id == "" {
		id = fmt.Sprintf("%s:%d", source, len(emails))
	} else {
		id = fmt.Sprintf("%s:%s", source, id)
	}

	return Contact{
		ID:        id,
		Name:      name,
		Emails:    emails,
		Nicknames: nicknames,
		Groups:    groups,
		Source:    source,
	}
}

func extractPersonName(p *people.Person) string {
	if len(p.Names) > 0 && p.Names[0] != nil {
		if p.Names[0].DisplayName != "" {
			return p.Names[0].DisplayName
		}
		first := p.Names[0].GivenName
		last := p.Names[0].FamilyName
		return strings.TrimSpace(first + " " + last)
	}
	return ""
}

func extractPersonEmails(p *people.Person) []string {
	var emails []string
	for _, e := range p.EmailAddresses {
		if e != nil && e.Value != "" {
			emails = append(emails, e.Value)
		}
	}
	return emails
}

func extractPersonNicknames(p *people.Person) []string {
	var nicks []string
	for _, n := range p.Nicknames {
		if n != nil && n.Value != "" {
			nicks = append(nicks, n.Value)
		}
	}
	return nicks
}

func extractPersonGroups(p *people.Person, groupMap map[string]string) []string {
	var groups []string
	seen := make(map[string]bool)
	for _, m := range p.Memberships {
		if m == nil || m.ContactGroupMembership == nil {
			continue
		}
		resName := m.ContactGroupMembership.ContactGroupResourceName
		if gName, ok := groupMap[resName]; ok && !seen[gName] {
			seen[gName] = true
			groups = append(groups, gName)
		}
	}
	return groups
}

func checkScopeError(err error, accountName string) error {
	if gErr, ok := err.(*googleapi.Error); ok {
		if gErr.Code == http.StatusForbidden {
			return fmt.Errorf("insufficient authentication scopes to read Google Contacts for account %q: "+
				"please run 'mail_cli account login %s' to authorize Contacts access", accountName, accountName)
		}
	}
	return fmt.Errorf("failed to fetch Google Contacts: %w", err)
}
