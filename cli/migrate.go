package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/sarielhp/mail_cli/app"

	"github.com/sarielhp/clihelp"
)

// MigrateCmd returns the clihelp.Command for the migrate command.
func MigrateCmd(session *Session) clihelp.Command {
	return clihelp.Command{
		Name:            "migrate",
		Group:           "Setup",
		Description:     "Copy config and credentials via SSH.",
		LongDescription: "Copy configuration and credentials to a remote machine via SSH/SCP.",
		UsageLine:       "mail_cli account migrate [user@]host",
		Parameters: []clihelp.Param{
			{Name: "[user@]host", Description: "Destination host, as [user@]host"},
		},
		Examples: []clihelp.Example{
			{Line: "mail_cli account migrate myserver.local"},
			{Line: "mail_cli account migrate user@myserver.local"},
		},
		Args: clihelp.ExactArgs(1),
		Run: func(ctx *clihelp.Context) error {
			return migrateRun(session, ctx.Args[0])
		},
	}
}

func resolveSSHTarget(target string) (string, error) {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return "", fmt.Errorf("migrate: target cannot be empty")
	}
	if strings.HasPrefix(trimmed, "-") {
		return "", fmt.Errorf("migrate: invalid target %q: leading hyphens not permitted", target)
	}
	if strings.Count(trimmed, "@") > 1 {
		return "", fmt.Errorf("migrate: invalid target %q: multiple '@' not permitted", target)
	}
	if strings.ContainsAny(trimmed, " \t\n\r`$&;|<>(){}[]\\\"'") {
		return "", fmt.Errorf("migrate: invalid target %q: contains invalid characters", target)
	}

	userPart, host := parseTarget(trimmed)
	if strings.HasPrefix(userPart, "-") {
		return "", fmt.Errorf("migrate: invalid user %q: leading hyphens not permitted", userPart)
	}
	if strings.HasPrefix(host, "-") || host == "" {
		return "", fmt.Errorf("migrate: invalid host %q", host)
	}

	if userPart != "" {
		return fmt.Sprintf("%s@%s", userPart, host), nil
	}
	current, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("migrate: cannot determine current user: %w", err)
	}
	if current.Username == "root" {
		return "", fmt.Errorf("migrate: cannot use root as default user; specify user@host instead")
	}
	return fmt.Sprintf("%s@%s", current.Username, host), nil
}

func collectMigrationFiles(configDir string) ([]string, error) {
	var files []string
	always := []string{"config.json"}
	for _, name := range always {
		path := filepath.Join(configDir, name)
		if _, err := os.Stat(path); err == nil {
			files = append(files, path)
		}
	}

	patterns := []string{"credentials.json", "token.json", "token_*.json"}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(configDir, pattern))
		if err != nil {
			return nil, fmt.Errorf("migrate: failed to glob %s: %w", pattern, err)
		}
		files = append(files, matches...)
	}

	tokensDir := filepath.Join(configDir, "tokens")
	if entries, err := os.ReadDir(tokensDir); err == nil {
		for _, entry := range entries {
			files = append(files, filepath.Join(tokensDir, entry.Name()))
		}
	}
	return files, nil
}

func copyFilesOverSSH(w io.Writer, sshTarget, configDir string, files []string) error {
	sshCmd := exec.Command("ssh", "--", sshTarget, "mkdir", "-p", filepath.Join(".config", app.AppName))
	sshCmd.Stderr = os.Stderr
	if err := sshCmd.Run(); err != nil {
		return fmt.Errorf("migrate: failed to create remote config directory: %w", err)
	}

	for _, src := range files {
		rel, _ := filepath.Rel(configDir, src)
		dst := fmt.Sprintf("%s:%s/%s", sshTarget, filepath.Join(".config", app.AppName), rel)

		scpCmd := exec.Command("scp", "--", src, dst)
		scpCmd.Stderr = os.Stderr
		if err := scpCmd.Run(); err != nil {
			return fmt.Errorf("migrate: failed to copy %s: %w", rel, err)
		}
		fmt.Fprintf(w, "  ✓ %s\n", rel)
	}
	return nil
}

func migrateRun(session *Session, target string) error {
	if os.Geteuid() == 0 {
		return fmt.Errorf("migrate: refusing to run as root")
	}

	sshTarget, err := resolveSSHTarget(target)
	if err != nil {
		return err
	}

	if _, sshErr := exec.LookPath("ssh"); sshErr != nil {
		return fmt.Errorf("migrate: ssh not found in PATH")
	}
	if _, scpErr := exec.LookPath("scp"); scpErr != nil {
		return fmt.Errorf("migrate: scp not found in PATH")
	}

	configDir := session.Config.ConfigDir
	files, err := collectMigrationFiles(configDir)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("migrate: no files found to copy in %s", configDir)
	}

	if err := copyFilesOverSSH(session.out(), sshTarget, configDir, files); err != nil {
		return err
	}

	fmt.Fprintln(session.out())
	fmt.Fprintln(session.out(), "Migration complete. Run the following on the remote machine to verify:")
	fmt.Fprintf(session.out(), "  %s test\n", app.AppName)

	return nil
}

func parseTarget(raw string) (string, string) {
	if strings.Contains(raw, "@") {
		parts := strings.SplitN(raw, "@", 2)
		return parts[0], parts[1]
	}
	return "", raw
}
