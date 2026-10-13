package report

import "testing"

func TestDiscardAcceptsEverything(_ *testing.T) {
	r := Discard()
	r.Infof("a %d", 1)
	r.Successf("b")
	r.Warnf("c")
	r.Errorf("d")
	r.Printf("e")
	r.Progress(1, 2, "f")
}

func TestOrSubstitutesDiscardForNil(t *testing.T) {
	if Or(nil) == nil {
		t.Fatal("expected a usable Reporter for a nil argument")
	}
	r := Discard()
	if Or(r) != r {
		t.Error("expected Or to return the reporter it was given")
	}
}
