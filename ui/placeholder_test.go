package ui

import (
	"testing"
)

func TestPlaceholderEmailIsIdentifiable(t *testing.T) {
	p := PlaceholderEmail("broken1")
	if p.ID != "broken1" {
		t.Errorf("ID = %q", p.ID)
	}
	if p.Subject == "" || p.FromEmail != "[Error]" {
		t.Errorf("placeholder does not read as an error row: %+v", p)
	}
	if p.EmailDate.IsZero() {
		t.Error("placeholder has a zero date, which would sort unpredictably")
	}
}
