package contacts

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

// ScoredContact wraps Contact with interaction frequency, recency, and relevance score.
type ScoredContact struct {
	Contact
	Frequency  int       `json:"frequency,omitempty"`
	LastSeen   time.Time `json:"last_seen,omitempty"`
	Score      float64   `json:"score,omitempty"`
	IsSent     bool      `json:"is_sent,omitempty"`
	IsArchived bool      `json:"is_archived,omitempty"`
}

// Suggest finds and ranks contact suggestions matching query from curated contacts and history.
func Suggest(book *Book, history *HistoryBook, blacklist []string, query string, limit int) []ScoredContact {
	if limit <= 0 {
		limit = 15
	}
	cleanQuery := strings.TrimSpace(strings.ToLower(query))

	blacklistedMap := make(map[string]bool, len(blacklist))
	for _, b := range blacklist {
		blacklistedMap[strings.ToLower(strings.TrimSpace(b))] = true
	}

	merged := make(map[string]*ScoredContact)
	ingestCurated(book, cleanQuery, blacklistedMap, merged)
	ingestHistory(history, cleanQuery, blacklistedMap, merged)

	results := make([]ScoredContact, 0, len(merged))
	for _, sc := range merged {
		results = append(results, *sc)
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].PrimaryEmail() < results[j].PrimaryEmail()
	})

	if len(results) > limit {
		results = results[:limit]
	}
	return results
}

func ingestCurated(book *Book, cleanQuery string, blacklist map[string]bool, merged map[string]*ScoredContact) {
	if book == nil {
		return
	}
	for _, c := range book.Contacts {
		primary := strings.ToLower(c.PrimaryEmail())
		if primary == "" || isBlacklisted(primary, blacklist) {
			continue
		}
		match, isExactAlias := evaluateMatch(c, cleanQuery)
		if !match {
			continue
		}

		baseScore := 500.0
		if isExactAlias {
			baseScore += 1000.0
		}

		merged[primary] = &ScoredContact{
			Contact: c,
			Score:   baseScore,
		}
	}
}

func ingestHistory(history *HistoryBook, cleanQuery string, blacklist map[string]bool, merged map[string]*ScoredContact) {
	if history == nil {
		return
	}
	now := time.Now().UTC()
	for _, h := range history.Entries {
		primary := strings.ToLower(h.PrimaryEmail())
		if primary == "" || isBlacklisted(primary, blacklist) {
			continue
		}

		existing, found := merged[primary]
		if found {
			mergeExistingHistory(existing, h, now)
			continue
		}

		match, _ := evaluateMatch(h.Contact, cleanQuery)
		if !match {
			continue
		}
		merged[primary] = newHistoryContact(h, now)
	}
}

func mergeExistingHistory(existing *ScoredContact, h ScoredContact, now time.Time) {
	existing.Frequency += h.Frequency
	if h.LastSeen.After(existing.LastSeen) {
		existing.LastSeen = h.LastSeen
	}
	if h.IsSent {
		existing.IsSent = true
	}
	if h.IsArchived {
		existing.IsArchived = true
	}
	existing.Score += computeHistoryScore(h, now)
}

func newHistoryContact(h ScoredContact, now time.Time) *ScoredContact {
	return &ScoredContact{
		Contact:    h.Contact,
		Frequency:  h.Frequency,
		LastSeen:   h.LastSeen,
		IsSent:     h.IsSent,
		IsArchived: h.IsArchived,
		Score:      computeHistoryScore(h, now),
	}
}

func isBlacklisted(email string, blacklist map[string]bool) bool {
	if blacklist[email] {
		return true
	}
	parts := strings.Split(email, "@")
	if len(parts) == 2 {
		domain := parts[1]
		if blacklist[domain] || blacklist["@"+domain] {
			return true
		}
	}
	return false
}

func evaluateMatch(c Contact, query string) (bool, bool) {
	if query == "" {
		return true, false
	}

	// Exact nickname / alias check
	for _, n := range c.Nicknames {
		if strings.EqualFold(n, query) {
			return true, true
		}
		if strings.HasPrefix(strings.ToLower(n), query) {
			return true, false
		}
	}

	// Substring match on name
	if strings.Contains(strings.ToLower(c.Name), query) {
		return true, false
	}

	// Match on emails
	for _, e := range c.Emails {
		if strings.Contains(strings.ToLower(e), query) {
			return true, false
		}
	}

	// Match on groups
	for _, g := range c.Groups {
		if strings.Contains(strings.ToLower(g), query) {
			return true, false
		}
	}

	return false, false
}

func computeHistoryScore(h ScoredContact, now time.Time) float64 {
	score := 0.0

	// Frequency weight (up to 300 pts)
	cappedFreq := h.Frequency
	if cappedFreq > 20 {
		cappedFreq = 20
	}
	score += float64(cappedFreq * 15)

	// Direct sent boost (strongest outbound trust signal)
	if h.IsSent {
		score += 200.0
	}

	// Manual archive boost (intentional keep signal)
	if h.IsArchived {
		score += 75.0
	}

	// Recency decay boost
	if !h.LastSeen.IsZero() {
		age := now.Sub(h.LastSeen)
		switch {
		case age <= 7*24*time.Hour:
			score += 100.0
		case age <= 30*24*time.Hour:
			score += 50.0
		case age <= 90*24*time.Hour:
			score += 20.0
		case age <= 365*24*time.Hour:
			score += 5.0
		}
	}

	return score
}

// FormatSuggestTable renders ranked suggestions in a clean tabular view.
func FormatSuggestTable(w io.Writer, suggestions []ScoredContact) error {
	if len(suggestions) == 0 {
		fmt.Fprintln(w, "No matching suggestions.")
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tEMAIL\tALIASES\tGROUPS\tSCORE\tSOURCE")

	for _, sc := range suggestions {
		name := sc.Name
		if name == "" {
			name = "-"
		}
		email := sc.PrimaryEmail()
		if email == "" {
			email = "-"
		}
		aliases := strings.Join(sc.Nicknames, ", ")
		if aliases == "" {
			aliases = "-"
		}
		groups := strings.Join(sc.Groups, ", ")
		if groups == "" {
			groups = "-"
		}
		source := sc.Source
		if source == "" {
			source = "-"
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%.0f\t%s\n", name, email, aliases, groups, sc.Score, source)
	}

	return tw.Flush()
}
