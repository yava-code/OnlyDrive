// Package webui serves the gd control panel: a single-page dashboard over the
// existing CLI internals (config, daemon, serve, doctor). Localhost-only by
// design; an optional access token can be required via GD_UI_TOKEN.
package webui

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gd/internal/config"
	"gd/internal/daemon"
	"gd/internal/doctor"
	"gd/internal/rclone"
	"gd/internal/serve"
)

// DefaultPort is the port the UI listens on when --port is not given.
const DefaultPort = 5590

// Server is the gd control panel HTTP server.
type Server struct {
	Port     int
	Token    string // optional; when set, every request must present it
	srv      *http.Server
	actionMu sync.Mutex // serializes mutating actions (add/mount/daemon/...)
}

// New builds a Server reading the access token from GD_UI_TOKEN (optional).
func New(port int) *Server {
	return &Server{Port: port, Token: os.Getenv("GD_UI_TOKEN")}
}

// actionOK is the JSON body for successful actions.
type actionOK struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// actionErr is the JSON body for failed actions.
type actionErr struct {
	Error string `json:"error"`
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// authMiddleware enforces the optional token gate.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.Token != "" && r.Header.Get("X-GD-Token") != s.Token {
			writeJSON(w, http.StatusUnauthorized, actionErr{Error: "unauthorized: set X-GD-Token header"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Handler builds the full HTTP handler (exported for tests).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("POST /api/add", s.action(s.handleAdd))
	mux.HandleFunc("POST /api/mount", s.action(s.handleMount))
	mux.HandleFunc("POST /api/unmount", s.action(s.handleUnmount))
	mux.HandleFunc("POST /api/daemon", s.action(s.handleDaemon))
	mux.HandleFunc("POST /api/serve-s3", s.action(s.handleServeS3))
	mux.HandleFunc("POST /api/autostart", s.action(s.handleAutostart))
	mux.HandleFunc("POST /api/doctor", s.action(s.handleDoctor))
	mux.HandleFunc("POST /api/remove", s.action(s.handleRemove))
	mux.HandleFunc("/", s.handleIndex)
	return s.authMiddleware(mux)
}

// Run starts the (blocking) HTTP server on 127.0.0.1.
func (s *Server) Run() error {
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(s.Port))
	s.srv = &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w (is another gd ui running? try --port)", addr, err)
	}
	return s.srv.Serve(ln)
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}

// handleIndex serves the single-page dashboard. Registered as catch-all, so
// wrong-method requests to API paths land here too: answer 405, not 404.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Allow", "GET, POST")
		writeJSON(w, http.StatusMethodNotAllowed, actionErr{Error: "method not allowed for this endpoint"})
		return
	}
	// embedded logo (served from the binary, no external assets)
	if r.URL.Path == "/logo.png" {
		data, err := assets.ReadFile("assets/logo.png")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(data)
		return
	}
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSON(w, http.StatusMethodNotAllowed, actionErr{Error: "method not allowed"})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(indexHTML))
}

// action wraps a mutating handler with auth and panic-safety.
func (s *Server) action(h func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.actionMu.Lock()
		defer s.actionMu.Unlock()
		h(w, r)
	}
}

// handleState aggregates everything the dashboard shows.
func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, actionErr{Error: err.Error()})
		return
	}
	st := statePayload{
		DaemonRunning: daemon.Running(),
		Accounts:      make([]accountView, 0, len(cfg.Accounts)),
		Mounts:        make([]config.Mount, 0, len(cfg.Mounts)),
		S3Running:     false,
	}
	s3Running, s3Key, s3Secret, s3Addr := serve.S3Status()
	st.S3Running = s3Running
	st.S3Addr = s3Addr
	st.S3AccessKey = s3Key
	st.S3SecretKey = s3Secret

	if daemon.Running() {
		if m, err := manager(); err == nil {
			ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
			defer cancel()
			if mounts, err := m.ListMounts(ctx); err == nil {
				for _, mm := range mounts {
					if pt, ok := mm["MountPoint"].(string); ok {
						st.ActiveMounts = append(st.ActiveMounts, pt)
					}
				}
			}
		}
	}
	for _, a := range cfg.Accounts {
		av := accountView{
			Name:    a.Name,
			Email:   a.Email,
			Remote:  a.Remote,
			Mounted: cfg.MountForAccount(a.Name) != nil,
		}
		if m, err := manager(); err == nil {
			ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
			defer cancel()
			if about, err := m.About(ctx, a.Remote+":"); err == nil {
				av.Total, _ = about["total"].(float64)
				av.Used, _ = about["used"].(float64)
				av.QuotaOK = true
			}
		}
		st.Accounts = append(st.Accounts, av)
	}
	writeJSON(w, http.StatusOK, st)
}

// statePayload is the GET /api/state response.
type statePayload struct {
	DaemonRunning bool           `json:"daemon_running"`
	Accounts      []accountView  `json:"accounts"`
	Mounts        []config.Mount `json:"mounts"`
	ActiveMounts  []string       `json:"active_mounts"`
	S3Running     bool           `json:"s3_running"`
	S3Addr        string         `json:"s3_addr,omitempty"`
	S3AccessKey   string         `json:"s3_access_key,omitempty"`
	S3SecretKey   string         `json:"s3_secret_key,omitempty"`
}

// accountView is one account row in the dashboard.
type accountView struct {
	Name    string  `json:"name"`
	Email   string  `json:"email"`
	Remote  string  `json:"remote"`
	Mounted bool    `json:"mounted"`
	QuotaOK bool    `json:"quota_ok"`
	Total   float64 `json:"total,omitempty"`
	Used    float64 `json:"used,omitempty"`
}

// manager builds an authenticated rclone Manager (same as MCP server does).
func manager() (*rclone.Manager, error) {
	m, err := rclone.New()
	if err != nil {
		return nil, err
	}
	pass, err := daemon.LoadOrCreatePass()
	if err != nil {
		return nil, err
	}
	m.SetAuth("gd", pass)
	return m, nil
}

// handleAdd starts the OAuth flow for one more Google account.
func (s *Server) handleAdd(w http.ResponseWriter, r *http.Request) {
	msg, err := addAccount()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, actionErr{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, actionOK{OK: true, Message: msg})
}

// handleMount mounts one account (body: {"account":"acc1"} or first).
func (s *Server) handleMount(w http.ResponseWriter, r *http.Request) {
	acc, err := accountFromBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, actionErr{Error: err.Error()})
		return
	}
	if err := runMount(acc); err != nil {
		writeJSON(w, http.StatusInternalServerError, actionErr{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, actionOK{OK: true, Message: "mounted " + acc})
}

// handleUnmount unmounts one account (body: {"account":"acc1"}).
func (s *Server) handleUnmount(w http.ResponseWriter, r *http.Request) {
	acc, err := accountFromBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, actionErr{Error: err.Error()})
		return
	}
	if err := runUnmount(acc); err != nil {
		writeJSON(w, http.StatusInternalServerError, actionErr{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, actionOK{OK: true, Message: "unmounted " + acc})
}

// handleDaemon starts or stops the background daemon (body: {"action":"start"|"stop"}).
func (s *Server) handleDaemon(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Action string `json:"action"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	switch in.Action {
	case "start":
		st, err := daemon.Start()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, actionErr{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, actionOK{OK: true, Message: "daemon: " + st})
	case "stop":
		if err := daemon.Stop(); err != nil {
			writeJSON(w, http.StatusInternalServerError, actionErr{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, actionOK{OK: true, Message: "daemon stopped"})
	default:
		writeJSON(w, http.StatusBadRequest, actionErr{Error: `action must be "start" or "stop"`})
	}
}

// handleServeS3 starts or stops the S3 endpoint (body: {"action":"start"|"stop"}).
func (s *Server) handleServeS3(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Action string `json:"action"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	switch in.Action {
	case "start":
		if _, err := serve.EnsureUnion(); err != nil {
			writeJSON(w, http.StatusInternalServerError, actionErr{Error: err.Error()})
			return
		}
		acc, sec, msg, err := serve.S3Start("gd-union", 9000)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, actionErr{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, actionOK{OK: true, Message: msg + "  access_key=" + acc + "  secret_key=" + sec})
	case "stop":
		if err := serve.S3Stop(); err != nil {
			writeJSON(w, http.StatusInternalServerError, actionErr{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, actionOK{OK: true, Message: "S3 stopped"})
	default:
		writeJSON(w, http.StatusBadRequest, actionErr{Error: `action must be "start" or "stop"`})
	}
}

// handleAutostart turns logon autostart on or off (body: {"action":"on"|"off"}).
func (s *Server) handleAutostart(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Action string `json:"action"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	switch in.Action {
	case "on":
		detail, err := serve.AutostartOn()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, actionErr{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, actionOK{OK: true, Message: "autostart registered: " + detail})
	case "off":
		if err := serve.AutostartOff(); err != nil {
			writeJSON(w, http.StatusInternalServerError, actionErr{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, actionOK{OK: true, Message: "autostart removed"})
	default:
		writeJSON(w, http.StatusBadRequest, actionErr{Error: `action must be "on" or "off"`})
	}
}

// handleDoctor runs diagnostics (body: {"fix":true} to auto-fix).
func (s *Server) handleDoctor(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Fix bool `json:"fix"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	checks, err := doctor.Run(ctx, in.Fix)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, actionErr{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "doctor finished",
		"checks":  checks,
	})
}

// handleRemove deletes an account (body: {"account":"acc1"}).
func (s *Server) handleRemove(w http.ResponseWriter, r *http.Request) {
	acc, err := accountFromBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, actionErr{Error: err.Error()})
		return
	}
	if err := removeAccount(acc); err != nil {
		writeJSON(w, http.StatusInternalServerError, actionErr{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, actionOK{OK: true, Message: "removed " + acc})
}

// accountFromBody extracts {"account": "..."} allowing empty for "first".
func accountFromBody(r *http.Request) (string, error) {
	var in struct {
		Account string `json:"account"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		return "", err
	}
	acc := strings.TrimSpace(in.Account)
	if acc == "" {
		cfg, err := config.Load()
		if err != nil {
			return "", err
		}
		if len(cfg.Accounts) == 0 {
			return "", fmt.Errorf("no accounts configured")
		}
		return cfg.Accounts[0].Name, nil
	}
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	if cfg.AccountByName(acc) == nil {
		return "", fmt.Errorf("account %q not found", acc)
	}
	return cfg.AccountByName(acc).Name, nil
}
