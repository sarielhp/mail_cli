// Package cfg_acc holds the per-account configuration types: the account
// itself, its routing rules, and its whitelist and blacklist.
//
// It is separate from cfg_g so that a package needing to describe one account
// does not pull in global configuration, and so the two can be reasoned about
// apart. Item and MessageFolderRef are aliases for the label package's
// types, which keeps one definition of a folder listing entry.
package cfg_acc
