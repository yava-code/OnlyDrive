package config

import "testing"

// TestIsDriveLetter pins the lexical split between Windows drive-letter
// targets and macOS/Linux mountpoint paths stored in Mount.Letter.
func TestIsDriveLetter(t *testing.T) {
	cases := map[string]bool{
		"X:":       true,
		"Z:":       true,
		"z:":       false, // gd only ever stores uppercase letters
		"X":        false,
		"X:\\":     false,
		"ABC":      false,
		"/tmp/mnt": false,
		"C:/":      false,
		"":         false,
	}
	for in, want := range cases {
		if got := IsDriveLetter(in); got != want {
			t.Errorf("IsDriveLetter(%q) = %v, want %v", in, got, want)
		}
	}
}
