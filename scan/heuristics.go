package scan

import (
	"regexp"
	"strings"
	"unicode"
)

// languageScripts maps an allowed_languages entry to the Unicode script it
// permits. Previously a hardcoded switch recognised only english, german,
// french and hebrew: adding "russian" or "chinese" to allowed_languages
// produced no error and no effect, so there was no way to permit a fifth
// script at all.
var languageScripts = map[string]*unicode.RangeTable{
	"english": unicode.Latin, "german": unicode.Latin, "french": unicode.Latin,
	"spanish": unicode.Latin, "italian": unicode.Latin, "portuguese": unicode.Latin,
	"dutch": unicode.Latin, "polish": unicode.Latin, "romanian": unicode.Latin,
	"turkish": unicode.Latin, "latin": unicode.Latin,
	"hebrew":     unicode.Hebrew,
	"arabic":     unicode.Arabic,
	"russian":    unicode.Cyrillic,
	"ukrainian":  unicode.Cyrillic,
	"bulgarian":  unicode.Cyrillic,
	"cyrillic":   unicode.Cyrillic,
	"greek":      unicode.Greek,
	"chinese":    unicode.Han,
	"han":        unicode.Han,
	"japanese":   unicode.Hiragana,
	"korean":     unicode.Hangul,
	"hindi":      unicode.Devanagari,
	"devanagari": unicode.Devanagari,
	"thai":       unicode.Thai,
}

// KnownLanguage reports whether an allowed_languages entry is recognised, so
// config validation can warn instead of silently ignoring a typo.
func KnownLanguage(lang string) bool {
	_, ok := languageScripts[strings.ToLower(strings.TrimSpace(lang))]
	return ok
}

func isRuneAllowed(r rune, whitelist []string) bool {
	if !unicode.IsLetter(r) {
		return true
	}

	for _, lang := range whitelist {
		l := strings.ToLower(strings.TrimSpace(lang))
		if tbl, ok := languageScripts[l]; ok && unicode.Is(tbl, r) {
			return true
		}
		// Japanese text mixes three scripts; treat them as one language.
		if l == "japanese" && (unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Han, r)) {
			return true
		}
	}

	return false
}

func detectScriptLabel(r rune) string {
	if unicode.Is(unicode.Cyrillic, r) {
		return "Cyrillic/Russian"
	}
	if unicode.Is(unicode.Greek, r) {
		return "Greek"
	}
	if unicode.Is(unicode.Han, r) {
		return "Han"
	}
	if unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) {
		return "Japanese"
	}
	if unicode.Is(unicode.Hangul, r) {
		return "Korean"
	}
	if unicode.Is(unicode.Arabic, r) {
		return "Arabic"
	}
	if unicode.Is(unicode.Devanagari, r) {
		return "Hindi/Devanagari"
	}
	if unicode.Is(unicode.Thai, r) {
		return "Thai"
	}
	return "Unknown Script"
}

var urlRegex = regexp.MustCompile(`https?://[^\s<>]+|www\.[^\s<>]+`)

func cleanTextForNLP(text string) string {
	text = urlRegex.ReplaceAllString(text, "")
	var sb strings.Builder
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsSpace(r) {
			sb.WriteRune(r)
		}
	}
	return strings.TrimSpace(sb.String())
}

func isDetectedLanguageWhitelisted(detectedLang string, iso6391 string, whitelist []string) bool {
	detName := strings.ToLower(detectedLang)
	detISO := strings.ToLower(iso6391)

	for _, lang := range whitelist {
		l := strings.ToLower(strings.TrimSpace(lang))
		if l == detName || l == detISO {
			return true
		}
		if (l == "english" && detISO == "en") ||
			(l == "german" && detISO == "de") ||
			(l == "french" && detISO == "fr") ||
			(l == "hebrew" && (detISO == "he" || detISO == "iw")) {
			return true
		}
	}
	return false
}

func isContiguousGreekRunes(runes []rune, idx int) bool {
	if idx > 0 && unicode.Is(unicode.Greek, runes[idx-1]) {
		return true
	}
	if idx < len(runes)-1 && unicode.Is(unicode.Greek, runes[idx+1]) {
		return true
	}
	return false
}

// MinDisallowedScriptLetters is how many letters outside allowed_languages a
// field must contain before it counts as blocked.
//
// The audit previously tripped on the *first* such letter anywhere in the
// subject, the From header, or the first 8KB of body, with no count or ratio.
// One Cyrillic character in a signature, or one Chinese product name in an
// otherwise English subject, was enough to add +20 and mark the message
// "reject (Blocked Language Script)" -- filing legitimate mail as spam
// regardless of what the statistical classifier thought.
const MinDisallowedScriptLetters = 3

// disallowedScript reports the first disallowed letter and its script label
// once at least MinDisallowedScriptLetters of them are present. Isolated Greek
// letters are ignored, as they are overwhelmingly mathematical notation.
func disallowedScript(text string, allowed []string) (blocked bool, firstChar rune, label string) {
	runes := []rune(text)
	count := 0
	var first rune
	var firstLabel string
	for idx, r := range runes {
		if isRuneAllowed(r, allowed) {
			continue
		}
		if unicode.Is(unicode.Greek, r) && !isContiguousGreekRunes(runes, idx) {
			continue
		}
		if count == 0 {
			first = r
			firstLabel = detectScriptLabel(r)
		}
		count++
		if count >= MinDisallowedScriptLetters {
			return true, first, firstLabel
		}
	}
	return false, 0, ""
}
