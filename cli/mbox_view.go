package cli

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/cfg_acc"
	"github.com/sarielhp/mail_cli/cfg_g"

	"github.com/sarielhp/clihelp"
)

func mboxViewCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "view",
		Description: "Browse a local mbox or .eml file in the TUI.",
		UsageLine:   "mail_cli mbox view <file>",
		Parameters: []clihelp.Param{
			{Name: "<file>", Description: "A local mbox file, or a single message ending in .eml"},
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli mbox view archive.mbox"},
			{Line: "mail_cli mbox view ~/mail/backup.mbox"},
			{Line: "mail_cli mbox view message.eml", Description: "Open one saved message"},
		},
		Args: clihelp.ExactArgs(1),
		Run: func(ctx *clihelp.Context) error {
			return runMboxView(session, ctx.Args[0])
		},
	}
}

func runMboxView(session *Session, mboxPath string) error {
	absPath, err := filepath.Abs(mboxPath)
	if err != nil {
		return fmt.Errorf("invalid mbox path: %w", err)
	}
	fi, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("cannot access mbox %q: %w", absPath, err)
	}
	if fi.IsDir() {
		return fmt.Errorf("%q is a directory, not a mail file", absPath)
	}

	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(absPath)))[:12]
	base := cfg_g.SanitizeLabelForCache(filepath.Base(absPath))
	cacheDir := filepath.Join(session.Config.CacheRoot, "mbox_views", fmt.Sprintf("%s_%s", base, hash))

	// A pseudo-account: the mbox is not in accounts[], so there is no entry to
	// take a canonical name from.
	accName := cfg_g.AccountName("mbox")
	displayName := filepath.Base(absPath)
	localCfg := *session.Config
	localCfg.SelectedAccount = accName
	localCfg.ReadOnly = true
	// The view's cache is outside the per-account layout, so it is named
	// explicitly rather than derived from an account name.
	localCfg.AccountCache = cachedir.At(cacheDir)
	localCfg.Accounts = []cfg_acc.AccountConfig{
		{
			Name:        string(accName),
			DisplayName: displayName,
			Type:        "mbox",
			ReadOnly:    true,
		},
	}

	client, err := session.accounts().OpenMbox(&localCfg, absPath, localCfg.AccountCache)
	if err != nil {
		return err
	}

	return session.tui().RunWithClient(&localCfg, client, "INBOX")
}
