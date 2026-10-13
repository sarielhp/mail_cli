// Package cfg_g is the global configuration: loading and saving the file,
// resolving which account a command acts on, and parsing the %label references
// the command line takes.
//
// Account lookup happens here and only here. It used to be hand-rolled at eight
// sites under three different matching rules, which disagreed -- an identifier
// matching nothing resolved to the first account in the file, so a command ran
// against the wrong mailbox and exited 0. Lookup now errors on both the unknown
// and the ambiguous case, and WithAccount returns a copy rather than mutating
// shared state.
package cfg_g
