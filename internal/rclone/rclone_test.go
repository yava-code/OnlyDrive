package rclone

import (
	"context"
	"encoding/base64"
	"testing"
	"time"
)

func TestExtractTokenBlob(t *testing.T) {
	out := "Paste the following into your remote machine --->\n{\"access_token\":\"aaa\",\"expiry\":\"2026-01-01T00:00:00Z\"}  \nSuccess!"
	got := extractTokenBlob(out)
	if got != `{"access_token":"aaa","expiry":"2026-01-01T00:00:00Z"}` {
		t.Fatalf("got %q", got)
	}
	if extractTokenBlob("nothing here") != "" {
		t.Fatal("expected empty for non-token output")
	}
	// nested braces
	nested := `{"access_token":"a","extra":{"b":1}}`
	if extractTokenBlob(nested) != nested {
		t.Fatalf("nested parse failed: %q", extractTokenBlob(nested))
	}
}

func TestExtractURL(t *testing.T) {
	// rclone authorize --auth-no-open-browser prints this NOTICE (on stderr):
	out := "NOTICE: Please go to the following link: http://127.0.0.1:53682/auth?state=FopnWW7WAiFRZa9wD1s7RQ\nLog in and authorize rclone for access"
	got := extractURL(out)
	if got != "http://127.0.0.1:53682/auth?state=FopnWW7WAiFRZa9wD1s7RQ" {
		t.Fatalf("got %q", got)
	}
	if extractURL("see https://example.com/only") != "" {
		t.Fatal("expected non-auth URL to be ignored")
	}
}

func TestBase64Encode(t *testing.T) {
	if base64Encode([]byte("hello")) != base64.StdEncoding.EncodeToString([]byte("hello")) {
		t.Fatal("base64 mismatch")
	}
}

func TestTruncate(t *testing.T) {
	if truncate("abcdef", 3) != "abc…" {
		t.Fatalf("got %q", truncate("abcdef", 3))
	}
	if truncate("ab", 3) != "ab" {
		t.Fatal("short string must not be truncated")
	}
}

func TestEmailForTokenErrors(t *testing.T) {
	// malformed blob
	if _, err := EmailForToken(context.Background(), "not-json"); err == nil {
		t.Fatal("expected error for malformed token blob")
	}
	// missing access_token
	if _, err := EmailForToken(context.Background(), `{}`); err == nil {
		t.Fatal("expected error for blob without access_token")
	}
	// bogus token -> Google returns non-200; must surface an error, not panic
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := EmailForToken(ctx, `{"access_token":"bogus"}`); err == nil {
		t.Fatal("expected error for invalid access token")
	}
}
