package contacts

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
)

type muttRecipient struct {
	Name     string
	Email    string
	AliasRef string
}

type muttEntry struct {
	Key        string
	Recipients []muttRecipient
}

var (
	aliasLineRe = regexp.MustCompile(`^\s*alias\s+(\S+)\s+(.*)$`)
	nameEmailRe = regexp.MustCompile(`^(.*?)\s*<([^>]+)>$`)
	bareEmailRe = regexp.MustCompile(`^[^\s<>@]+@[^\s<>@]+$`)
)

// ParseMuttFile parses a Mutt alias file on disk into a list of Contacts.
func ParseMuttFile(filepath string) ([]Contact, error) {
	f, err := os.Open(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to open mutt alias file %s: %w", filepath, err)
	}
	defer f.Close()

	return ParseMuttAliases(f, "mutt")
}

// ParseMuttAliases parses Mutt aliases from an io.Reader.
func ParseMuttAliases(r io.Reader, sourceName string) ([]Contact, error) {
	rawAliases, err := readRawAliases(r)
	if err != nil {
		return nil, err
	}

	entries := make(map[string]muttEntry, len(rawAliases))
	for key, val := range rawAliases {
		recs := parseMuttValue(val)
		entries[key] = muttEntry{Key: key, Recipients: recs}
	}

	flattened := flattenMuttEntries(entries)
	return buildContactsFromFlattened(flattened, sourceName), nil
}

func readRawAliases(r io.Reader) (map[string]string, error) {
	scanner := bufio.NewScanner(r)
	raw := make(map[string]string)
	var currentKey string
	var currentLines []string

	for scanner.Scan() {
		line := scanner.Text()
		idx := strings.Index(line, "#")
		if idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if m := aliasLineRe.FindStringSubmatch(line); m != nil {
			if currentKey != "" {
				raw[currentKey] = strings.Join(currentLines, " ")
			}
			currentKey = m[1]
			currentLines = []string{m[2]}
			continue
		}

		if currentKey != "" {
			currentLines = append(currentLines, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading aliases: %w", err)
	}
	if currentKey != "" {
		raw[currentKey] = strings.Join(currentLines, " ")
	}

	return raw, nil
}

func parseMuttValue(str string) []muttRecipient {
	clean := strings.ReplaceAll(str, "\\", " ")
	clean = strings.TrimSpace(clean)

	tokens := splitTokensByCommas(clean)
	if len(tokens) == 1 && !strings.Contains(tokens[0], "<") && !strings.Contains(tokens[0], "@") {
		subTokens := strings.Fields(tokens[0])
		if len(subTokens) > 1 {
			tokens = subTokens
		}
	}

	var recipients []muttRecipient
	for _, tok := range tokens {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		if rec, ok := parseSingleRecipient(tok); ok {
			recipients = append(recipients, rec)
		}
	}
	return recipients
}

func splitTokensByCommas(str string) []string {
	var parts []string
	var cur strings.Builder
	inQuote := false
	escaped := false

	for _, ch := range str {
		if escaped {
			cur.WriteRune(ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '"' {
			inQuote = !inQuote
			cur.WriteRune(ch)
			continue
		}
		if ch == ',' && !inQuote {
			p := strings.TrimSpace(cur.String())
			if p != "" {
				parts = append(parts, p)
			}
			cur.Reset()
			continue
		}
		cur.WriteRune(ch)
	}

	p := strings.TrimSpace(cur.String())
	if p != "" {
		parts = append(parts, p)
	}
	return parts
}

func parseSingleRecipient(tok string) (muttRecipient, bool) {
	if m := nameEmailRe.FindStringSubmatch(tok); m != nil {
		name := strings.TrimSpace(m[1])
		name = strings.Trim(name, `"'`)
		email := strings.TrimSpace(m[2])
		return muttRecipient{Name: name, Email: email}, true
	}

	if bareEmailRe.MatchString(tok) {
		return muttRecipient{Email: tok}, true
	}

	if !strings.Contains(tok, "@") && !strings.ContainsAny(tok, " \t") {
		return muttRecipient{AliasRef: tok}, true
	}

	return muttRecipient{Name: tok, AliasRef: tok}, true
}

func flattenMuttEntries(entries map[string]muttEntry) map[string][]muttRecipient {
	flattened := make(map[string][]muttRecipient, len(entries))
	for key := range entries {
		visited := make(map[string]bool)
		flattened[key] = resolveMuttKey(key, entries, visited)
	}
	return flattened
}

func resolveMuttKey(key string, entries map[string]muttEntry, visited map[string]bool) []muttRecipient {
	if visited[key] {
		return nil
	}
	visited[key] = true

	entry, ok := entries[key]
	if !ok {
		return nil
	}

	var resolved []muttRecipient
	seenEmail := make(map[string]bool)

	for _, rec := range entry.Recipients {
		if rec.AliasRef != "" {
			childVisited := copyVisitedMap(visited)
			nested := resolveMuttKey(rec.AliasRef, entries, childVisited)
			for _, n := range nested {
				e := strings.ToLower(n.Email)
				if e != "" && !seenEmail[e] {
					seenEmail[e] = true
					resolved = append(resolved, n)
				}
			}
			continue
		}

		e := strings.ToLower(rec.Email)
		if e != "" && !seenEmail[e] {
			seenEmail[e] = true
			resolved = append(resolved, rec)
		} else if e == "" && rec.Name != "" {
			resolved = append(resolved, rec)
		}
	}

	return resolved
}

func copyVisitedMap(src map[string]bool) map[string]bool {
	dst := make(map[string]bool, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func buildContactsFromFlattened(flattened map[string][]muttRecipient, sourceName string) []Contact {
	var contacts []Contact

	keys := make([]string, 0, len(flattened))
	for k := range flattened {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		recipients := flattened[key]
		if len(recipients) == 0 {
			continue
		}

		if len(recipients) == 1 {
			r := recipients[0]
			name := r.Name
			if name == "" {
				name = key
			}
			var emails []string
			if r.Email != "" {
				emails = []string{r.Email}
			}
			contacts = append(contacts, Contact{
				ID:        fmt.Sprintf("%s:%s", sourceName, key),
				Name:      name,
				Emails:    emails,
				Nicknames: []string{key},
				Source:    sourceName,
			})
			continue
		}

		// Group alias
		var emails []string
		for _, r := range recipients {
			if r.Email != "" {
				emails = append(emails, r.Email)
			}
		}

		contacts = append(contacts, Contact{
			ID:        fmt.Sprintf("%s:group:%s", sourceName, key),
			Name:      fmt.Sprintf("Group: %s", key),
			Emails:    emails,
			Nicknames: []string{key},
			Groups:    []string{key},
			Source:    sourceName,
		})
	}

	return contacts
}
