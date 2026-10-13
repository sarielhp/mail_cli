package interactive

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeMessage is one synthetic message a fake JMAP server holds.
type fakeMessage struct {
	id, subject, from string
	date              time.Time
}

// raw is the message as RFC 5322 text, which the server serves as its blob.
func (m fakeMessage) raw(to string) string {
	return strings.Join([]string{
		"From: " + m.from,
		"To: " + to,
		"Subject: " + m.subject,
		"Date: " + m.date.Format(time.RFC1123Z),
		"Message-ID: <" + m.id + "@example.com>",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"This is the synthetic message " + m.subject + ".",
		"",
	}, "\r\n")
}

// mailboxMove is what one Email/set did to one message: the mailboxes it was
// added to and removed from, or that it was destroyed.
type mailboxMove struct {
	id        string
	added     []string
	removed   []string
	destroyed bool
}

// fakeJMAP is a JMAP server on 127.0.0.1 holding one account. It implements
// only what the jmap backend calls -- the session resource, Mailbox/get,
// Email/query, Email/get, Email/set and blob download -- and records every
// Email/set that changes a mailbox, so a test can say which server was asked
// to move which message.
type fakeJMAP struct {
	t        *testing.T
	srv      *httptest.Server
	user     string
	mailbox  map[string]string // mailbox id -> name
	mu       sync.Mutex
	messages map[string]fakeMessage
	in       map[string]map[string]bool // message id -> mailbox ids
	moves    []mailboxMove
	unknown  []string
}

// fakeMailboxes are every fake account's mailboxes, by id: the inbox, and the
// folders archive and delete move mail into.
var fakeMailboxes = map[string]string{
	"mb-inbox": "Inbox", "mb-archive": "Archive", "mb-trash": "Trash",
	"mb-junk": "Junk", "mb-sent": "Sent", "mb-drafts": "Drafts",
}

// newFakeJMAP starts a server whose Inbox holds msgs; it stops with t.
func newFakeJMAP(t *testing.T, user string, msgs ...fakeMessage) *fakeJMAP {
	t.Helper()
	f := &fakeJMAP{t: t, user: user, mailbox: fakeMailboxes,
		messages: map[string]fakeMessage{}, in: map[string]map[string]bool{}}
	for _, m := range msgs {
		f.messages[m.id] = m
		f.in[m.id] = map[string]bool{"mb-inbox": true}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /session", f.session)
	mux.HandleFunc("POST /api", f.api)
	mux.HandleFunc("GET /download/{account}/{blob}/{name}", f.download)
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// account is the config.json entry that points mail_cli at this server.
func (f *fakeJMAP) account(name string) map[string]any {
	return map[string]any{
		"name":             name,
		"type":             "jmap",
		"username":         f.user,
		"password":         "fake-token-" + name,
		"jmap_session_url": f.srv.URL + "/session",
		"spam_folder":      "Junk",
		"received_folder":  "Archive",
	}
}

func (f *fakeJMAP) session(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{
		"capabilities": map[string]any{
			"urn:ietf:params:jmap:core": map[string]any{"maxSizeUpload": 50000000, "maxObjectsInGet": 500, "maxObjectsInSet": 500},
			"urn:ietf:params:jmap:mail": map[string]any{},
		},
		"accounts": map[string]any{"acc1": map[string]any{
			"name": f.user, "isPersonal": true,
			"accountCapabilities": map[string]any{"urn:ietf:params:jmap:mail": map[string]any{}},
		}},
		"primaryAccounts": map[string]any{"urn:ietf:params:jmap:mail": "acc1"},
		"username":        f.user,
		"apiUrl":          f.srv.URL + "/api",
		"downloadUrl":     f.srv.URL + "/download/{accountId}/{blobId}/{name}?type={type}",
		"uploadUrl":       f.srv.URL + "/upload/{accountId}",
		"eventSourceUrl":  f.srv.URL + "/events",
		"state":           "s1",
	})
}

// api answers a JMAP request, one method call at a time.
func (f *fakeJMAP) api(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MethodCalls [][3]json.RawMessage `json:"methodCalls"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	responses := make([]any, 0, len(req.MethodCalls))
	for _, call := range req.MethodCalls {
		var name, callID string
		_ = json.Unmarshal(call[0], &name)
		_ = json.Unmarshal(call[2], &callID)
		method, args := f.call(name, call[1])
		responses = append(responses, []any{method, args, callID})
	}
	writeJSON(w, map[string]any{"methodResponses": responses, "sessionState": "s1"})
}

// call performs one method; an unknown method answers a JMAP error and is
// recorded, so a test failure can name what the backend asked for.
func (f *fakeJMAP) call(name string, raw json.RawMessage) (string, any) {
	var args struct {
		IDs    []string `json:"ids"`
		Filter struct {
			InMailbox string `json:"inMailbox"`
		} `json:"filter"`
		Update  map[string]map[string]any `json:"update"`
		Destroy []string                  `json:"destroy"`
	}
	_ = json.Unmarshal(raw, &args)
	switch name {
	case "Mailbox/get":
		return name, map[string]any{"accountId": "acc1", "state": "m1", "list": f.mailboxList(), "notFound": []string{}}
	case "Email/query":
		ids := f.query(args.Filter.InMailbox)
		return name, map[string]any{"accountId": "acc1", "queryState": "q1", "position": 0, "total": len(ids), "ids": ids}
	case "Email/get":
		return name, f.get(args.IDs)
	case "Email/set":
		return name, f.set(args.Update, args.Destroy)
	}
	f.unknown = append(f.unknown, name)
	return "error", map[string]any{"type": "unknownMethod"}
}

func (f *fakeJMAP) mailboxList() []any {
	var list []any
	for id, name := range f.mailbox {
		total := 0
		for _, in := range f.in {
			if in[id] {
				total++
			}
		}
		list = append(list, map[string]any{
			"id": id, "name": name, "parentId": nil, "role": strings.ToLower(name),
			"totalEmails": total, "unreadEmails": total, "isSubscribed": true,
		})
	}
	return list
}

// query lists the messages in mailbox, newest first; no mailbox lists all.
func (f *fakeJMAP) query(mailbox string) []string {
	ids := []string{}
	for id, in := range f.in {
		if mailbox == "" || in[mailbox] {
			ids = append(ids, id)
		}
	}
	slices.SortFunc(ids, func(a, b string) int {
		return f.messages[b].date.Compare(f.messages[a].date)
	})
	return ids
}

func (f *fakeJMAP) get(ids []string) map[string]any {
	list, notFound := []any{}, []string{}
	for _, id := range ids {
		m, ok := f.messages[id]
		in, exists := f.in[id]
		if !ok || !exists {
			notFound = append(notFound, id)
			continue
		}
		list = append(list, map[string]any{
			"id": id, "blobId": "blob-" + id, "threadId": "t-" + id,
			"mailboxIds": in, "keywords": map[string]bool{},
			"subject": m.subject, "receivedAt": m.date.UTC().Format(time.RFC3339),
		})
	}
	return map[string]any{"accountId": "acc1", "state": "e1", "list": list, "notFound": notFound}
}

// set applies and records mailbox changes and destroys. A message this server
// does not hold is refused as notFound, as a real server would.
func (f *fakeJMAP) set(update map[string]map[string]any, destroy []string) map[string]any {
	updated, notUpdated := map[string]any{}, map[string]any{}
	for id, patch := range update {
		in, ok := f.in[id]
		if !ok {
			notUpdated[id] = map[string]any{"type": "notFound"}
			continue
		}
		mv := mailboxMove{id: id}
		for key, val := range patch {
			mb, isMailbox := strings.CutPrefix(key, "mailboxIds/")
			if !isMailbox {
				continue
			}
			if val == true {
				in[mb] = true
				mv.added = append(mv.added, mb)
			} else {
				delete(in, mb)
				mv.removed = append(mv.removed, mb)
			}
		}
		if len(mv.added)+len(mv.removed) > 0 {
			f.moves = append(f.moves, mv)
		}
		updated[id] = nil
	}
	destroyed, notDestroyed := []string{}, map[string]any{}
	for _, id := range destroy {
		if _, ok := f.in[id]; !ok {
			notDestroyed[id] = map[string]any{"type": "notFound"}
			continue
		}
		delete(f.in, id)
		destroyed = append(destroyed, id)
		f.moves = append(f.moves, mailboxMove{id: id, destroyed: true})
	}
	return map[string]any{"accountId": "acc1", "oldState": "e1", "newState": "e2",
		"updated": updated, "notUpdated": notUpdated, "destroyed": destroyed, "notDestroyed": notDestroyed}
}

func (f *fakeJMAP) download(w http.ResponseWriter, r *http.Request) {
	id, ok := strings.CutPrefix(path.Clean(r.PathValue("blob")), "blob-")
	f.mu.Lock()
	m, known := f.messages[id]
	f.mu.Unlock()
	if !ok || !known {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "message/rfc822")
	_, _ = fmt.Fprint(w, m.raw(f.user))
}

// Moves is every mailbox change and destroy the server has accepted so far.
func (f *fakeJMAP) Moves() []mailboxMove {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.moves)
}

// WaitMoves waits until the server has recorded at least n moves.
func (f *fakeJMAP) WaitMoves(n int) []mailboxMove {
	f.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		moves := f.Moves()
		if len(moves) >= n {
			return moves
		}
		if time.Now().After(deadline) {
			f.mu.Lock()
			unknown := slices.Clone(f.unknown)
			f.mu.Unlock()
			f.t.Fatalf("%s: got %d mailbox change(s) %+v, want %d; unknown methods called: %q",
				f.user, len(moves), moves, n, unknown)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
