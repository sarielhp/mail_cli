package label

import "strings"

// ResolveFolder names the folder the TUI should display, and always names one.
//
// It shares MatchOne with the CLI, so a name resolves the same way in both, but
// not the policy: the CLI fails on an ambiguous name and this cannot, because
// something has to be on screen. An ambiguous name therefore falls through to
// the fallback rather than to an arbitrary winner -- the previous version took
// the first substring match in listing order, which is a guess dressed as an
// answer.
func ResolveFolder(folders []Item, name string, fallback string) string {
	names := FullNames(folders)

	if match, _ := MatchOne(names, name); match != "" {
		return match
	}
	if match, _ := MatchOne(names, fallback); match != "" {
		return match
	}
	for _, f := range folders {
		if strings.EqualFold(f.FullName, "INBOX") {
			return f.FullName
		}
	}
	if len(folders) > 0 {
		return folders[0].FullName
	}
	return name
}
