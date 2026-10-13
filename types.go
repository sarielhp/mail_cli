package main

import (
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"
)

// Short names for the types this package handles most. The spam-scoring and
// rule aliases that used to live here had no users once uicommon was dissolved.
type (
	Config        = cfg_g.Config
	FileConfig    = cfg_g.FileConfig
	AccountConfig = cfg_acc.AccountConfig
)
