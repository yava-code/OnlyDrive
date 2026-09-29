package main

import (
	"testing"

	"gd/internal/config"
)

// The mask logic itself is covered by TestMaskSecret in internal/config;
// here we only pin the exact masked output line of `gd oauth show`, which
// was verified live.
func TestOAuthShowMasksSecret(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "(not set)"},
		{"short", "*****"},
		{"GOCSPX-1234567890abcdefg", "GOCS…defg"},
	}
	for _, c := range cases {
		if got := config.MaskSecret(c.in); got != c.want {
			t.Errorf("MaskSecret(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
