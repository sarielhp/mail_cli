package contacts

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/api/people/v1"
)

func TestPersonToContact(t *testing.T) {
	groupMap := map[string]string{
		"contactGroups/g1": "algo",
	}

	p := &people.Person{
		ResourceName: "people/c12345",
		Names: []*people.Name{
			{DisplayName: "Dave Rivers"},
		},
		EmailAddresses: []*people.EmailAddress{
			{Value: "rivers@cs.example.edu"},
		},
		Nicknames: []*people.Nickname{
			{Value: "dave"},
		},
		Memberships: []*people.Membership{
			{
				ContactGroupMembership: &people.ContactGroupMembership{
					ContactGroupResourceName: "contactGroups/g1",
				},
			},
		},
	}

	c := personToContact(p, "google:work", groupMap)
	if c.ID != "google:work:people/c12345" {
		t.Errorf("expected ID google:work:people/c12345, got %q", c.ID)
	}
	if c.Name != "Dave Rivers" {
		t.Errorf("expected Name Dave Rivers, got %q", c.Name)
	}
	if len(c.Emails) != 1 || c.Emails[0] != "rivers@cs.example.edu" {
		t.Errorf("expected email rivers@cs.example.edu, got %v", c.Emails)
	}
	if len(c.Nicknames) != 1 || c.Nicknames[0] != "dave" {
		t.Errorf("expected nickname dave, got %v", c.Nicknames)
	}
	if len(c.Groups) != 1 || c.Groups[0] != "algo" {
		t.Errorf("expected group algo, got %v", c.Groups)
	}
}

func TestFetchGroupMap_Mock(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"contactGroups": [
				{"resourceName": "contactGroups/g1", "name": "algo", "formattedName": "algo", "groupType": "USER_CONTACT_GROUP"},
				{"resourceName": "contactGroups/sys", "name": "myContacts", "formattedName": "Contacts", "groupType": "SYSTEM_CONTACT_GROUP"}
			]
		}`))
	}))
	defer ts.Close()

	srv, err := people.NewService(context.Background(), option.WithEndpoint(ts.URL), option.WithHTTPClient(ts.Client()))
	if err != nil {
		t.Fatalf("failed to create people service: %v", err)
	}

	gMap := fetchGroupMap(context.Background(), srv)
	if gMap["contactGroups/g1"] != "algo" {
		t.Errorf("expected contactGroups/g1 to be 'algo', got %q", gMap["contactGroups/g1"])
	}
	if _, ok := gMap["contactGroups/sys"]; ok {
		t.Errorf("expected system group myContacts/Contacts to be excluded")
	}
}
