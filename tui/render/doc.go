// Package render turns the TUI's model state into styled terminal output:
// themes, coloured segments, and the individual lines of the folder tree and
// message list.
//
// It is kept apart from the tui package so that layout decisions can be tested
// as string transformations, without a model or an event loop.
package render
