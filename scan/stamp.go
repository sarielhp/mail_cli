package scan

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sarielhp/mail_cli/cfg_g"
)

// checksVersion changes whenever the checks themselves change -- a new
// heuristic, a different cutoff -- so evidence computed by an older binary is
// re-scanned rather than trusted.
const checksVersion = 1

// Stamp identifies everything a message's evidence depends on besides the
// message: bogofilter's database, the settings that decide a check, and the
// checks' own version. Evidence carrying another stamp is stale and the
// message is scanned again.
//
// Training changes the database and so the stamp, which is what makes a
// `spam learn` re-judge the rest of the mail instead of leaving every earlier
// verdict standing.
func Stamp(config *cfg_g.Config) uint32 {
	h := fnv.New32a()
	fmt.Fprintf(h, "v=%d|db=%d|cut=%g|pol=%t|lang=%s|bl=%s|wl=%s",
		checksVersion, wordlistModTime(), config.ScoreThreshold, config.BlockPolitical,
		normalizedList(config.AllowedLanguages), normalizedList(config.Blacklist), normalizedList(config.Whitelist))
	return h.Sum32()
}

// wordlistModTime is when bogofilter's database last changed, or 0 when it
// cannot be found. bogofilter reads $BOGOFILTER_DIR, else ~/.bogofilter.
func wordlistModTime() int64 {
	dir := os.Getenv("BOGOFILTER_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return 0
		}
		dir = filepath.Join(home, ".bogofilter")
	}
	fi, err := os.Stat(filepath.Join(dir, "wordlist.db"))
	if err != nil {
		return 0
	}
	return fi.ModTime().UnixNano()
}

func normalizedList(items []string) string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, strings.ToLower(strings.TrimSpace(it)))
	}
	slices.Sort(out)
	return strings.Join(out, ",")
}
