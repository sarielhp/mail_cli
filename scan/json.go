package scan

import (
	"encoding/json"
	"io"

	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
)

// FolderReport is the machine-readable result of scanning one folder: the
// classified counts and the per-message verdicts.
type FolderReport struct {
	Account      string          `json:"account"`
	Folder       string          `json:"folder"`
	Total        int             `json:"total"`
	Spam         int             `json:"spam"`
	Ham          int             `json:"ham"`
	Blacklisted  int             `json:"blacklisted"`
	MovedByRules int             `json:"moved_by_rules"`
	Moved        int             `json:"moved"`
	Messages     []MessageReport `json:"messages"`
}

// MessageReport is one scanned message's verdict.
type MessageReport struct {
	ID            string  `json:"id"`
	Subject       string  `json:"subject"`
	From          string  `json:"from"`
	IsSpam        bool    `json:"is_spam"`
	IsBlacklisted bool    `json:"is_blacklisted"`
	IsPolitical   bool    `json:"is_political"`
	Score         float64 `json:"score"`
	Action        string  `json:"action,omitempty"`
}

// MarshalReports writes reports as a single JSON array. A nil slice becomes
// "[]", not "null", so a script can always index it -- and so does a folder's
// messages, which the early returns of an empty or rule-emptied folder left
// unset.
func MarshalReports(w io.Writer, reports []FolderReport) error {
	normalized := make([]FolderReport, len(reports))
	for i, r := range reports {
		if r.Messages == nil {
			r.Messages = []MessageReport{}
		}
		normalized[i] = r
	}
	reports = normalized
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(reports)
}

// buildMessageReports assembles the per-message verdicts for a scanned folder,
// or nothing when JSON output was not requested -- reading every message's
// headers is work worth skipping for the human table, which renders them
// itself. The political flag comes from the same symbol the human listing keys
// on, so the two views of a message cannot disagree about what it is.
func buildMessageReports(config *cfg_g.Config, enabled bool, ids, spamIDs, blacklistedIDs []email.ID, results map[email.ID]*Response) []MessageReport {
	if !enabled {
		return nil
	}
	spamSet := idSet(spamIDs)
	blacklistSet := idSet(blacklistedIDs)
	out := make([]MessageReport, 0, len(ids))
	for _, id := range ids {
		report := MessageReport{
			ID:            string(id),
			IsSpam:        spamSet[id],
			IsBlacklisted: blacklistSet[id],
		}
		if em, err := msg.CachedEmail(config.AccountCache, id); err == nil {
			report.Subject = em.Subject
			report.From = em.FromRaw
		}
		if res := results[id]; res != nil {
			report.Score = res.Score
			report.Action = res.Action
			if _, hasPoli := res.Symbols["CUSTOM_POLITICAL_BLOCK"]; hasPoli {
				report.IsPolitical = true
			}
		}
		out = append(out, report)
	}
	return out
}

func idSet(ids []email.ID) map[email.ID]bool {
	set := make(map[email.ID]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set
}
