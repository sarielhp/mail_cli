package gmail_test

import (
	"github.com/sarielhp/mail_cli/backend/gmail"
	"github.com/sarielhp/mail_cli/cfg_g"
	"os"
	"path/filepath"
	"testing"
)

func TestDecodeGmailRaw(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:    "Standard Base64URL no padding",
			input:   "SGVsbG8gV29ybGQ",
			want:    "Hello World",
			wantErr: false,
		},
		{
			name:    "Standard Base64URL with padding",
			input:   "SGVsbG8gV29ybGQ=",
			want:    "Hello World",
			wantErr: false,
		},
		{
			name:    "Standard Base64 with padding",
			input:   "SGVsbG8gV29ybGQ=",
			want:    "Hello World",
			wantErr: false,
		},
		{
			name:    "Base64 with newlines and spaces",
			input:   "SGVsbG8g\n V29ybGQ\r\n=",
			want:    "Hello World",
			wantErr: false,
		},
		{
			name:    "Base64 with URL characters",
			input:   "PDw_Pj4",
			want:    "<<?>>",
			wantErr: false,
		},
		{
			name:    "Base64 with standard slash character",
			input:   "PDw/Pj4=",
			want:    "<<?>>",
			wantErr: false,
		},
		{
			name:    "Invalid base64",
			input:   "!!!invalid!!!",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBytes, err := gmail.DecodeGmailRaw(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("gmail.DecodeGmailRaw() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if !tt.wantErr && string(gotBytes) != tt.want {
				t.Errorf("decodeGmailRaw() got = %q, want = %q", string(gotBytes), tt.want)
			}
		})
	}
}

// TestGetTokenPath_MigratesOldSanitizedTokenForward is H4's regression: an
// account name holding a character outside [A-Za-z0-9_-] sanitizes
// differently now than it did before H4, so a token saved under the old
// name would otherwise look missing and force an unnecessary
// re-authentication. GetTokenPath must find and move it forward itself.
func TestGetTokenPath_MigratesOldSanitizedTokenForward(t *testing.T) {
	tmpDir := t.TempDir()
	accName := "corp.com"
	cfg := &gmail.Config{SelectedAccount: cfg_g.AccountName(accName), Paths: cfg_g.Paths{ConfigDir: tmpDir}}

	oldPath := filepath.Join(tmpDir, "token_"+cfg_g.LegacySanitizeLabelForCache(accName)+".json")
	if err := os.WriteFile(oldPath, []byte(`{"access_token":"old-token"}`), 0600); err != nil {
		t.Fatalf("seeding old-sanitized token: %v", err)
	}

	newPath := gmail.GetTokenPath(cfg)
	if newPath == oldPath {
		t.Fatalf("test fixture invalid: %q sanitizes the same both ways", accName)
	}

	data, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("expected the old token migrated to %q: %v", newPath, err)
	}
	if string(data) != `{"access_token":"old-token"}` {
		t.Errorf("migrated token content mismatch: got %q", data)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("expected old token file removed, stat err = %v", err)
	}
}
