package scan

import "testing"

// TestScanDisallowedScriptNeedsMoreThanOneLetter guards the false positive that
// filed legitimate mail as spam: the audit used to trip on the first
// out-of-script letter anywhere in the subject, From header, or body.
func TestScanDisallowedScriptNeedsMoreThanOneLetter(t *testing.T) {
	allowed := []string{"english"}

	for _, tc := range []struct {
		name string
		text string
		want bool
	}{
		{"plain english", "Quarterly report attached", false},
		{"one cyrillic letter in a signature", "Regards, Ivan Ivanov (Иvanov)", false},
		{"two cyrillic letters", "Regards, Иvanov Иvanovich", false},
		{"a run of cyrillic", "Привет, дорогой друг", true},
		{"isolated greek is mathematical notation", "the angle theta (θ) is fixed", false},
		{"a chinese product name", "Order for 小 confirmed", false},
		{"a wall of han", "免费赚钱快速致富点击这里", true},
		{"digits and punctuation are never letters", "!!! 12345 ??? $$$", false},
	} {
		got, _, _ := disallowedScript(tc.text, allowed)
		if got != tc.want {
			t.Errorf("%s: disallowedScript(%q) = %v, want %v", tc.name, tc.text, got, tc.want)
		}
	}
}

// TestAllowedLanguagesIsConfigurable guards against the hardcoded switch that
// recognised only english/german/french/hebrew, so adding any other language to
// allowed_languages produced no error and no effect.
func TestAllowedLanguagesIsConfigurable(t *testing.T) {
	russian := "Привет, дорогой друг"
	if blocked, _, _ := disallowedScript(russian, []string{"english"}); !blocked {
		t.Error("Cyrillic should be blocked when only english is allowed")
	}
	if blocked, _, _ := disallowedScript(russian, []string{"english", "russian"}); blocked {
		t.Error("adding russian to allowed_languages must actually permit Cyrillic")
	}

	chinese := "免费赚钱快速致富点击这里"
	if blocked, _, _ := disallowedScript(chinese, []string{"english", "chinese"}); blocked {
		t.Error("adding chinese to allowed_languages must actually permit Han")
	}

	hebrew := "שלום חבר יקר מאוד"
	if blocked, _, _ := disallowedScript(hebrew, []string{"english", "hebrew"}); blocked {
		t.Error("hebrew should remain permitted")
	}
}

func TestKnownLanguage(t *testing.T) {
	for _, l := range []string{"english", "Hebrew", " russian ", "chinese"} {
		if !KnownLanguage(l) {
			t.Errorf("KnownLanguage(%q) = false, want true", l)
		}
	}
	if KnownLanguage("klingon") {
		t.Error("KnownLanguage(\"klingon\") = true, want false")
	}
}
