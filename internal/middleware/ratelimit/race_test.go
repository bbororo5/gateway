package ratelimit

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// TestRateLimiter_Concurrency_Race 는 수많은 고루틴이 동시에 요청을 보낼 때
// 데이터 레이스가 발생하지 않는지 검증합니다. (반드시 -race 플래그와 함께 실행)
func TestRateLimiter_Concurrency_Race(t *testing.T) {
	// 넉넉한 RPS
	limiter := New(10000, 10000)
	handler := limiter.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	var wg sync.WaitGroup
	concurrentCount := 100 // 100개의 동시 고루틴

	// 각 고루틴마다 서로 다른 IP 또는 같은 IP를 섞어서 요청
	for i := 0; i < concurrentCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// IP를 10개로 제한하여 경합 유발 (100개 요청이 10개 IP 공유 -> 락 경합 발생)
			ip := fmt.Sprintf("192.168.0.%d", idx%10)
			req := newRequest(ip)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)
		}(i)
	}

	wg.Wait()
}
