// Package mailclient defines what a mail backend must be able to do, and the
// decorators that wrap one.
//
// The interfaces are split by role -- LabelReader, EmailFetcher, EmailWriter,
// MessageAccessor -- so a function can ask for the capability it uses instead
// of the whole client, and so a read-only caller cannot reach a mutating
// method by accident. Reader and Mutator embed those roles: adding a method to
// Mutator fails to compile until every decorator handles it, which is the
// property that keeps ReadOnlyMailClient honest.
//
// Two decorators live here. CheckingMailClient owns cache reconciliation, so a
// server move is reflected in the local folder index exactly once rather than
// at eight call sites. ReadOnlyMailClient refuses or simulates every mutating
// method and is the only sanctioned way to touch a live account in a test.
package mailclient
