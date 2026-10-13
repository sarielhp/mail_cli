package tui

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/report"

	tea "github.com/charmbracelet/bubbletea"
)

// InitTuiWithClient launches the TUI using an explicitly provided MailClient.
func InitTuiWithClient(cfg *cfg_g.Config, client mailclient.MailClient, labelPrefix, initialNotice string, bk *Backend, rep *ProgramReporter) error {
	cfg.Quiet = true
	app.InitTuiLogger(cfg.ConfigDir)
	defer app.CloseTuiLogger()

	defer recoverTuiPanic()

	model := newTuiModel(client, labelPrefix, bk)
	model.cfg = cfg
	model.rep = report.Or(rep)
	if initialNotice != "" {
		model.err = errors.New(initialNotice)
		model.showError = true
	}
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	// The reporter was built before the program existed, so the backends it was
	// wired into could be constructed. Now it has somewhere to send.
	if rep != nil {
		rep.Send = func(msg any) { p.Send(msg) }
	}
	final, err := p.Run()
	if err != nil {
		return err
	}
	printExitNotice(final)
	return nil
}

// printExitNotice says where a draft went when the program exited by saving
// one. The index view would have shown it as a status line; with the screen
// gone this is the only place left to say it.
func printExitNotice(final tea.Model) {
	if fm, ok := final.(*tuiModel); ok && fm.exitNotice != "" {
		fmt.Fprintln(os.Stderr, fm.exitNotice)
	}
}

// recoverTuiPanic logs a TUI panic and points at the log, so a crash inside the
// alternate screen leaves a readable trace rather than a garbled terminal.
func recoverTuiPanic() {
	if r := recover(); r != nil {
		slog.Error("TUI panicked", slog.Any("panic", r), slog.String("stack", string(debug.Stack())))
		fmt.Fprintf(os.Stderr, "\n%s TUI Panicked: %v\nSee logs/tui.log for full details.\n", app.PrefixError, r)
	}
}

// InitTuiMode initializes and runs the interactive TUI program.
func InitTuiMode(config interface{}, labelPrefix, initialNotice string, bk *Backend, rep *ProgramReporter) error {
	cfg := config.(*cfg_g.Config)
	if err := app.RunPreFlightCheck(); err != nil {
		return err
	}
	client, err := bk.clients().ForSelected(cfg)
	if err != nil {
		return err
	}
	return InitTuiWithClient(cfg, client, labelPrefix, initialNotice, bk, rep)
}

// InitTuiCompose launches the interactive compose screen directly.
func InitTuiCompose(cfg *cfg_g.Config, client mailclient.MailClient, opts ComposeOptions, bk *Backend, rep *ProgramReporter) error {
	cfg.Quiet = true
	if err := client.Validate(); err != nil {
		return err
	}
	app.InitTuiLogger(cfg.ConfigDir)
	defer app.CloseTuiLogger()

	defer recoverTuiPanic()

	model := newTuiModel(client, "", bk)
	model.cfg = cfg
	model.rep = report.Or(rep)
	model.standaloneCompose = true
	model.composeOpts = opts
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	if rep != nil {
		rep.Send = func(msg any) { p.Send(msg) }
	}
	final, err := p.Run()
	if err != nil {
		return err
	}
	printExitNotice(final)
	return nil
}
