package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jeonseon-ung/gateway/internal/config"
	"github.com/jeonseon-ung/gateway/internal/engine"
	"github.com/jeonseon-ung/gateway/internal/middleware"
)

// Benchmark_Integrated_System 은 실제 게이트웨이 파이프라인(미들웨어+엔진+프록시)의 통합 성능을 측정합니다.
// 이 테스트는 로컬 Loopback TCP 연결을 포함하므로, 실제 운영 환경과 매우 유사한 RPS 수치를 제공합니다.
func Benchmark_Integrated_System(b *testing.B) {
	// 1. Mock Backend Server (우리가 프록시할 대상)
	// 최대한 가볍게 응답만 줌 (Hello World)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend-Reply", "Metrics")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer backend.Close()

	// 2. 엔진 설정
	eng := engine.New()
	// 모든 요청을 위에서 만든 Mock Backend로 라우팅
	// 경로: /api/bench -> backend.URL
	eng.UpdateServiceEndpoints("/api/bench", []string{backend.URL}, nil)

	// 3. 전체 설정 (RateLimit 등 포함)
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
		},
		Middleware: config.MiddlewareConfig{
			// 로깅은 벤치마크에서 I/O 병목이 될 수 있으므로,
			// 실제 프로덕션 퍼포먼스(CPU Bound)를 집중 측정하기 위해
			// 로깅은 켜두되 출력은 Discard로 보냄.
			GlobalEnabled: []string{"ratelimit", "logging"},
			RateLimit: config.RateLimitConfig{
				Enabled:           true,
				RequestsPerSecond: 1000000, // 벤치마크 방해 안 받게 넉넉히
				Burst:             100000,
			},
		},
	}

	// 로그 출력을 /dev/null로 돌려서 콘솔 출력 노이즈 제거
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))

	// 4. 핸들러 체인 구성
	handler := middleware.BuildChain(cfg.Middleware, eng)

	// 5. 벤치마크 실행
	// 미리 요청 객체 템플릿 생성
	baseReq, _ := http.NewRequest("GET", "/api/bench/resource", nil)

	b.ReportAllocs()
	b.ResetTimer()

	// 병렬 부하 테스트 (실제 서버는 멀티코어에서 동작하므로 RunParallel이 적합)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// 각 반복마다 Recorder는 새로 만들어야 함 (Writer니까)
			w := httptest.NewRecorder()

			// Request는 읽기 전용으로 쓰이지만, 헤더 조작 등이 있을 수 있으니
			// 안전하게 clone 하거나 그냥 사용 (http.Request는 포인터지만 표준 라이브러리 핸들러들은 보통 Safe하게 다룸)
			// 여기서는 Shallow Copy로 충분
			req := baseReq.Clone(baseReq.Context())

			// 실제 ServeHTTP 로직 수행 (Middleware -> RateLimit -> Engine -> ReverseProxy -> MockBackend)
			handler.ServeHTTP(w, req)
		}
	})
}
