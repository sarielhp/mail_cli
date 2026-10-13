// Package watcher polls mail accounts in the background and announces what is
// new: a desktop notification, a user script, and a flush of the offline
// outbox.
//
// It is a foreground process by design. systemd --user, tmux or a desktop
// session manager supervises it; the watcher never double-forks, never writes a
// PID file, and exits when its context is cancelled. A supervisor that restarts
// it and a log it writes to are the two things a daemon needs, and both already
// exist outside the program.
//
// The watermark it keeps -- which message IDs it has already seen, per account,
// under <download_dir>/watcher/state.json -- is what makes the difference
// between "there is new mail" and "here is what arrived since you last looked".
// An account's first poll records everything it finds as seen and announces
// nothing, so a fresh install does not fire a notification per message already
// sitting in the inbox.
//
// It does not hold the cache lock across a poll. dirlock serializes a
// read-modify-write of one cache index, and the download path takes it from
// worker goroutines; holding it here, in the polling goroutine, would block
// those workers on a mutex this goroutine owns and deadlock the fetch. What the
// watcher does need it for -- a consistent read and write of its own watermark
// -- it takes, briefly, around the state file alone.
package watcher
