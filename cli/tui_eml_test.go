package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsEmlFile(t *testing.T) {
	dir := t.TempDir()
	eml := filepath.Join(dir, "saved.eml")
	if err := os.WriteFile(eml, []byte("Subject: x\n\nbody\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "dir.eml"), 0o700); err != nil {
		t.Fatal(err)
	}
	cases := map[string]bool{
		eml:                               true,
		filepath.Join(dir, "missing.eml"): false, // a label prefix, not a file
		filepath.Join(dir, "dir.eml"):     false,
		"%receipts":                       false,
	}
	for arg, want := range cases {
		if got := isEmlFile(arg); got != want {
			t.Errorf("isEmlFile(%q) = %v, want %v", arg, got, want)
		}
	}
}
