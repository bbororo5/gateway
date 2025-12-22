package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jeonseon-ung/gateway/internal/config"
	"github.com/jeonseon-ung/gateway/internal/discovery"
	"github.com/jeonseon-ung/gateway/internal/discovery/mock"
	"github.com/jeonseon-ung/gateway/internal/engine"
	"github.com/jeonseon-ung/gateway/internal/server"
	"github.com/jeonseon-ung/gateway/internal/syncer"
)

func main() {
	// 1. 기본 Logger 설정 (JSON 포맷)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// 2. 설정 로드
	cfg, err := config.Load("config.yaml")
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	slog.Info("Configuration loaded", "port", cfg.Server.Port)

	// 3. 엔진 초기화 (라우팅 및 로드밸런싱 코어)
	eng := engine.New()

	// 4. 서비스 디스커버리 Provider 설정
	var provider discovery.Provider

	switch cfg.Discovery.Provider {
	case "mock":
		provider = mock.New()
		slog.Info("Using Mock Discovery Provider")
	default:
		// 현재는 mock만 구현되어 있으므로 기본값 처리 또는 에러
		slog.Warn("Unknown discovery provider, falling back to mock", "provider", cfg.Discovery.Provider)
		provider = mock.New()
	}

	// 5. Syncer 초기화 및 실행 (Discovery -> Engine 동기화)
	// 앱이 종료될 때 Syncer도 종료되도록 Context 사용
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sync := syncer.New(eng, provider)
	go func() {
		// Syncer는 백그라운드에서 이벤트를 감지하여 엔진을 업데이트합니다.
		if err := sync.Start(ctx); err != nil {
			slog.Error("Syncer failed", "error", err)
			// 치명적 오류 처리 필요 시 여기서 os.Exit 호출 가능
		}
	}()

	// 6. HTTP 서버 초기화
	// Config와 Engine을 주입받아 미들웨어 체인을 구성합니다.
	srv := server.New(cfg, eng)

	// 7. 서버 실행 (Non-blocking)
	go func() {
		if err := srv.Start(); err != nil {
			slog.Error("Server failed to start", "error", err)
			os.Exit(1)
		}
	}()

	// 8. Graceful Shutdown 대기
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// 시그널이 올 때까지 대기
	sig := <-quit
	slog.Info("Shutting down server...", "signal", sig)

	// 정리 작업을 위한 타임아웃 컨텍스트 생성
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer shutdownCancel()

	// HTTP 서버 종료
	if err := srv.Stop(shutdownCtx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
	}

	// Provider 정리 (연결 종료 등)
	if err := provider.Close(); err != nil {
		slog.Error("Failed to close discovery provider", "error", err)
	}

	slog.Info("Server exited properly")
}
