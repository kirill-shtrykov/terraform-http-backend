package server

import (
	"fmt"
	"net/http"
	"time"
)

const (
	defaultTimeout      = 5 * time.Second
	defaultReadTimeout  = 10 * time.Second
	defaultWriteTimeout = 15 * time.Second
	defaultIdleTimeout  = 60 * time.Second
)

type Server struct {
	Addr string
	Mux  *http.ServeMux
}

func (s *Server) RegisterHandler(pattern string, handler http.Handler) {
	s.Mux.Handle(pattern, handler)
}

func (s *Server) Run() error {
	srv := &http.Server{
		Addr:              s.Addr,
		Handler:           s.Mux,
		ReadTimeout:       defaultReadTimeout,
		WriteTimeout:      defaultWriteTimeout,
		IdleTimeout:       defaultIdleTimeout,
		ReadHeaderTimeout: defaultTimeout,
	}

	if err := srv.ListenAndServe(); err != nil {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}

	return nil
}

func New(addr string) *Server {
	mux := http.NewServeMux()

	return &Server{Addr: addr, Mux: mux}
}
