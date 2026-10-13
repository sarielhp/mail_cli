package msg

import (
	"encoding/json"
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"io"
	"log/slog"
	"time"

	"github.com/sarielhp/mail_cli/email"
)

// CachedEmail parses a cached message into a email.Email, filling in what the
// cache index knows on top of the headers: the classification flags, and a date
// for messages whose own Date header is missing or unparsable.
//
// This was open-coded three times -- in ui, in last, and inline in the TUI's
// load command -- with the copies having drifted apart. It lived in uicommon,
// which made the mbox backend import a UI package to read its own cache; there
// is nothing presentational in it. It returns an error
// when the message is not cached or does not parse, because the callers
// genuinely disagree about what that means: a folder listing shows an error
// row in place of the message, while the TUI leaves it out. That decision
// stays with them, as does whether to reverse Hebrew runs in the subject, which
// the two listing paths do and the TUI does not.
func CachedEmail(account cachedir.Account, id email.ID) (*email.Email, error) {
	downloadDir := account.Path()
	if data, info, err := ReadParsedSummaryAndInfo(account, id); err == nil {
		var parsed email.Email
		if err := json.Unmarshal(data, &parsed); err == nil {
			return applyInfo(info, &parsed), nil
		}
	}

	var parsed *email.Email
	readErr := WithReader(account, id, func(r io.Reader) error {
		parsed = email.ParseReader(r, id, "")
		return nil
	})
	if readErr != nil {
		return nil, fmt.Errorf("read cached message %s: %w", id, readErr)
	}
	if parsed == nil {
		return nil, fmt.Errorf("parse cached message %s: malformed MIME", id)
	}

	if err := storeCachedSummary(downloadDir, id, parsed); err != nil {
		slog.Warn("failed to persist parsed summary cache", slog.String("id", string(id)), slog.Any("error", err))
	}
	return applyCachedInfo(downloadDir, id, parsed), nil
}

func storeCachedSummary(downloadDir string, id email.ID, parsed *email.Email) error {
	data, err := json.Marshal(parsed)
	if err != nil {
		return err
	}
	return StoreParsedSummary(cachedir.At(downloadDir), id, data)
}

func applyInfo(info *Info, parsed *email.Email) *email.Email {
	if parsed.EmailDate.IsZero() {
		switch {
		case info != nil && !info.Date.IsZero():
			parsed.EmailDate = info.Date
		default:
			parsed.EmailDate = time.Now()
		}
		parsed.FormattedDate = email.FormatEmailDate(parsed.EmailDate)
	}
	if info != nil {
		parsed.IsSpam = info.IsSpam()
		parsed.IsPolitical = info.IsPolitical()
		parsed.IsBlacklisted = info.IsBlacklisted()
	}
	return parsed
}

func applyCachedInfo(downloadDir string, id email.ID, parsed *email.Email) *email.Email {
	info, infoErr := GetInfo(cachedir.At(downloadDir), id)
	if infoErr != nil {
		info = nil
	}
	return applyInfo(info, parsed)
}
