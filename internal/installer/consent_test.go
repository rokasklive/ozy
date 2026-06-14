package installer

import (
	"strings"
	"testing"
)

func TestConsentPolicyDecide(t *testing.T) {
	cases := []struct {
		name   string
		policy ConsentPolicy
		risk   Risk
		want   Decision
	}{
		{"ordinary + yes auto-proceeds", ConsentPolicy{AssumeYes: true, Interactive: true}, Ordinary, Proceed},
		{"ordinary without yes asks", ConsentPolicy{AssumeYes: false, Interactive: true}, Ordinary, AskUser},
		{"ordinary non-interactive no consent", ConsentPolicy{AssumeYes: false, Interactive: false}, Ordinary, SkipNoConsent},
		// The load-bearing rule: --yes must NOT auto-accept risky actions.
		{"risky + yes still asks", ConsentPolicy{AssumeYes: true, Interactive: true}, Risky, AskUser},
		{"risky non-interactive is skipped", ConsentPolicy{AssumeYes: true, Interactive: false}, Risky, SkipNoConsent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.policy.Decide(tc.risk); got != tc.want {
				t.Errorf("Decide(%v) = %v, want %v", tc.risk, got, tc.want)
			}
		})
	}
}

func TestConfirm(t *testing.T) {
	cases := []struct {
		in   string
		def  bool
		want bool
	}{
		{"y\n", false, true},
		{"yes\n", false, true},
		{"n\n", true, false},
		{"no\n", true, false},
		{"\n", true, true},    // empty falls back to default
		{"\n", false, false},  // empty falls back to default
		{"huh\n", true, true}, // unrecognized falls back to default
	}
	for _, tc := range cases {
		if got := Confirm(strings.NewReader(tc.in), tc.def); got != tc.want {
			t.Errorf("Confirm(%q, def=%v) = %v, want %v", tc.in, tc.def, got, tc.want)
		}
	}
}
