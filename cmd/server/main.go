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
	checkOnly := flag.Bool("check-config", false, "validate configuration and routes without listening")
	flag.Parse()
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, *configPath, *checkOnly); err != nil {
		slog.Error("Gateway exited with an error", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, configPath string, checkOnly bool) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	eng := engine.New()
	defer eng.CloseIdleConnections()
	watcher, err := configuredRouteFile(eng, cfg, configPath)
	if err != nil {
		return err
	}
	if checkOnly {
		slog.Info("Gateway configuration is valid")
		return nil
	}

	watchCtx, cancelWatch := context.WithCancel(ctx)
	defer cancelWatch()
	watchDone := make(chan error, 1)
	if watcher != nil {
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

func configuredRouteFile(eng *engine.Engine, cfg *config.Config, configPath string) (*syncer.RouteFile, error) {
	if cfg.Discovery.Provider != "file" {
		return nil, nil
	}
	path := cfg.Discovery.RouteFile
	if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(configPath), path)
	}
	watcher := syncer.NewRouteFile(eng, path)
	if err := watcher.Reload(); err != nil {
		return nil, fmt.Errorf("load initial route table: %w", err)
	}
	return watcher, nil
}
