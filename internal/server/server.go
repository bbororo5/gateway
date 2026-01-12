package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/jeonseon-ung/gateway/internal/config"
	"github.com/jeonseon-ung/gateway/internal/engine"
	"github.com/jeonseon-ung/gateway/internal/httpstats"
	"github.com/jeonseon-ung/gateway/internal/middleware"
	"github.com/jeonseon-ung/gateway/pkg/utils"
)

type Server struct {
	httpServer *http.Server
	admin      *http.Server
	draining   atomic.Bool
}

func New(cfg *config.Config, eng *engine.Engine) *Server {
	srv := cfg.Server
	prefixes, _ := srv.ProxyPrefixes() // Load validates trusted proxy prefixes.
	handler := utils.TrustClientIP(prefixes, middleware.BuildChain(cfg.Middleware, eng))
	s := &Server{
		httpServer: &http.Server{
			Addr:              fmt.Sprintf(":%d", srv.Port),
			Handler:           handler,
			ReadTimeout:       srv.ReadTimeout,
			ReadHeaderTimeout: srv.ReadHeaderTimeout,
			WriteTimeout:      srv.WriteTimeout,
			IdleTimeout:       srv.IdleTimeout,
		},
	}
	if srv.AdminAddress != "" {
		metrics := &httpstats.Metrics{}
		s.httpServer.Handler = metrics.Instrument(handler)
		s.admin = &http.Server{
			Addr:              srv.AdminAddress,
			Handler:           s.adminHandler(eng, metrics),
			ReadHeaderTimeout: 3 * time.Second,
			WriteTimeout:      5 * time.Second,
			IdleTimeout:       15 * time.Second,
		}
	}
	return s
}

func (s *Server) adminHandler(eng *engine.Engine, metrics *httpstats.Metrics) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if s.draining.Load() || !eng.Ready() {
			http.Error(w, "Gateway not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.Handle("GET /metrics", metrics)
	return mux
}

func (s *Server) Start() error {
	public, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("listen on gateway address: %w", err)
	}
	defer public.Close()

	if s.admin == nil {
		return serve(s.httpServer, public)
	}
	admin, err := net.Listen("tcp", s.admin.Addr)
	if err != nil {
		return fmt.Errorf("listen on admin address: %w", err)
	}
	defer admin.Close()

	results := make(chan error, 2)
	go func() { results <- serve(s.httpServer, public) }()
	go func() { results <- serve(s.admin, admin) }()
	if err := <-results; err != nil {
		return err
	}
	return <-results
}

func serve(server *http.Server, listener net.Listener) error {
	slog.Info("HTTP server listening", "address", listener.Addr().String())
	if err := server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	s.draining.Store(true)
	stop := func(server *http.Server) error {
		if err := server.Shutdown(ctx); err != nil {
			return errors.Join(err, server.Close())
		}
		return nil
	}
	err := stop(s.httpServer)
	if s.admin != nil {
		err = errors.Join(err, stop(s.admin))
	}
	return err
}
