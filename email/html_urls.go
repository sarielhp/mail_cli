package email

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

var htmlURLRegex = regexp.MustCompile(`https?://[^\s<>"']+`)

// protectHTMLURLs substitutes short words so w3m cannot split URL text.
// The original URLs are restored as plain text, not terminal escapes.
//
// A token is itself an absolute URL because it also replaces href targets,
// which w3m lists under References: a bare word there is a relative link,
// and w3m resolved it against the working directory, so every restored
// target read file:///<cwd>/https://...
func protectHTMLURLs(source string) (string, func(string) string) {
	prefix := "murl"
	lower := strings.ToLower(source)
	for strings.Contains(lower, prefix) {
		prefix += "z"
	}
	var replacements []string
	protected := htmlURLRegex.ReplaceAllStringFunc(source, func(target string) string {
		token := fmt.Sprintf("https://%s%dx.invalid/", prefix, len(replacements)/2)
		replacements = append(replacements, token, html.UnescapeString(target))
		return token
	})
	return protected, strings.NewReplacer(replacements...).Replace
}
