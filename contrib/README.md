# contrib

Files that belong to a release but not to the Go build: package recipes and
demo assets.

| Path | Purpose |
|---|---|
| `aur/PKGBUILD` | Template for the AUR `mail-cli` package (builds from the tagged source tarball, since releases are source-only). |
| `demo.tape` | VHS tape that renders the README hero GIF. |
| `systemd/mail_cli-watcher.service` | User unit that supervises `mail_cli watch` under `systemd --user`. |

The systemd unit is a template: copy it to `~/.config/systemd/user/`, adjust
`ExecStart` if the binary is not in `~/bin`, then `systemctl --user enable
--now mail_cli-watcher`.
