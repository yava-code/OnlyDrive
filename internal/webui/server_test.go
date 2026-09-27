package webui

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func testServer(t *testing.T, token string) *Server {
	t.Helper()
	t.Setenv("GD_HOME", t.TempDir())
	if token != "" {
		t.Setenv("GD_UI_TOKEN", token)
	} else {
		os.Unsetenv("GD_UI_TOKEN")
	}
	return New(DefaultPort)
}

func TestStateEndpointEmpty(t *testing.T) {
	s := testServer(t, "")
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("state: got HTTP %d", resp.StatusCode)
	}
	var st statePayload
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	// Empty environment: empty state must be valid JSON with no accounts.
	if len(st.Accounts) != 0 {
		t.Fatalf("expected 0 accounts, got %d", len(st.Accounts))
	}
}

func TestIndexPageServed(t *testing.T) {
	s := testServer(t, "")
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("index: got HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	page := string(body)
	for _, want := range []string{"gd control panel", "Add Google account", "btn-doctor", "logo.png"} {
		if !strings.Contains(page, want) {
			t.Fatalf("index page missing %q", want)
		}
	}
}

func TestIndexNotFoundPath(t *testing.T) {
	s := testServer(t, "")
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/nope")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown path, got %d", resp.StatusCode)
	}
}

func TestLogoEmbedded(t *testing.T) {
	s := testServer(t, "")
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/logo.png")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logo: got HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 1000 {
		t.Fatalf("logo suspiciously small: %d bytes", len(data))
	}
	// PNG magic
	if data[0] != 0x89 || data[1] != 'P' {
		t.Fatal("logo is not a PNG")
	}
}

func TestTokenGate(t *testing.T) {
	s := testServer(t, "sekrit")
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	// No token: rejected.
	resp, err := http.Get(srv.URL + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", resp.StatusCode)
	}
	// Wrong token: rejected.
	req, _ := http.NewRequest("GET", srv.URL+"/api/state", nil)
	req.Header.Set("X-GD-Token", "wrong")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 with wrong token, got %d", resp2.StatusCode)
	}
	// Right token: accepted.
	req2, _ := http.NewRequest("GET", srv.URL+"/api/state", nil)
	req2.Header.Set("X-GD-Token", "sekrit")
	resp3, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with correct token, got %d", resp3.StatusCode)
	}
}

func TestActionValidation(t *testing.T) {
	s := testServer(t, "")
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	// daemon with invalid action must 400
	resp, err := http.Post(srv.URL+"/api/daemon", "application/json", strings.NewReader(`{"action":"explode"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("daemon bad action: expected 400, got %d", resp.StatusCode)
	}
	// autostart with invalid action must 400
	resp2, err := http.Post(srv.URL+"/api/autostart", "application/json", strings.NewReader(`{"action":"maybe"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest {
		t.Fatalf("autostart bad action: expected 400, got %d", resp2.StatusCode)
	}
	// remove with unknown account must 400
	resp3, err := http.Post(srv.URL+"/api/remove", "application/json", strings.NewReader(`{"account":"ghost"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp3.Body.Close()
	if resp3.StatusCode != http.StatusBadRequest {
		t.Fatalf("remove unknown account: expected 400, got %d", resp3.StatusCode)
	}
	// mount with no accounts must 400
	resp4, err := http.Post(srv.URL+"/api/mount", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	resp4.Body.Close()
	if resp4.StatusCode != http.StatusBadRequest {
		t.Fatalf("mount with no accounts: expected 400, got %d", resp4.StatusCode)
	}
}

func TestMethodWrongVerb(t *testing.T) {
	s := testServer(t, "")
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	// POST to a GET-only route: must be rejected, not swallowed by catch-all.
	resp, err := http.Post(srv.URL+"/api/state", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for POST /api/state, got %d", resp.StatusCode)
	}
}
