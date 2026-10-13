// Package pgp is the mail program's OpenPGP layer: it shells out to the system
// gpg and knows how a PGP/MIME message is shaped.
//
// It runs the gpg binary rather than linking an OpenPGP implementation. That
// reuses the keyring the user already has, their smartcard or YubiKey, and the
// passphrase caching gpg-agent already does -- and a Go OpenPGP library that
// reimplements all three would be a second, worse keyring. The cost is a
// subprocess per operation, which is nothing next to the network round trip
// that fetched the message.
//
// Everything the caller needs to decide is returned as data: a Status parsed
// from gpg's status channel, and the plaintext. Nothing here prints, so the CLI
// and the TUI can present the same result differently.
package pgp
