// Package browser launches a web browser on a URL or local file.
//
// It lived in backend/gmail, where it had nothing to do with Gmail and forced
// show to import a mail backend in order to display an HTML message body. Like
// terminal, it is a leaf with no mail_cli imports of its own, which is what lets
// any layer reach it.
//
// Neither function prints. Which browser was tried, and whether a fallback is
// worth mentioning, is a presentation decision that belongs to the caller.
package browser

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// OpenWith launches target using an explicit browser command.
func OpenWith(browserCmd, target string) error {
	if browserCmd == "" {
		return fmt.Errorf("no browser command given")
	}
	return exec.Command(browserCmd, target).Start()
}

// Open launches target using the platform's default browser.
//
// On Linux there is no single default, so the usual launchers are tried in turn
// and the error names every one that failed.
func Open(target string) error {
	switch runtime.GOOS {
	case "linux":
		return openLinux(target)
	case "darwin":
		return exec.Command("open", target).Start()
	case "windows":
		return windowsCmd(target).Start()
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func openLinux(target string) error {
	launchers := []string{"xdg-open", "x-www-browser", "google-chrome", "firefox"}

	var errs []string
	for _, launcher := range launchers {
		err := exec.Command(launcher, target).Start()
		if err == nil {
			return nil
		}
		errs = append(errs, fmt.Sprintf("%s: %v", launcher, err))
	}
	return fmt.Errorf("failed to launch any browser on linux (%s)", strings.Join(errs, ", "))
}

func windowsCmd(target string) *exec.Cmd {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
}
