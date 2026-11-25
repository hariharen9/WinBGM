package backend

import "testing"

func TestIsNewerSemver(t *testing.T) {
	cases := []struct {
		latest  string
		current string
		want    bool
	}{
		{"1.0.1", "1.0.0", true},
		{"v1.1.0", "1.0.0", true},
		{"2.0.0", "1.9.9", true},
		{"1.0.0", "1.0.0", false},
		{"0.9.9", "1.0.0", false},
	}
	for _, tc := range cases {
		if got := isNewerSemver(tc.latest, tc.current); got != tc.want {
			t.Errorf("isNewerSemver(%q, %q) = %v, want %v", tc.latest, tc.current, got, tc.want)
		}
	}
}

func TestSanitizeTaskName(t *testing.T) {
	if got := sanitizeTaskName(`My App: v1/2`); got != "My App_ v1_2" {
		t.Errorf("unexpected sanitizeTaskName: %q", got)
	}
}
