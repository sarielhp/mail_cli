package label

import (
	"errors"
	"testing"
)

func TestValidateNewName(t *testing.T) {
	for _, name := range []string{"%foo", " %foo", "%", "%Work/Sub"} {
		if err := ValidateNewName(name); !errors.Is(err, ErrReservedName) {
			t.Errorf("ValidateNewName(%q) = %v, want ErrReservedName", name, err)
		}
	}
	for _, name := range []string{"foo", "Work/%sub", "50% off"} {
		if err := ValidateNewName(name); err != nil {
			t.Errorf("ValidateNewName(%q) = %v, want nil", name, err)
		}
	}
}

func TestForCreate(t *testing.T) {
	if _, err := ForCreate("%oops"); !errors.Is(err, ErrReservedName) {
		t.Errorf("ForCreate(%%oops) = %v, want ErrReservedName", err)
	}
	f, err := ForCreate("Work/New")
	if err != nil || f.String() != "Work/New" || f.IsZero() {
		t.Errorf("ForCreate(Work/New) = %q, %v", f, err)
	}
	if !(Folder{}).IsZero() {
		t.Error("zero Folder is not IsZero")
	}
}
