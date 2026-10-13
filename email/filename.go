package email

import (
	"regexp"
	"strings"
)

var (
	// subjectNoiseRe matches one leading reply or forward marker -- Re:,
	// Fwd:, Fw:, and the German and French Aw:, Wg:, Tr:, optionally numbered
	// as in Re[2]: -- or one bracketed list tag such as [list-announce].
	subjectNoiseRe = regexp.MustCompile(`(?i)^\s*(?:(?:re|fwd?|aw|wg|tr)\s*(?:\[\d+\])?\s*:|\[[^\]]*\])\s*`)

	// nonNameRe is everything that does not belong in a file name. A hyphen
	// is in it on purpose: names use underscores throughout.
	nonNameRe = regexp.MustCompile(`[^A-Za-z0-9.]+`)
)

// FileStemLimit is how long a saved message's name is, before the
// extension, when that is enough to tell it apart (see ShortStem).
const FileStemLimit = 30

// maxStem bounds a stem well under any file system's name limit.
const maxStem = 120

// SubjectFileStem turns a subject into a file name stem: reply and forward
// markers and list tags are dropped from the front, and every run of
// anything other than letters, digits and dots becomes a single underscore.
// It does not shorten the result to fit a directory; ShortStem does that.
//
// The export command and the TUI's message save both name files with it, so
// a message saved either way gets the same name.
func SubjectFileStem(subj string) string {
	s := strings.TrimSpace(subj)
	for {
		t := subjectNoiseRe.ReplaceAllString(s, "")
		if t == s {
			break
		}
		s = t
	}
	s = strings.Trim(nonNameRe.ReplaceAllString(s, "_"), "._")
	if len(s) > maxStem {
		s = strings.TrimRight(s[:maxStem], "._")
	}
	if s == "" {
		return "no_subject"
	}
	return s
}

// ShortStem cuts stem to limit bytes when that prefix is unique among taken
// -- the names already in the directory -- and otherwise lengthens it a
// character at a time until it is. A stem that never becomes unique comes
// back whole; the writer then adds a _2 suffix. Comparison ignores case,
// since some file systems do.
func ShortStem(stem string, limit int, taken []string) string {
	if len(stem) <= limit {
		return stem
	}
	lower := make([]string, len(taken))
	for i, t := range taken {
		lower[i] = strings.ToLower(t)
	}
	for n := limit; n < len(stem); n++ {
		prefix := strings.TrimRight(stem[:n], "._")
		if prefix != "" && !anyHasPrefix(lower, strings.ToLower(prefix)) {
			return prefix
		}
	}
	return stem
}

func anyHasPrefix(names []string, prefix string) bool {
	for _, n := range names {
		if strings.HasPrefix(n, prefix) {
			return true
		}
	}
	return false
}
