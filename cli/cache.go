package cli

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/contacts"
	"github.com/sarielhp/mail_cli/email"

	"github.com/sarielhp/clihelp"
)

var flagPruneWipe bool

func resolveAccountCacheDir(session *Session) cachedir.Account {
	if session == nil || session.Config == nil {
		return cachedir.Account{}
	}
	dir, err := session.Config.SelectedAccountCache()
	if err != nil {
		return cachedir.Account{}
	}
	return dir
}

func pruneScoresDir(scoresPath string, cutoff time.Time) int {
	entries, err := os.ReadDir(scoresPath)
	if err != nil {
		return 0
	}
	deleted := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		fullPath := filepath.Join(scoresPath, e.Name())
		if err := os.Remove(fullPath); err != nil {
			slog.Warn("failed to remove score file during prune", slog.String("path", fullPath), slog.Any("error", err))
			continue
		}
		deleted++
	}
	return deleted
}

func pruneCacheDir(pruneDir cachedir.Account, cutoff time.Time) (int, error) {
	var totalDeleted int
	var prunedIDs []email.ID
	err := filepath.WalkDir(pruneDir.Path(), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".scores" {
			totalDeleted += pruneScoresDir(path, cutoff)
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".eml") {
			info, err := d.Info()
			if err == nil && info.ModTime().Before(cutoff) {
				prunedIDs = append(prunedIDs, email.ID(strings.TrimSuffix(d.Name(), ".eml")))
			}
		}
		return nil
	})
	if len(prunedIDs) > 0 {
		deleted, errDel := msg.DeleteBatch(pruneDir, prunedIDs)
		if errDel != nil {
			slog.Warn("failed to delete message records during prune", slog.Any("error", errDel))
		}
		totalDeleted += deleted
		if errPurge := folderindex.PurgeIDs(pruneDir, prunedIDs); errPurge != nil {
			slog.Warn("failed to purge label index records during prune", slog.Any("error", errPurge))
		}
	}
	return totalDeleted, err
}

func runCachePrune(session *Session, args []string, wipe bool) error {
	days := 30
	if len(args) > 0 {
		var err error
		days, err = strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("%w: invalid number of days: %s", app.ErrUsage, args[0])
		}
	}

	if wipe || days <= 0 {
		days = 0
	}
	if wipe {
		_ = os.Remove(contacts.DefaultHistoryPath(session.Config))
	}

	cutoff := time.Now().AddDate(0, 0, -days)
	pruneDir := resolveAccountCacheDir(session)

	if _, err := os.Stat(pruneDir.Path()); os.IsNotExist(err) {
		fmt.Fprintf(session.out(), "Cache directory does not exist: %s\n", pruneDir)
		return nil
	}

	totalDeleted, err := pruneCacheDir(pruneDir, cutoff)
	if err != nil {
		return fmt.Errorf("error pruning cache: %w", err)
	}

	fmt.Fprintf(session.out(), "Pruned %d cached file(s) older than %d day(s) from %s.\n", totalDeleted, days, pruneDir)
	return nil
}

func runCacheReset(session *Session) error {
	if err := os.Remove(contacts.DefaultHistoryPath(session.Config)); err != nil && !os.IsNotExist(err) {
		slog.Warn("failed to remove contact history during cache reset", slog.Any("error", err))
	}
	resetDir := resolveAccountCacheDir(session)
	if err := os.RemoveAll(resetDir.Path()); err != nil {
		return fmt.Errorf("failed to remove cache directory %s: %w", resetDir, err)
	}
	if err := os.MkdirAll(resetDir.Path(), 0700); err != nil {
		return fmt.Errorf("failed to recreate cache directory %s: %w", resetDir, err)
	}
	fmt.Fprintf(session.errOut(), "%s Cache directory reset successfully: %s\n", app.PrefixSuccess, resetDir)
	return nil
}

func newCachePruneCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "prune",
		Title:           "utils cache prune [days] [--wipe]",
		Description:     "Prune cached emails older than N days.",
		LongDescription: "Prune cached emails and spam scores older than the specified number of days (default: 30). With --wipe, purges all cached messages regardless of age.",
		UsageLine:       "mail_cli utils cache prune [days] [-w, --wipe]",
		Parameters: []clihelp.Param{
			{Name: "[days]", Description: "Age threshold in days for pruning cached emails (default: 30)."},
		},
		Options: []clihelp.Option{
			clihelp.Bool(&flagPruneWipe, "-w, --wipe", false, "Purge all cached email files regardless of age."),
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli utils cache prune"},
			{Line: "mail_cli utils cache prune 7"},
			{Line: "mail_cli utils cache prune --wipe"},
		},
		Args: clihelp.MaximumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			return runCachePrune(session, ctx.Args, flagPruneWipe)
		},
	}
}

func newCacheResetCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "clear",
		NoAbbrev:        true,
		Aliases:         []string{"reset"},
		Description:     "Recreate cache directory and wipe data.",
		LongDescription: "Recreate the cache directory for the current account, wiping all cached data.",
		UsageLine:       "mail_cli utils cache clear [--yes]",
		Args:            clihelp.NoArgs,
		Options: []clihelp.Option{
			clihelp.Bool(&flagCacheResetYes, "-y, --yes", false, "Proceed without the confirmation prompt."),
		},
		Run: func(_ *clihelp.Context) error {
			if err := session.Confirm("wipe this account's cache", flagCacheResetYes); err != nil {
				return declined(session, err)
			}
			return runCacheReset(session)
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli utils cache clear"},
			{Line: "mail_cli utils cache repair"},
		},
	}
}

// CacheCmd returns the clihelp.Command for the cache command.
func CacheCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:        "cache",
		Group:       "Setup",
		Description: "Manage the local email download cache.",
		UsageLine:   "mail_cli utils cache <subcommand> [args...]",
		Subcommands: []clihelp.Command{
			newCachePruneCmd(session),
			newCacheResetCmd(session),
			newCacheRepairCmd(session),
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli utils cache prune"},
			{Line: "mail_cli utils cache prune 7"},
			{Line: "mail_cli utils cache prune --wipe"},
			{Line: "mail_cli utils cache clear"},
		},
	}
}
