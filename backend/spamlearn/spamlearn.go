// Package spamlearn records which messages a backend has trained the spam
// filter on.
//
// Each backend keeps its own "already learned" map and writes it to a training
// ledger; the write is the same everywhere, so it lives here rather than being
// copied per backend.
package spamlearn

import (
	"log/slog"

	"github.com/sarielhp/mail_cli/train"
)

// RecordTrained records every ID marked done in trained into the training
// ledger at path, converging the file on the ledger's single shape. A ledger
// that cannot be written is logged and does not fail the caller: the training
// itself has already happened.
func RecordTrained(path string, trained map[string]bool) {
	ledger := train.OpenOrEmpty(path)
	var ids []string
	for id, done := range trained {
		if done {
			ids = append(ids, id)
		}
	}
	if err := ledger.AddAll(ids); err != nil {
		slog.Warn("could not record trained messages",
			slog.String("path", path), slog.Any("error", err))
	}
}
