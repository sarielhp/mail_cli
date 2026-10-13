package tui

import (
	"strings"
	"testing"
)

func TestBuildWindowsOpenFileCmd(t *testing.T) {
	testPath := `C:\Users\victim\AppData\Local\Temp\invoice&calc.exe.pdf`
	cmd := buildWindowsOpenFileCmd(testPath)

	if cmd.Path != "rundll32" && !strings.HasSuffix(cmd.Path, "rundll32") {
		t.Errorf("expected command rundll32, got %q", cmd.Path)
	}

	expectedArgs := []string{"rundll32", "url.dll,FileProtocolHandler", testPath}
	if len(cmd.Args) != len(expectedArgs) {
		t.Fatalf("expected %d args, got %d: %v", len(expectedArgs), len(cmd.Args), cmd.Args)
	}
	for i, arg := range expectedArgs {
		if cmd.Args[i] != arg {
			t.Errorf("arg[%d] = %q, want %q", i, cmd.Args[i], arg)
		}
	}
}
