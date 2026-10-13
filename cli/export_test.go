package cli

import (
	"errors"
	"testing"

	"github.com/sarielhp/mail_cli/app"
	"github.com/sarielhp/mail_cli/cfg_g"
)

func TestRunExport_Validation(t *testing.T) {
	session := &Session{
		Config: &cfg_g.Config{},
	}

	// 1. Missing args
	err := runExport(session, nil)
	if err == nil {
		t.Fatalf("expected error for missing args")
	}
	if !errors.Is(err, app.ErrUsage) {
		t.Errorf("expected ErrUsage, got %v", err)
	}

	// 2. Empty ID
	err = runExport(session, []string{"   "})
	if err == nil {
		t.Fatalf("expected error for empty ID")
	}
	if !errors.Is(err, app.ErrUsage) {
		t.Errorf("expected ErrUsage, got %v", err)
	}

	// 3. Invalid format
	flagExportFormat = "invalid_format"
	defer func() { flagExportFormat = "" }()
	err = runExport(session, []string{"msg123"})
	if err == nil {
		t.Fatalf("expected error for invalid format")
	}
	if !errors.Is(err, app.ErrUsage) {
		t.Errorf("expected ErrUsage, got %v", err)
	}
}

func TestRunExport_PassesOptions(t *testing.T) {
	var capturedOpts ExportOptions
	called := false

	session := dryRunSession("work")
	session.Deps.Mail = stubMail{ExportFn: func(_ *cfg_g.Config, opts ExportOptions) error {
		called = true
		capturedOpts = opts
		return nil
	}}

	flagExportOutput = "thread.mbox"
	flagExportFormat = "mbox"
	flagExportSingle = true
	defer func() {
		flagExportOutput = ""
		flagExportFormat = ""
		flagExportSingle = false
	}()

	err := runExport(session, []string{"thread001"})
	if err != nil {
		t.Fatalf("runExport failed: %v", err)
	}
	if !called {
		t.Fatalf("expected Mail.Export to be called")
	}

	if capturedOpts.MessageID != "thread001" {
		t.Errorf("MessageID = %q, want 'thread001'", capturedOpts.MessageID)
	}
	if capturedOpts.Account != "work" {
		t.Errorf("Account = %q, want 'work'", capturedOpts.Account)
	}
	if capturedOpts.Output != "thread.mbox" {
		t.Errorf("Output = %q, want 'thread.mbox'", capturedOpts.Output)
	}
	if capturedOpts.Format != "mbox" {
		t.Errorf("Format = %q, want 'mbox'", capturedOpts.Format)
	}
	if !capturedOpts.Single {
		t.Errorf("Single = false, want true")
	}
	if !capturedOpts.DryRun {
		t.Errorf("DryRun = false, want true")
	}
}
