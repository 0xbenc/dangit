package cli

import "testing"

func TestIntroVersionLabel(t *testing.T) {
	cases := map[string]string{"": "dev", "dev": "dev", "1.2.3": "v1.2.3", "0.2.0": "v0.2.0"}
	for in, want := range cases {
		if got := introVersionLabel(in); got != want {
			t.Errorf("introVersionLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIntroDecision(t *testing.T) {
	cases := []struct {
		name        string
		f           flags
		env         []string
		last, build string
		want        bool
	}{
		{"first run plays", flags{}, nil, "", "1.0.0", true},
		{"same version stays quiet", flags{}, nil, "1.0.0", "1.0.0", false},
		{"new version plays", flags{}, nil, "0.9.0", "1.0.0", true},
		{"--no-intro suppresses", flags{noIntro: true}, nil, "", "1.0.0", false},
		{"DANGIT_NO_INTRO suppresses", flags{}, []string{"DANGIT_NO_INTRO=1"}, "", "1.0.0", false},
		{"--intro forces same version", flags{intro: true}, nil, "1.0.0", "1.0.0", true},
		{"DANGIT_INTRO_ALWAYS forces", flags{}, []string{"DANGIT_INTRO_ALWAYS=1"}, "1.0.0", "1.0.0", true},
		{"suppress beats force (flags)", flags{intro: true, noIntro: true}, nil, "1.0.0", "1.0.0", false},
		{"suppress beats force (env)", flags{}, []string{"DANGIT_NO_INTRO=1", "DANGIT_INTRO_ALWAYS=1"}, "", "1.0.0", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := introDecision(tc.f, tc.env, tc.last, tc.build); got != tc.want {
				t.Errorf("introDecision = %v, want %v", got, tc.want)
			}
		})
	}
}
