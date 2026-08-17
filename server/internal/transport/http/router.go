// Package http wires the relay-server HTTP surface: /health, /metrics and
// the /ws WebSocket endpoint (implemented by the relay package).
package http

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/shellsync/relay-server/relay"
)

// Deps wires the router dependencies.
type Deps struct {
	Version string
	Hub     *relay.Hub
	Log     *slog.Logger
}

// Server is the relay HTTP server.
type Server struct {
	Deps
}

// New creates a Server.
func New(d Deps) *Server {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	return &Server{Deps: d}
}

// Handler builds the chi router.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "OPTIONS"},
		MaxAge:         300,
	}))
	r.Get("/health", s.Hub.HealthHandler(s.Version))
	r.Get("/metrics", s.Hub.MetricsHandler())
	r.Get("/ws", s.Hub.ServeWS)
	return r
}
