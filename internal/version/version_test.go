package version

import "testing"

func TestNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"1.2.0", "1.1.9", true},
		{"v1.2.0", "1.2.0", false},
		{"1.2.0", "1.2.1", false},
		{"1.10.0", "1.9.0", true},
		{"1.2.0", "dev", true},
		{"dev", "1.2.0", false},
		{"", "1.0.0", false},
	}
	for _, tc := range cases {
		if got := Newer(tc.latest, tc.current); got != tc.want {
			t.Errorf("Newer(%q, %q)=%v want %v", tc.latest, tc.current, got, tc.want)
		}
	}
}
