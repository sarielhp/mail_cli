package label

import "strings"

// MatchOne finds the single folder name that input designates, searching
// most-specific first: an exact match (ignoring case), then a unique match on a
// "/"-delimited path suffix or on a leading prefix, then a unique substring.
//
// It returns the match and no candidates when exactly one folder answers;
// no match and the candidate list when several do; and nothing at all when none
// does. Deciding what an ambiguous answer means is the caller's: the CLI fails
// with it, and the TUI -- which has to display something -- falls back.
//
// This is the matching half of every label argument in the program. It used to
// be written three times, and the three disagreed: two of them picked an
// arbitrary winner where this one reports the tie.
func MatchOne(names []string, input string) (match string, candidates []string) {
	if input == "" {
		return "", nil
	}
	lower := strings.ToLower(input)

	for _, n := range names {
		if strings.ToLower(n) == lower {
			return n, nil
		}
	}

	suffix := "/" + lower
	var byEdge []string
	for _, n := range names {
		nl := strings.ToLower(n)
		if strings.HasSuffix(nl, suffix) || strings.HasPrefix(nl, lower) {
			byEdge = append(byEdge, n)
		}
	}
	if m, c := decide(byEdge); m != "" || c != nil {
		return m, c
	}

	var bySubstring []string
	for _, n := range names {
		if strings.Contains(strings.ToLower(n), lower) {
			bySubstring = append(bySubstring, n)
		}
	}
	return decide(bySubstring)
}

func decide(candidates []string) (string, []string) {
	candidates = Unique(candidates)
	switch len(candidates) {
	case 0:
		return "", nil
	case 1:
		return candidates[0], nil
	default:
		return "", candidates
	}
}

// Unique removes repeated names, comparing without case, keeping first order.
func Unique(names []string) []string {
	seen := make(map[string]bool, len(names))
	out := make([]string, 0, len(names))
	for _, n := range names {
		if key := strings.ToLower(n); !seen[key] {
			seen[key] = true
			out = append(out, n)
		}
	}
	return out
}

// FullNames projects a folder listing onto the names MatchOne searches.
func FullNames(folders []Item) []string {
	out := make([]string, 0, len(folders))
	for _, f := range folders {
		out = append(out, f.FullName)
	}
	return out
}
