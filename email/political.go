package email

import (
	"regexp"
	"strings"
	"sync"
)

// containsPhrase reports whether text contains phrase at word boundaries.
//
// The boundary stops a keyword matching inside a longer word ("gop" inside
// "gopher"), and is applied to the phrase as a whole so multi-word phrases such
// as "paid for by" still match.
func containsPhrase(text, phrase string) bool {
	re, ok := phraseCache.Load(phrase)
	if !ok {
		compiled := regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}])` + regexp.QuoteMeta(phrase) + `(?:[^\p{L}\p{N}]|$)`)
		re, _ = phraseCache.LoadOrStore(phrase, compiled)
	}
	return re.(*regexp.Regexp).MatchString(text)
}

var phraseCache sync.Map // phrase -> *regexp.Regexp

var politicalInstantBlocks = map[string]string{
	"actblue.com":                          "ActBlue donation link",
	"winred.com":                           "WinRed donation link",
	"paid for by":                          "Political PAC disclosure ('paid for by')",
	"contributions are not tax deductible": "Non-tax-deductible disclosure",
	"contributions or gifts to":            "Donation policy statement",
	"not authorized by any candidate":      "Independent expenditure statement",
	"political action committee":           "PAC reference",
}

var publicDomains = map[string]bool{
	"gmail.com":      true,
	"yahoo.com":      true,
	"outlook.com":    true,
	"hotmail.com":    true,
	"proton.me":      true,
	"protonmail.com": true,
	"icloud.com":     true,
	"aol.com":        true,
	"zoho.com":       true,
	"gmx.com":        true,
	"mail.com":       true,
	"yandex.com":     true,
}

// DetectPolitical reports whether a message is political fundraising spam.
//
// Detection is deliberately limited to instant blocks: a donation-platform
// domain (ActBlue, WinRed) or a PAC disclosure phrase ("paid for by",
// "contributions are not tax deductible"). These are near-certain markers.
//
// It used to also score generic political vocabulary -- "democrat", "congress",
// "election", "campaign", "ballot", plus donation and urgency words -- against a
// threshold. Those are ordinary English, and the scorer filed routine mail as
// political spam far more often than it caught a real appeal, so the vocabulary
// scoring was removed. A message must now contain an instant block to be
// labelled political.
func DetectPolitical(subject, body string) (bool, float64, []string) {
	subjectLower := strings.ToLower(subject)
	bodyLower := strings.ToLower(body)

	const maxTextLen = 8192
	fullText := subjectLower + " " + bodyLower
	if len(fullText) > maxTextLen {
		fullText = fullText[:maxTextLen]
	}

	var score float64
	var triggered []string
	for keyword, reason := range politicalInstantBlocks {
		if containsPhrase(fullText, keyword) {
			score += 20.0
			triggered = append(triggered, reason)
		}
	}

	isPolitical := len(triggered) > 0
	return isPolitical, score, triggered
}

// IsSafeToAutoBlacklist evaluates if a political sender is safe to blacklist based on conservative criteria.
func IsSafeToAutoBlacklist(senderEmail, listUnsubscribe string, score float64) bool {
	if listUnsubscribe == "" {
		return false
	}
	if score < 15.0 {
		return false
	}

	parts := strings.Split(senderEmail, "@")
	if len(parts) != 2 {
		return false
	}
	domain := strings.ToLower(strings.TrimSpace(parts[1]))

	return !publicDomains[domain]
}
