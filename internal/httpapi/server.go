/*
Copyright 2026 OpenSSF Scorecard Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package httpapi implements the Scorecard-webapp GET contract over the
// orchestrator (design D1/D11): GET /projects/{host}/{org}/{repo} (+ ?commit=),
// /badge, /capabilities, /health, and /readyz. It uses the standard library's
// net/http with Go 1.22 method+wildcard routing — the same choice `scorecard
// serve` landed on — so the durable pieces can graft upstream.
//
// Result bodies are canonical Scorecard JSON2, served verbatim. Provenance
// (source, resolved commit, date, version, completeness) is carried in response
// headers so the JSON2 body stays webapp-compatible for clients like
// scorecard-mcp (design D4/D12).
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/ossf/scorecard-infra/internal/model"
	"github.com/ossf/scorecard-infra/internal/orchestrator"
	"github.com/ossf/scorecard-infra/internal/scan"
)

// Provenance response headers (design D12).
const (
	headerSource    = "X-Scorecard-Source"
	headerCommit    = "X-Scorecard-Resolved-Commit"
	headerDate      = "X-Scorecard-Generated-At"
	headerVersion   = "X-Scorecard-Version"
	headerComplete  = "X-Scorecard-Complete"
	contentTypeJSON = "application/json"
)

// Producer is the orchestrator behavior the HTTP layer depends on.
type Producer interface {
	GetOrProduce(ctx context.Context, ref model.RepoRef, commit string) (*orchestrator.Outcome, error)
}

// Server serves the Scorecard API contract. Field order favors pointer packing
// (govet fieldalignment); the by-value caps sits last so its non-pointer tail
// does not split the pointer fields.
type Server struct {
	producer Producer
	ready    func(context.Context) error
	logger   *slog.Logger
	caps     Capabilities
}

// Option customizes a Server.
type Option func(*Server)

// WithReadiness sets the readiness probe used by /readyz. A nil check (the
// default) reports ready.
func WithReadiness(ready func(context.Context) error) Option {
	return func(s *Server) { s.ready = ready }
}

// WithLogger sets the request logger.
func WithLogger(l *slog.Logger) Option {
	return func(s *Server) { s.logger = l }
}

// New builds a Server over the given producer and advertised capabilities.
func New(producer Producer, caps Capabilities, opts ...Option) *Server {
	s := &Server{
		producer: producer,
		caps:     caps,
		logger:   slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// ServeConfig configures the HTTP listener and its timeouts.
type ServeConfig struct {
	Addr              string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

// ListenAndServe runs the HTTP server until ctx is cancelled (e.g. by a
// SIGINT/SIGTERM-derived context supplied by main), then shuts down gracefully
// within ShutdownTimeout. It returns nil on a clean shutdown.
func (s *Server) ListenAndServe(ctx context.Context, cfg ServeConfig) error {
	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: orDefaultDuration(cfg.ReadHeaderTimeout, 10*time.Second),
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		err := httpServer.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errCh <- err
	}()

	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("httpapi: server error: %w", err)
		}
		return nil
	case <-ctx.Done():
		s.logger.Info("shutting down HTTP server")
	}

	shutdownTimeout := orDefaultDuration(cfg.ShutdownTimeout, 10*time.Second)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	// A fresh context is intentional: the parent ctx is already cancelled, and
	// shutdown needs its own timeout to drain in-flight requests.
	if err := httpServer.Shutdown(shutdownCtx); err != nil { //nolint:contextcheck // deliberate fresh shutdown deadline
		return fmt.Errorf("httpapi: graceful shutdown: %w", err)
	}
	return nil
}

// orDefaultDuration returns d when positive, otherwise def.
func orDefaultDuration(d, def time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return def
}

// Handler returns the HTTP router for the API contract.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /projects/{host}/{org}/{repo}", s.handleResult)
	mux.HandleFunc("GET /projects/{host}/{org}/{repo}/badge", s.handleBadge)
	mux.HandleFunc("GET /capabilities", s.handleCapabilities)
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /readyz", s.handleReadyz)
	return mux
}

// refFromPath parses and validates the {host}/{org}/{repo} path values and the
// optional ?commit= query parameter, writing a 400 response on failure.
func (s *Server) refFromPath(w http.ResponseWriter, r *http.Request) (model.RepoRef, string, bool) {
	ref, err := model.ParseRepoRef(r.PathValue("host"), r.PathValue("org"), r.PathValue("repo"))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid repository reference",
			"expected /projects/{host}/{org}/{repo} with host github.com or gitlab.com")
		return model.RepoRef{}, "", false
	}
	commit, err := model.ParseCommit(r.URL.Query().Get("commit"))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid commit",
			"commit must be a 40-character hex SHA")
		return model.RepoRef{}, "", false
	}
	return ref, commit, true
}

// handleResult serves GET /projects/{host}/{org}/{repo}[?commit=] as JSON2.
func (s *Server) handleResult(w http.ResponseWriter, r *http.Request) {
	ref, commit, ok := s.refFromPath(w, r)
	if !ok {
		return
	}

	out, err := s.producer.GetOrProduce(r.Context(), ref, commit)
	if err != nil {
		s.writeProduceError(w, ref, err)
		return
	}

	if !out.Ready {
		s.writeNotReady(w, out)
		return
	}

	s.setProvenance(w, out.Provenance)
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(http.StatusOK)
	if _, werr := w.Write(out.Body); werr != nil {
		s.logger.Warn("writing result body", "error", werr)
	}
}

// handleCapabilities serves GET /capabilities.
func (s *Server) handleCapabilities(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, s.caps)
}

// handleHealth serves GET /health (liveness).
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// handleReadyz serves GET /readyz (readiness).
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if s.ready != nil {
		if err := s.ready(r.Context()); err != nil {
			s.writeError(w, http.StatusServiceUnavailable, "not ready", err.Error())
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

// setProvenance writes the provenance response headers (design D12).
func (s *Server) setProvenance(w http.ResponseWriter, p model.Provenance) {
	h := w.Header()
	h.Set(headerSource, string(p.Source))
	h.Set(headerCommit, p.Commit)
	h.Set(headerDate, p.Date)
	h.Set(headerVersion, p.ScorecardVersion)
	h.Set(headerComplete, strconv.FormatBool(p.Complete))
}

// writeNotReady emits a 202 telling the client to retry while a scan runs.
func (s *Server) writeNotReady(w http.ResponseWriter, out *orchestrator.Outcome) {
	secs := int(out.RetryAfter.Round(time.Second) / time.Second)
	if secs < 1 {
		secs = 1
	}
	w.Header().Set(headerSource, string(model.SourceLive))
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	s.writeJSON(w, http.StatusAccepted, map[string]any{
		"status":              "scanning",
		"retry_after_seconds": secs,
		"message":             "a scan is in progress for this repository; retry shortly",
	})
}

// writeProduceError maps an orchestrator error to an HTTP status. A skipped
// repository (unreachable/blocked) is a 404; anything else is treated as an
// upstream failure. Framing describes the request/scan, never the repository's
// security posture.
func (s *Server) writeProduceError(w http.ResponseWriter, ref model.RepoRef, err error) {
	if errors.Is(err, scan.ErrSkipped) {
		s.writeError(w, http.StatusNotFound, "no result available",
			"the repository could not be scanned (unreachable, blocked, or not accessible with the configured token)")
		return
	}
	s.logger.Error("producing result failed", "ref", ref.Name(), "error", err)
	s.writeError(w, http.StatusBadGateway, "scan failed",
		"the on-demand scan could not be completed; please retry later")
}

// writeError writes a JSON error body with the given status.
func (s *Server) writeError(w http.ResponseWriter, status int, msg, detail string) {
	s.writeJSON(w, status, map[string]string{"error": msg, "detail": detail})
}

// writeJSON encodes v as a JSON response with the given status.
func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.logger.Warn("encoding JSON response", "error", err)
	}
}
