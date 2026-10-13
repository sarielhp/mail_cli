package app

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestFprintError(t *testing.T) {
	var buf bytes.Buffer
	err := errors.New("something went wrong")
	FprintError(&buf, err)

	output := buf.String()
	if !strings.Contains(output, "──────────") {
		t.Errorf("expected separator in output, got: %q", output)
	}
	if !strings.Contains(output, "Error:") {
		t.Errorf("expected Error: in output, got: %q", output)
	}
	if !strings.Contains(output, "something went wrong") {
		t.Errorf("expected error message in output, got: %q", output)
	}
}

func TestFprintError_Nil(t *testing.T) {
	var buf bytes.Buffer
	FprintError(&buf, nil)
	if buf.Len() != 0 {
		t.Errorf("expected empty output for nil error, got: %q", buf.String())
	}
}

func TestFprintErrorf(t *testing.T) {
	var buf bytes.Buffer
	FprintErrorf(&buf, "failed with code %d", 404)

	output := buf.String()
	if !strings.Contains(output, "failed with code 404") {
		t.Errorf("expected formatted message in output, got: %q", output)
	}
}
