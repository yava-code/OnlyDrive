package main

import "testing"

func TestMaskSecret(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "(not set)"},
		// <= 8 chars: only asterisks, no leak of length-adjacent hints.
		{"short", "*****"},
		// > 8: first 4, ellipsis, last 4.
		{"GOCSPX-1234567890abcdefg", "GOCS…defg"},
	}
	for _, c := range cases {
		if got := maskSecret(c.in); got != c.want {
			t.Errorf("maskSecret(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
