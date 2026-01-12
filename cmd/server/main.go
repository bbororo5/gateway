package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/jeonseon-ung/gateway/internal/config"
	"github.com/jeonseon-ung/gateway/internal/discovery/mock"
	"github.com/jeonseon-ung/gateway/internal/engine"
	"github.com/jeonseon-ung/gateway/internal/server"
	"github.com/jeonseon-ung/gateway/internal/syncer"
)

func main() {
	configPath := flag.String("config", "config.yaml", "gateway configuration file")
	flag.Parse()
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, *configPath); err != nil {
		slog.Error("Gateway exited with an error", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	eng := engine.New()
	watchCtx, cancelWatch := context.WithCancel(ctx)
	defer cancelWatch()
	watchDone := make(chan error, 1)
	if cfg.Discovery.Provider == "file" {
		path := cfg.Discovery.RouteFile
		if !filepath.IsAbs(path) {
			path = filepath.Join(filepath.Dir(configPath), path)
		}
		watcher := syncer.NewRouteFile(eng, path)
		if err := watcher.Reload(); err != nil {
			return fmt.Errorf("load initial route table: %w", err)
		}
		go func() {
			watcher.Watch(watchCtx, cfg.Discovery.WatchInterval)
			watchDone <- nil
		}()
	} else {
		provider := mock.New()
		defer provider.Close()
		go func() { watchDone <- syncer.New(eng, provider).Start(watchCtx) }()
	}

	srv := server.New(cfg, eng)
	serverDone := make(chan error, 1)
	go func() { serverDone <- srv.Start() }()

	watcherStopped := false
	select {
	case <-ctx.Done():
	case err = <-serverDone:
	case err = <-watchDone:
		watcherStopped = true
	}

	cancelWatch()
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancelShutdown()
	err = errors.Join(err, srv.Stop(shutdownCtx))
	if !watcherStopped {
		err = errors.Join(err, <-watchDone)
	}
	return err
}
