package tui

import (
	"github.com/sarielhp/mail_cli/email"
	"sort"
	"strings"
	"unicode"
)

type fuzzyMatch struct {
	email email.Email
	score int
}

func fuzzyScoreRunes(qRunes []rune, target string) int {
	if len(qRunes) == 0 {
		return 0
	}

	qi := 0
	score := 0
	prevMatched := false
	wordBoundary := true

	for ti, r := range target {
		if qi >= len(qRunes) {
			break
		}
		if unicode.ToLower(r) == qRunes[qi] {
			score += 10
			if prevMatched {
				score += 5
			}
			if wordBoundary {
				score += 3
			}
			if ti == 0 {
				score += 2
			}
			prevMatched = true
			qi++
		} else {
			prevMatched = false
		}
		wordBoundary = !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}

	if qi < len(qRunes) {
		return 0
	}

	return score
}

func fuzzyScore(query, target string) int {
	if query == "" {
		return 0
	}
	return fuzzyScoreRunes([]rune(strings.ToLower(query)), target)
}

func fuzzyMatchEmailRunes(qRunes []rune, e email.Email) int {
	score := fuzzyScoreRunes(qRunes, e.Subject)
	if score == 0 {
		score = fuzzyScoreRunes(qRunes, e.FromRaw)
	}
	if score == 0 {
		score = fuzzyScoreRunes(qRunes, e.FromEmail)
	}
	if score == 0 {
		score = fuzzyScoreRunes(qRunes, string(e.ID))
	}
	return score
}

func fuzzyFilterEmails(query string, emails []email.Email) []email.Email {
	if query == "" {
		return emails
	}

	qRunes := []rune(strings.ToLower(query))
	var matches []fuzzyMatch
	for _, e := range emails {
		if score := fuzzyMatchEmailRunes(qRunes, e); score > 0 {
			matches = append(matches, fuzzyMatch{email: e, score: score})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].email.EmailDate.After(matches[j].email.EmailDate)
	})

	result := make([]email.Email, 0, len(matches))
	for _, m := range matches {
		result = append(result, m.email)
	}
	return result
}
