// Package app holds the small utilities every layer may use: colour styles and
// the [+]/[!] prefixes, the loggers, temporary directories, the version string,
// and ErrUsage.
//
// It is deliberately thin. It once also held a Session type carrying thirty
// function pointers, which made it a hub every package depended on; that is
// gone, and what remains is leaf-shaped on purpose. Nothing here may import a
// layer above it.
package app
