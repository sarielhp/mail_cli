// Package virtual stores named result sets: a list of message IDs plus the
// folder each came from, saved under the cache directory.
//
// It began as the backing store for the `last` command and lived in that
// package, but nothing about it is specific to recent mail. Any command that
// produces a set of messages the user will want to look through afterwards --
// `last`, `search` -- wants exactly this, and mirroring the set into the folder
// index means `show` and the TUI can browse it like any other folder.
package virtual

import (
	"encoding/json"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/sarielhp/mail_cli/cache/dirlock"
	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cfg_g"
)

// Mailbox represents a named virtual folder containing message IDs and their source folders.
type Mailbox struct {
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	MessageIDs  []email.ID          `json:"message_ids"`
	FolderMap   map[email.ID]string `json:"folder_map"` // message ID -> original folder name
}

// virtualDir returns the path to the virtual mailboxes directory in cache.
func virtualDir(downloadDir string) string {
	return filepath.Join(downloadDir, "virtual")
}

// virtualPath returns the path to a specific virtual mailbox JSON file.
func virtualPath(downloadDir, name string) string {
	return filepath.Join(virtualDir(downloadDir), cfg_g.SanitizeLabelForCache(name)+".json")
}

// Save writes a Mailbox to the cache. It also updates the cache/label index
// so that virtual mailboxes can be inspected using standard label cache APIs.
//
// Two writers of the same virtual mailbox -- two concurrent `last`/`search`
// runs, or the TUI racing a scheduled scan against a shared account cache --
// used to share one unlocked "<name>.json.tmp": their writes could interleave
// and a failing rename could replace the previous good file with a mangled
// one (M7). dirlock.AtomicWrite gives the JSON write a unique temp name and
// fsyncs it before rename; the directory guard also covers the label-index
// mirror, so a reader never observes the two out of step.
func Save(account cachedir.Account, vm *Mailbox) error {
	downloadDir := account.Path()
	release, err := dirlock.Guard(downloadDir)
	if err != nil {
		slog.Debug("cache lock unavailable, continuing with in-process lock only",
			slog.String("dir", downloadDir), slog.Any("error", err))
	}
	defer release()

	data, err := json.Marshal(vm)
	if err != nil {
		return err
	}
	p := virtualPath(downloadDir, vm.Name)
	if err := dirlock.AtomicWrite(p, data, 0600); err != nil {
		return err
	}

	// Also mirror into label index under virtual/<name> or virtual_<name>
	virtualLabelName := "virtual/" + vm.Name
	_ = folderindex.ReplaceAll(account, virtualLabelName, vm.MessageIDs)
	return nil
}

// Load reads a Mailbox from the cache.
func Load(account cachedir.Account, name string) (*Mailbox, error) {
	downloadDir := account.Path()
	p := virtualPath(downloadDir, name)
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var vm Mailbox
	if err := json.Unmarshal(data, &vm); err != nil {
		return nil, err
	}
	return &vm, nil
}

// FindMessageFolder looks up which source folder a message originated from across all virtual mailboxes or cached indexes.
//
// It used to reconstruct each mailbox's name from its filename
// (strings.TrimSuffix(e.Name(), ".json")) and hand that to Load, which
// re-sanitizes whatever name it is given. That round-trip was a no-op
// before H4, when SanitizeLabelForCache's output was always already a fixed
// point of itself (only [A-Za-z0-9_-], all already safe to pass through
// again). It stopped being one once the fix started escaping unsafe runes
// as "~<hex>~": '~' is not itself safe, so re-sanitizing an already-escaped
// filename escapes it a second time and looks under a path nothing was ever
// written to. Reading each file directly, instead of re-deriving and
// re-sanitizing its name, has no such round-trip to break.
func FindMessageFolder(account cachedir.Account, msgID email.ID) string {
	downloadDir := account.Path()
	// First check virtual mailboxes
	vDir := virtualDir(downloadDir)
	if entries, err := os.ReadDir(vDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(vDir, e.Name()))
			if err != nil {
				continue
			}
			var vm Mailbox
			if json.Unmarshal(data, &vm) == nil {
				if f, ok := vm.FolderMap[msgID]; ok && f != "" {
					return f
				}
			}
		}
	}
	return ""
}
