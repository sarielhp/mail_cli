// Package filebrowser is the interactive directory picker used when saving an
// attachment, built on bubbletea like the rest of the interface.
//
// It browses the user's own filesystem on their behalf, so there is no trust
// boundary here to defend -- only the ordinary care of cleaning a path before
// showing it.
package filebrowser
