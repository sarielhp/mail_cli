package watcher

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/sarielhp/mail_cli/email"
)

// Notification is one new message, as the watcher announces it.
type Notification struct {
	Account string
	From    string
	Subject string
	Date    string
	ID      email.ID
}

// Notifier delivers one new-mail alert. The watcher holds one so a test can
// capture what would have been shown instead of spawning notify-send.
type Notifier interface {
	Notify(context.Context, Notification) error
}

// notifySendNotifier shows a desktop notification through notify-send, which is
// present on any freedesktop session and absent elsewhere. An absent binary
// returns an error, and the watcher falls back to logging the alert.
type notifySendNotifier struct{}

func (notifySendNotifier) Notify(ctx context.Context, n Notification) error {
	title := fmt.Sprintf("mail_cli [%s]", n.Account)
	body := fmt.Sprintf("%s\n%s", n.From, n.Subject)
	// The leading -- keeps a subject that begins with a dash from being read as
	// an option.
	ctx, cancel := context.WithTimeout(ctx, deliverTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "notify-send", "--app-name=mail_cli", "--", title, body)
	cmd.WaitDelay = time.Second
	return cmd.Run()
}

// runHook runs the --on-mail program with one new message in its environment.
// The message is passed as variables rather than arguments so the hook can
// ignore what it does not care about and a subject with spaces needs no
// quoting. It is killed after deliverTimeout, so a hook that blocks cannot
// stall the watcher.
func runHook(ctx context.Context, path string, n Notification) error {
	ctx, cancel := context.WithTimeout(ctx, deliverTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path)
	cmd.WaitDelay = time.Second
	cmd.Env = append(os.Environ(), hookEnv(n)...)
	return cmd.Run()
}

func hookEnv(n Notification) []string {
	return []string{
		"MAIL_CLI_ACCOUNT=" + n.Account,
		"MAIL_CLI_FROM=" + n.From,
		"MAIL_CLI_SUBJECT=" + n.Subject,
		"MAIL_CLI_DATE=" + n.Date,
		"MAIL_CLI_ID=" + string(n.ID),
	}
}
