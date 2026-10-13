package gmail

import (
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
)

// Config aliases the runtime configuration, so Gmail code reads it under its own
// name.
type Config = cfg_g.Config

// AccountConfig aliases the per-account configuration the Gmail backend is built
// from.
type AccountConfig = cfg_acc.AccountConfig
type item = cfg_acc.Item
type rule = cfg_acc.Rule
