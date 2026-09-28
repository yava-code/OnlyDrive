package version

import (
	"strings"
	"testing"
)

func TestStringIsVPlusNumber(t *testing.T) {
	if String() != "v"+Number {
		t.Fatalf("String() = %q, want %q", String(), "v"+Number)
	}
	if !strings.HasPrefix(String(), "v") {
		t.Fatalf("String() must carry the v prefix, got %q", String())
	}
}
