package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/jeonseon-ung/gateway/internal/config"
	"github.com/jeonseon-ung/gateway/internal/engine"
	"github.com/jeonseon-ung/gateway/internal/middleware" // 패키지 추가
)

type Server struct {
	cfg        config.ServerConfig
	httpServer *http.Server
}

func New(fullCfg *config.Config, eng *engine.Engine) *Server {
	finalHandler := middleware.BuildChain(fullCfg.Middleware, eng)

	return &Server{
		cfg: fullCfg.Server,
		httpServer: &http.Server{
			Addr:         fmt.Sprintf(":%d", fullCfg.Server.Port),
			Handler:      finalHandler,
			ReadTimeout:  fullCfg.Server.ReadTimeout,
			WriteTimeout: fullCfg.Server.WriteTimeout,
		},
	}
}

func (s *Server) Start() error {
	slog.Info("HTTP Server listening", "port", s.cfg.Port)

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("http server error: %w", err)
	}
	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	slog.Info("HTTP Server stopping...")
	return s.httpServer.Shutdown(ctx)
}
