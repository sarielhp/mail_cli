package email

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// ShortID is the eight-hex-digit abbreviation of a backend message ID that
// listings show and commands accept: the first four bytes of its SHA-256.
//
// It is a hash, not a prefix, so only something that has seen the full ID can
// expand it -- the local cache, or a folder listing. A server cannot. Commands
// that fell back to asking the server for an argument they could not resolve
// locally passed a short ID along unchanged and reported "not found", which
// read as "no such message" rather than "not downloaded yet".
type ShortID string

const shortIDLen = 8

// ComputeShortID abbreviates a backend message ID.
func ComputeShortID(id ID) ShortID {
	h := sha256.Sum256([]byte(id))
	return ShortID(strings.ToUpper(hex.EncodeToString(h[:4])))
}

// ParseShortID reports whether s has the form of a short ID, and returns it
// normalized. Having the form does not mean it names anything.
func ParseShortID(s string) (ShortID, bool) {
	s = strings.TrimSpace(s)
	if len(s) != shortIDLen {
		return "", false
	}
	if _, err := hex.DecodeString(s); err != nil {
		return "", false
	}
	return ShortID(strings.ToUpper(s)), true
}

// Matches reports whether ref, as typed, is this short ID.
func (s ShortID) Matches(ref string) bool {
	return strings.EqualFold(string(s), strings.TrimSpace(ref))
}

// IDMatches reports whether ref, as a user typed it, names the message whose
// backend ID is id: the full ID ignoring case, or its short ID. Every lookup
// of a typed message ID goes through it, so they cannot disagree.
func IDMatches(id ID, ref string) bool {
	ref = strings.TrimSpace(ref)
	return strings.EqualFold(string(id), ref) || ComputeShortID(id).Matches(ref)
}
