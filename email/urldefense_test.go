package email

import (
	"testing"
)

func TestDecodeURLDefense_V1(t *testing.T) {
	v1 := "https://urldefense.proofpoint.com/v1/url?u=https%3A%2F%2Fwww.google.com%2Fsearch%3Fq%3Dgolang&k=p5W0"
	decoded, ok := DecodeURLDefense(v1)
	if !ok {
		t.Fatalf("expected ok=true for v1")
	}
	expected := "https://www.google.com/search?q=golang"
	if decoded != expected {
		t.Errorf("expected %q, got %q", expected, decoded)
	}
}

func TestDecodeURLDefense_V2(t *testing.T) {
	v2 := "https://urldefense.proofpoint.com/v2/url?u=https-3A__www.example.com_path-2Dname-3Fquery-3D1-26test-3D2&d=DwIFAg"
	decoded, ok := DecodeURLDefense(v2)
	if !ok {
		t.Fatalf("expected ok=true for v2")
	}
	expected := "https://www.example.com/path-name?query=1&test=2"
	if decoded != expected {
		t.Errorf("expected %q, got %q", expected, decoded)
	}
}

func TestDecodeURLDefense_V3_Plain(t *testing.T) {
	v3 := "https://urldefense.com/v3/__https://www.google.com/__;!!abc!def$"
	decoded, ok := DecodeURLDefense(v3)
	if !ok {
		t.Fatalf("expected ok=true for v3 plain")
	}
	expected := "https://www.google.com/"
	if decoded != expected {
		t.Errorf("expected %q, got %q", expected, decoded)
	}
}

func TestDecodeURLDefense_V3_WithReplacements(t *testing.T) {
	// "IyM=" is base64 for "##"
	v3 := "https://urldefense.com/v3/__http://www.example.com/*test*test__;IyM=!!abc!def$"
	decoded, ok := DecodeURLDefense(v3)
	if !ok {
		t.Fatalf("expected ok=true for v3 with replacements")
	}
	expected := "http://www.example.com/#test#test"
	if decoded != expected {
		t.Errorf("expected %q, got %q", expected, decoded)
	}
}

func TestUnwrapURLDefenseInText(t *testing.T) {
	text := "Please check https://urldefense.com/v3/__https://github.com/__;!!abc!def$ for updates."
	unwrapped := UnwrapURLDefenseInText(text)
	expected := "Please check https://github.com/ for updates."
	if unwrapped != expected {
		t.Errorf("expected %q, got %q", expected, unwrapped)
	}
}
