package app

import (
	"fmt"
	"os/exec"
)

// RunPreFlightCheck verifies the host has bogofilter in PATH, returning an
// error that says how to install it when it does not.
func RunPreFlightCheck() error {
	_, err := exec.LookPath("bogofilter")
	if err != nil {
		return fmt.Errorf("bogofilter executable not found in PATH. Please install bogofilter on your system.\n" +
			"On Debian/Ubuntu/Kali Linux, execute:\n" +
			"  sudo apt-get install bogofilter")
	}
	return nil
}
