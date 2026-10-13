// Package label is about folder names: matching one the user typed against the
// ones the server has, ordering a listing, and nesting it into a tree.
//
// MatchOne is the matching half of every label argument in the program. It
// reports a tie rather than resolving it, because who may guess differs by
// caller: the command line fails on an ambiguous name and the TUI falls back to
// something displayable. The algorithm is shared; the policy is not.
//
// The package imports nothing but the standard library, which is what lets both
// the CLI and the backends depend on it.
package label
