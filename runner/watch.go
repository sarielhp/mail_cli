package runner

import (
	"context"

	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/mailops"
	"github.com/sarielhp/mail_cli/report"
	"github.com/sarielhp/mail_cli/watcher"
)

// WatchService runs the background mail watcher for the CLI. It is the seam
// between the watcher, which names no backend, and the factory, which is the
// only package that may.
type WatchService struct {
	Rep report.Reporter
}

func (s WatchService) rep() report.Reporter { return report.Or(s.Rep) }

// Watch polls the configured accounts until ctx is cancelled.
func (s WatchService) Watch(ctx context.Context, config *cfg_g.Config, opts watcher.Options) error {
	rep := s.rep()
	return watcher.Run(ctx, watcher.Deps{
		Config:   config,
		Reporter: rep,
		ClientFor: func(acc cfg_acc.AccountConfig) (mailclient.Reader, error) {
			return mailops.NewMailClient(acc, config, rep)
		},
		Flush: func() error {
			_, err := OutboxService{Rep: rep}.Flush(config)
			return err
		},
	}, opts)
}
