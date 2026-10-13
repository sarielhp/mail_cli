package email

// ID is a message's identifier as its backend assigns it: a Gmail message ID,
// a JMAP email ID, a Graph item ID, an mbox offset key. It is what every
// backend call, cache lookup and folder index is keyed by.
//
// It is a distinct type because three kinds of message identifier share the
// string shape: this one, the ShortID that listings print, and the HeaderID in
// Message-ID. A string a user typed is none of them until resolved, and
// passing one straight to a backend is how `msg show <short id>` used to ask
// the server for a hash it could never know.
type ID string

func (id ID) String() string { return string(id) }

// IDs converts identifiers that arrive untyped -- a server response, a JSON
// file the cache wrote -- in one place rather than at each element.
func IDs(ss []string) []ID {
	if ss == nil {
		return nil
	}
	out := make([]ID, len(ss))
	for i, s := range ss {
		out[i] = ID(s)
	}
	return out
}

// IDStrings is the reverse of IDs, for an API or a file that takes strings.
func IDStrings(ids []ID) []string {
	if ids == nil {
		return nil
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out
}
