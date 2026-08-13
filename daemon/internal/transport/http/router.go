package http

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/shellsync/daemon/internal/auth"
	"github.com/shellsync/daemon/internal/service"
)

// Deps wires the handler dependencies.
type Deps struct {
	Version   string
	StartedAt time.Time
	Svc       *service.Services
	Auth      *auth.Verifier
	WS        http.Handler // /ws upgrade handler (from the ws package); nil to disable
	Shutdown  context.CancelFunc
}

// Server is the HTTP/REST server.
type Server struct {
	Deps
}

// New creates a Server.
func New(d Deps) *Server {
	if d.StartedAt.IsZero() {
		d.StartedAt = time.Now()
	}
	return &Server{Deps: d}
}

// Handler builds the chi router with all routes.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(recoverMiddleware)
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// public routes
	r.Get("/health", s.health)
	if s.WS != nil {
		r.Get("/ws", s.WS.ServeHTTP)
	}

	r.Route("/api", func(r chi.Router) {
		// pairing is public (it is how a client obtains a token)
		r.Route("/pair", func(r chi.Router) {
			r.Post("/init", s.pairInit)
			r.Post("/verify", s.pairVerify)
		})

		// authenticated routes
		r.Group(func(r chi.Router) {
			r.Use(authMiddleware(s.Auth))

			r.Route("/tasks", func(r chi.Router) {
				r.Get("/", s.listTasks)
				r.Post("/", s.createTask)
				r.Get("/{id}", s.getTask)
				r.Patch("/{id}", s.updateTask)
				r.Delete("/{id}", s.deleteTask)
			})

			r.Route("/todos", func(r chi.Router) {
				r.Get("/", s.listTodos)
				r.Post("/", s.createTodo)
				r.Get("/{id}", s.getTodo)
				r.Patch("/{id}", s.updateTodo)
				r.Delete("/{id}", s.deleteTodo)
			})

			r.Route("/terminals", func(r chi.Router) {
				r.Get("/", s.listTerminals)
				r.Post("/", s.createTerminal)
				r.Get("/{id}", s.getTerminal)
				r.Patch("/{id}", s.updateTerminal)
				r.Post("/{id}/resize", s.resizeTerminal)
				r.Post("/{id}/restart", s.restartTerminal)
				r.Delete("/{id}", s.deleteTerminal)
				r.Get("/{id}/logs", s.terminalLogs)
				r.Get("/{id}/logs/tail", s.terminalLogsTail)
			})

			r.Route("/devices", func(r chi.Router) {
				r.Get("/", s.listDevices)
				r.Delete("/{id}", s.deleteDevice)
			})

			r.Get("/settings", s.getSettings)
			r.Patch("/settings", s.patchSettings)

			r.Get("/shells", s.listShells)
			r.Post("/daemon/shutdown", s.shutdown)
		})
	})

	return r
}

// Serve binds to :port (0 = dynamic) and serves the router. It returns the
// actual port and a shutdown function.
func Serve(handler http.Handler, port int) (int, func(context.Context) error, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return 0, nil, fmt.Errorf("listen :%d: %w", port, err)
	}
	actual := ln.Addr().(*net.TCPAddr).Port
	srv := &http.Server{Handler: handler}
	go func() { _ = srv.Serve(ln) }()
	return actual, func(ctx context.Context) error { return srv.Shutdown(ctx) }, nil
}
