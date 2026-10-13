package cli

import (
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/mailclient"
)

// stubSearchClient supplies the one method saveSearchMailbox reaches. Embedding
// the interface means the compiler still checks the signature while leaving the
// rest unimplemented.
type stubSearchClient struct {
	mailclient.MailClient
	downloadDir string
}

func (s *stubSearchClient) Config() *cfg_g.Config {
	return &cfg_g.Config{Paths: cfg_g.Paths{CacheRoot: s.downloadDir}, AccountCache: cachedir.At(s.downloadDir)}
}
