package label

import (
	"errors"
	"fmt"
	"strings"
)

// Sigil marks a label reference on the command line: `%receipts`.
const Sigil = "%"

// ErrReservedName reports a folder name the program refuses to create.
var ErrReservedName = errors.New("reserved label name")

// ValidateNewName reports whether a folder may be created, or renamed to, name.
//
// A name that starts with the sigil cannot be typed back: `%foo` on the
// command line means the folder "foo", so a folder literally named "%foo" is
// reachable only through the `%%foo` escape, and every command that forgot
// to parse its argument created one. Refusing it at creation is what keeps
// that class of mistake from leaving a folder the user then cannot name.
func ValidateNewName(name string) error {
	if strings.HasPrefix(strings.TrimSpace(name), Sigil) {
		return fmt.Errorf("%w: %q starts with %q, which marks a label reference on the command line",
			ErrReservedName, name, Sigil)
	}
	return nil
}

// Folder is a folder name the program has a reason to trust: the server
// reported it, or it was checked as a name to create. It cannot be built from
// a string outside this package except through FromServer or ForCreate, so a
// command-line argument cannot reach a service that takes a Folder without
// passing through resolution -- the mistake that sent "%archive" to the
// server as a folder name.
type Folder struct{ name string }

// FromServer wraps a name the server reported, or one configured as a server
// folder (spam_learn, received_folder). It does not validate: the name already
// exists.
func FromServer(name string) Folder { return Folder{name: name} }

// ForCreate wraps a name the backend will create, refusing what
// ValidateNewName refuses.
func ForCreate(name string) (Folder, error) {
	if err := ValidateNewName(name); err != nil {
		return Folder{}, err
	}
	return Folder{name: name}, nil
}

// String is the folder's name as the server spells it.
func (f Folder) String() string { return f.name }

// IsZero reports a Folder that was never set.
func (f Folder) IsZero() bool { return f.name == "" }

// FromServerAll wraps a listing the server returned.
func FromServerAll(names []string) []Folder {
	folders := make([]Folder, len(names))
	for i, n := range names {
		folders[i] = Folder{name: n}
	}
	return folders
}

// Strings is the inverse of FromServerAll, for display and for the cache,
// which indexes folders by name.
func Strings(folders []Folder) []string {
	names := make([]string, len(folders))
	for i, f := range folders {
		names[i] = f.name
	}
	return names
}
