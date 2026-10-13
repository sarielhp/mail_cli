package organize

import (
	"fmt"
	"github.com/sarielhp/mail_cli/label"
	"sort"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/mailclient"
)

// ServerDraft is one message in the account's Drafts folder as the drafts
// command shows it. To comes from the message itself: a summary carries the
// sender, and for a draft the sender is always us.
type ServerDraft struct {
	ID        email.ID
	ShortID   email.ShortID
	Subject   string
	To        string
	MessageID string
	Date      time.Time
	// Unreadable marks a draft that could not be fetched or parsed. It is
	// still listed, so it can be removed by ID, but a prune never touches it.
	Unreadable bool
}

// DraftClient is what the server-draft operations need from a backend.
type DraftClient interface {
	mailclient.LabelReader
	mailclient.BackendInfo
	mailclient.EmailFetcher
	mailclient.MessageAccessor
	mailclient.EmailWriter
}

// ListServerDrafts lists the Drafts folder, newest first, and names the folder.
func ListServerDrafts(client DraftClient) (label.Folder, []ServerDraft, error) {
	folder, err := ResolveDraftsTarget(client)
	if err != nil {
		return label.Folder{}, nil, err
	}
	drafts, err := listFolder(client, folder)
	return folder, drafts, err
}

// ListSentMessages lists the Sent folder the same way, bounded by the
// account's fetch limit like every folder listing.
func ListSentMessages(client DraftClient) ([]ServerDraft, error) {
	folder, err := ResolveSentTarget(client)
	if err != nil {
		return nil, err
	}
	return listFolder(client, folder)
}

// listFolder reads each message in turn rather than as one batch: Sent can
// hold large attachments, and one message at a time is all a header needs.
func listFolder(client DraftClient, folder label.Folder) ([]ServerDraft, error) {
	ids, err := client.FetchAndDownloadEmails(folder, cfg_g.SanitizeLabelForCache(folder.String()))
	if err != nil {
		return nil, err
	}
	out := make([]ServerDraft, 0, len(ids))
	for _, id := range ids {
		cm, err := readCompose(client, id)
		if err != nil {
			out = append(out, ServerDraft{ID: id, ShortID: email.ComputeShortID(id), Subject: "(unreadable)", Unreadable: true})
			continue
		}
		out = append(out, ServerDraft{
			ID:        id,
			ShortID:   email.ComputeShortID(id),
			Subject:   strings.TrimSpace(cm.Subject),
			To:        strings.TrimSpace(cm.To),
			MessageID: strings.TrimSpace(cm.MessageID),
			Date:      cm.Date,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date.After(out[j].Date) })
	return out, nil
}

func readCompose(client DraftClient, id email.ID) (*email.ComposeMessage, error) {
	raw, err := client.GetRawMessage(id)
	if err != nil {
		return nil, err
	}
	return email.ParseRFC822ToCompose(raw)
}

// MatchServerDrafts finds the listed drafts the user named, by full ID or by
// the short form the listing shows. A name that matches nothing, or a short
// ID that matches more than one draft, comes back in unknown so the caller
// refuses rather than trashing a guess.
func MatchServerDrafts(drafts []ServerDraft, names []string) ([]ServerDraft, []string) {
	var matched []ServerDraft
	var unknown []string
	for _, name := range names {
		var hits []ServerDraft
		for _, d := range drafts {
			if string(d.ID) == name {
				hits = []ServerDraft{d}
				break
			}
			if d.ShortID.Matches(name) {
				hits = append(hits, d)
			}
		}
		if len(hits) != 1 {
			unknown = append(unknown, name)
			continue
		}
		matched = append(matched, hits[0])
	}
	return matched, unknown
}

// TrashServerDrafts moves drafts out of the Drafts folder into the trash and
// says where they went. The client has no per-message delete, and the trash
// expires on its own.
func TrashServerDrafts(client DraftClient, folder label.Folder, ids []email.ID) (label.Folder, error) {
	if len(ids) == 0 {
		return label.Folder{}, nil
	}
	trash, err := ResolveTrashTarget(client)
	if err != nil {
		return label.Folder{}, err
	}
	if trash.IsZero() || strings.EqualFold(trash.String(), folder.String()) {
		return label.Folder{}, fmt.Errorf("no trash folder to move drafts from %q into", folder)
	}
	return trash, client.MoveEmail(ids, folder, trash)
}

// PrunePlan is what a prune would trash and why.
type PrunePlan struct {
	// Duplicates are older copies of one message: the same Message-ID, which
	// every copy a compose session uploads shares. With BySubject, a draft
	// without a Message-ID also matches one with the same recipients and
	// subject.
	Duplicates []ServerDraft
	// Sent are drafts of messages that were sent (see wasSent).
	Sent []ServerDraft
}

// PruneOptions says which rules a prune applies.
type PruneOptions struct {
	// Sent also trashes drafts whose message is in the Sent folder.
	Sent bool
	// BySubject treats same recipients and subject as the same message when
	// Message-IDs cannot settle it. It can match two distinct messages in a
	// thread, which is why it is opt-in.
	BySubject bool
}

// IDs is every draft the plan would trash, each once.
func (p PrunePlan) IDs() []email.ID {
	seen := map[email.ID]bool{}
	var ids []email.ID
	for _, d := range append(append([]ServerDraft{}, p.Duplicates...), p.Sent...) {
		if !seen[d.ID] {
			seen[d.ID] = true
			ids = append(ids, d.ID)
		}
	}
	return ids
}

// PlanPrune decides which drafts are redundant. drafts must be newest first,
// as ListServerDrafts returns them; sent is consulted only with opts.Sent.
// An unreadable draft is never part of a plan.
func PlanPrune(drafts, sent []ServerDraft, opts PruneOptions) PrunePlan {
	var plan PrunePlan
	var kept []ServerDraft
	for _, d := range drafts {
		if d.Unreadable {
			continue
		}
		if matchesAny(d, kept, opts.BySubject) {
			plan.Duplicates = append(plan.Duplicates, d)
			continue
		}
		kept = append(kept, d)
		if opts.Sent && wasSent(d, sent, opts.BySubject) {
			plan.Sent = append(plan.Sent, d)
		}
	}
	return plan
}

// sameMessage applies the matching hierarchy AGENTS.md sets. Message-IDs are
// authoritative when both exist. The (To, Subject) heuristic runs only when
// one side lacks an ID, only when asked for, and never on an empty recipient
// or subject, which would match every untitled note to oneself.
func sameMessage(a, b ServerDraft, bySubject bool) bool {
	if a.MessageID != "" && b.MessageID != "" {
		return a.MessageID == b.MessageID
	}
	if !bySubject || a.To == "" || a.Subject == "" {
		return false
	}
	return strings.EqualFold(a.To, b.To) && strings.EqualFold(a.Subject, b.Subject)
}

func matchesAny(d ServerDraft, others []ServerDraft, bySubject bool) bool {
	for _, o := range others {
		if sameMessage(d, o, bySubject) {
			return true
		}
	}
	return false
}

// wasSent reports whether the draft's message is in Sent. The heuristic
// form also needs both dates known and the send strictly after the draft,
// so a draft written after the last reply in a thread survives.
func wasSent(d ServerDraft, sent []ServerDraft, bySubject bool) bool {
	for _, s := range sent {
		if d.MessageID != "" && s.MessageID != "" {
			if d.MessageID == s.MessageID {
				return true
			}
			continue
		}
		if d.Date.IsZero() || s.Date.IsZero() || !s.Date.After(d.Date) {
			continue
		}
		if sameMessage(d, s, bySubject) {
			return true
		}
	}
	return false
}

// PlanServerPrune lists the Drafts folder, and Sent when opts.Sent asks for
// it, and returns the folder, how many drafts it holds and what a prune would
// trash. It changes nothing.
func PlanServerPrune(client DraftClient, opts PruneOptions) (label.Folder, int, PrunePlan, error) {
	folder, drafts, err := ListServerDrafts(client)
	if err != nil {
		return label.Folder{}, 0, PrunePlan{}, err
	}
	var sent []ServerDraft
	if opts.Sent {
		if sent, err = ListSentMessages(client); err != nil {
			return label.Folder{}, 0, PrunePlan{}, err
		}
	}
	return folder, len(drafts), PlanPrune(drafts, sent, opts), nil
}

// FindServerDrafts lists the Drafts folder and resolves the names the user
// gave. It changes nothing.
func FindServerDrafts(client DraftClient, names []string) (label.Folder, []ServerDraft, []string, error) {
	folder, drafts, err := ListServerDrafts(client)
	if err != nil {
		return label.Folder{}, nil, nil, err
	}
	matched, unknown := MatchServerDrafts(drafts, names)
	return folder, matched, unknown, nil
}
