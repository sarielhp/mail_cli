// Package interactive holds the tests that drive mail_cli in a real terminal.
//
// They exist for what a unit test cannot see: the escapes a terminal actually
// receives, and input -- keys, mouse clicks -- arriving the way a person sends
// it. Each test runs the binary inside kitty on a private Xvfb display, so no
// window opens on the desktop, with HOME pointed at a temporary directory, so
// the user's config and cache are never read or written, and with a browser
// that only records the URLs it is asked to open.
//
// Nothing here runs by default; it is too slow for the quality gate. Run it
// when a change touches what only a terminal shows -- mouse handling, links,
// escape output:
//
//	MAIL_CLI_INTERACTIVE=1 go test ./interactive/ -v -count=1   (or: make interactive)
//
// With the variable set, a missing tool (xvfb-run, kitty, xdotool, ImageMagick,
// w3m) fails the run and names the packages to install.
package interactive
