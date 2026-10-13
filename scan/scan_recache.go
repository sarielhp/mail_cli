package scan

import (
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/spam"
	"log/slog"
)

// TrainHamLocally trains Bogofilter that the given message is legitimate (HAM).
func TrainHamLocally(id email.ID, emailBytes []byte, _ *cfg_g.Config) {
	if err := spam.TrainHam(emailBytes); err != nil {
		slog.Warn("Failed to run bogofilter HAM training", slog.String("message_id", string(id)), slog.String("error", err.Error()))
	} else {
		slog.Info("Trained bogofilter that whitelisted sender address is HAM", slog.String("message_id", string(id)))
	}
}
