// Package terminal holds the low-level terminal queries that several packages
// need and none of them owns.
//
// Width lived twice, identically, in app and email — including the same
// 70-column fallback. Neither could import the other: app reaches email through
// mailclient, so sharing it either way is an import cycle. A leaf package with
// no dependencies of its own is what both can reach, and it is the only reason
// this package exists.
package terminal

import (
	"os"

	"golang.org/x/term"
)

// FallbackWidth is what Width reports when there is no terminal to ask — output
// is redirected to a file or a pipe. It matches what clihelp lays help out at in
// the same situation, so wrapped mail and wrapped help agree.
const FallbackWidth = 70

// Width returns the terminal's column count, or FallbackWidth when stdout is not
// a terminal.
func Width() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 {
		return FallbackWidth
	}
	return width
}
