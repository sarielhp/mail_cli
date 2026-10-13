// Package cfghandler implements the config subcommands: show, set, unset and
// check.
//
// Which file a key lands in is decided by the key, not by a flag: browser and
// attachment_save_dir are global, the llm_* keys have their own handler, and
// everything else belongs to the account -A selects.
package cfghandler
