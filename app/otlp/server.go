package otlp

import (
	"compress/gzip"
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// TokenHeader carries the shared secret (set through
// OTEL_EXPORTER_OTLP_HEADERS=x-mimir-token=...). A Bearer Authorization
// header with the same value is accepted too.
const TokenHeader = "X-Mimir-Token"

// DefaultMaxBody bounds one export request after decompression.
const DefaultMaxBody = 4 << 20

// Server is a loopback-only OTLP/HTTP JSON receiver.
type Server struct {
	// Addr is the listen address; only loopback hosts are accepted.
	Addr string
	// Token, when set, is required on every request.
	Token string
	// MaxBody caps a request body (DefaultMaxBody when 0).
	MaxBody int64
	// AcceptTraces parses /v1/traces; otherwise spans are acknowledged and
	// dropped.
	AcceptTraces bool
	OnLogs       func([]Record)
	OnSpans      func([]Span)
	// OnReject is told about refused requests (status, reason).
	OnReject func(status int, reason string)

	mu   sync.Mutex
	srv  *http.Server
	ln   net.Listener
	addr string
}

// ErrNotLoopback refuses listen addresses other than 127.0.0.1 / ::1.
var ErrNotLoopback = errors.New("telemetry listener must bind a loopback address")

// Start binds and serves in the background. The bound address (with the
// real port) is available through ListenAddr afterwards.
func (s *Server) Start() error {
	host, _, err := net.SplitHostPort(s.Addr)
	if err != nil {
		return fmt.Errorf("telemetry listen address: %w", err)
	}
	if !isLoopback(host) {
		return ErrNotLoopback
	}
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
	s.mu.Lock()
	s.srv, s.ln, s.addr = srv, ln, ln.Addr().String()
	s.mu.Unlock()
	go func() { _ = srv.Serve(ln) }()
	return nil
}

// ListenAddr is the bound address, "" when not running.
func (s *Server) ListenAddr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addr
}

// Stop shuts the listener down.
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	srv := s.srv
	s.srv, s.ln, s.addr = nil, nil, ""
	s.mu.Unlock()
	if srv == nil {
		return nil
	}
	return srv.Shutdown(ctx)
}

// Handler serves /v1/logs, /v1/traces and /v1/metrics.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/logs", s.handle(func(body []byte) error {
		recs, err := ParseLogs(body)
		if err != nil {
			return err
		}
		if s.OnLogs != nil && len(recs) > 0 {
			s.OnLogs(recs)
		}
		return nil
	}))
	mux.HandleFunc("/v1/traces", s.handle(func(body []byte) error {
		if !s.AcceptTraces {
			return nil
		}
		spans, err := ParseSpans(body)
		if err != nil {
			return err
		}
		if s.OnSpans != nil && len(spans) > 0 {
			s.OnSpans(spans)
		}
		return nil
	}))
	// Metrics carry nothing the timeline needs; acknowledge so an exporter
	// that was pointed here anyway does not log errors.
	mux.HandleFunc("/v1/metrics", s.handle(func([]byte) error { return nil }))
	return mux
}

func (s *Server) reject(w http.ResponseWriter, status int, reason string) {
	if s.OnReject != nil {
		s.OnReject(status, reason)
	}
	http.Error(w, reason, status)
}

func (s *Server) handle(process func(body []byte) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			s.reject(w, http.StatusMethodNotAllowed, "POST only")
			return
		}
		if host, _, err := net.SplitHostPort(r.RemoteAddr); err != nil || !isLoopback(host) {
			s.reject(w, http.StatusForbidden, "loopback only")
			return
		}
		if s.Token != "" && !s.tokenOK(r) {
			s.reject(w, http.StatusUnauthorized, "missing or wrong token")
			return
		}
		ct := strings.ToLower(r.Header.Get("Content-Type"))
		if !strings.HasPrefix(ct, "application/json") {
			s.reject(w, http.StatusUnsupportedMediaType, "application/json expected (OTEL_EXPORTER_OTLP_PROTOCOL=http/json)")
			return
		}
		max := s.MaxBody
		if max <= 0 {
			max = DefaultMaxBody
		}
		var reader io.Reader = http.MaxBytesReader(w, r.Body, max)
		if strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip") {
			gz, err := gzip.NewReader(reader)
			if err != nil {
				s.reject(w, http.StatusBadRequest, "bad gzip")
				return
			}
			defer gz.Close()
			reader = io.LimitReader(gz, max+1)
		}
		body, err := io.ReadAll(reader)
		if err != nil || int64(len(body)) > max {
			s.reject(w, http.StatusRequestEntityTooLarge, "body too large")
			return
		}
		if err := process(body); err != nil {
			s.reject(w, http.StatusBadRequest, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}
}

func (s *Server) tokenOK(r *http.Request) bool {
	got := r.Header.Get(TokenHeader)
	if got == "" {
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			got = strings.TrimSpace(auth[len("bearer "):])
		}
	}
	return got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(s.Token)) == 1
}

func isLoopback(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}
