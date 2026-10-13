package email

import "testing"

func TestIsSafeToAutoBlacklist(t *testing.T) {
	tests := []struct {
		name            string
		sender          string
		listUnsubscribe string
		score           float64
		want            bool
	}{
		{
			name:            "Safe to block campaign",
			sender:          "democrats@bounce.myngp.com",
			listUnsubscribe: "<mailto:unsubscribe@myngp.com>",
			score:           18.5,
			want:            true,
		},
		{
			name:            "Missing list unsubscribe",
			sender:          "democrats@bounce.myngp.com",
			listUnsubscribe: "",
			score:           18.5,
			want:            false,
		},
		{
			name:            "Low score",
			sender:          "democrats@bounce.myngp.com",
			listUnsubscribe: "<mailto:unsubscribe@myngp.com>",
			score:           12.0,
			want:            false,
		},
		{
			name:            "Public provider gmail",
			sender:          "spam-bot@gmail.com",
			listUnsubscribe: "<mailto:unsubscribe@gmail.com>",
			score:           20.0,
			want:            false,
		},
		{
			name:            "Public provider yahoo",
			sender:          "campaign@yahoo.com",
			listUnsubscribe: "<mailto:unsubscribe@yahoo.com>",
			score:           20.0,
			want:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSafeToAutoBlacklist(tt.sender, tt.listUnsubscribe, tt.score)
			if got != tt.want {
				t.Errorf("IsSafeToAutoBlacklist(%q, %q, %f) = %v, want %v", tt.sender, tt.listUnsubscribe, tt.score, got, tt.want)
			}
		})
	}
}

func TestDetectPolitical(t *testing.T) {
	// Instant block
	isPol, score, triggers := DetectPolitical("Important alert", "Please visit actblue.com to support")
	if !isPol || score < 20.0 || len(triggers) == 0 {
		t.Errorf("expected actblue to trigger instant political block: isPol=%v, score=%f, triggers=%v", isPol, score, triggers)
	}

	// Context only (routine political words without solicitation language)
	isPol, score, triggers = DetectPolitical("University Lecture", "Discussion on congress, election history, and the campaign process")
	if isPol {
		t.Errorf("routine political words without donation solicitation should NOT be political: score=%f, triggers=%v", score, triggers)
	}

	// Donation vocabulary without an instant block is no longer detected: the
	// vocabulary scorer that used to catch this was removed as the main source
	// of false positives.
	isPol, score, triggers = DetectPolitical("Urgent Election Update", "The congress election is close, please chip in $5 and donate now deadline midnight")
	if isPol {
		t.Errorf("donation vocabulary without an instant block should NOT be political: score=%f, triggers=%v", score, triggers)
	}

	// Benign email
	isPol, score, triggers = DetectPolitical("Dinner tonight", "Hey, do you want to grab tacos at 7pm?")
	if isPol || score != 0 || len(triggers) != 0 {
		t.Errorf("benign email triggered political detector: score=%f, triggers=%v", score, triggers)
	}
}

// A short message is still political when it carries an instant block: the
// donation-platform domain or PAC disclosure is the signal, and length is not.
func TestShortInstantBlockIsPolitical(t *testing.T) {
	short := "Urgent: donate to the campaign now! Paid for by the Committee. https://actblue.com/donate"
	if isPol, score, triggered := DetectPolitical("Election update", short); !isPol {
		t.Errorf("a short message with an instant block must be political: score=%f, triggers=%v", score, triggered)
	}
}
