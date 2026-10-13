// Package gmail implements the mail client interfaces against the Gmail REST
// API, including its OAuth flow.
//
// Gmail labels are not folders: a message carries a set of them, so a move is
// an add and a remove rather than a relocation. That difference is contained
// here, and callers above see the same MoveEmail every other backend offers.
package gmail
