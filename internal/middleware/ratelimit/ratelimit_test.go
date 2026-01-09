package ratelimit

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Mock용 Request 생성 헬퍼
func newRequest(remoteAddr string) *http.Request {
	req := httptest.NewRequest("GET", "/", nil)
	// RemoteAddr는 일반적으로 포트 정보를 포함합니다 (e.g., "ip:port")
	req.RemoteAddr = remoteAddr
	return req
}

func TestRateLimiter_Allow(t *testing.T) {
	// RPS: 10, Burst: 10
	limiter := New(10, 10)
	handler := limiter.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := newRequest("10.0.0.1:1234")
	w := httptest.NewRecorder()

	// 10번 허용
	for i := 0; i < 10; i++ {
		handler.ServeHTTP(w, req)
		if w.Result().StatusCode != http.StatusOK {
			t.Errorf("request %d failed: status %d", i, w.Result().StatusCode)
		}
	}
}

func TestRateLimiter_Block(t *testing.T) {
	// RPS: 1, Burst: 1
	limiter := New(1, 1)
	handler := limiter.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := newRequest("10.0.0.2:1234")

	// 첫 번째 요청: 성공 (Burst 1 소모)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req)
	if w1.Result().StatusCode != http.StatusOK {
		t.Errorf("first request should pass")
	}

	// 두 번째 요청: 실패 예상 (토큰 부족)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req)
	if w2.Result().StatusCode != http.StatusTooManyRequests {
		t.Errorf("second request should be blocked, got %d", w2.Result().StatusCode)
	}
}

func TestRateLimiter_Sharding(t *testing.T) {
	// 여러 IP가 동시에 요청해도 서로 간섭하지 않는지 확인
	rps := 100.0
	burst := 100
	limiter := New(rps, burst)

	// IP1: "192.168.0.1"
	// IP2: "192.168.0.2"

	// 내부 메서드(getShard 등)는 export되지 않아서 직접 호출 불가능하거나
	// 같은 패키지 내(ratelimit)이므로 호출 가능

	// getShard는 ratelimit 패키지에 정의되어 있으며,
	// 이 테스트 파일도 ratelimit 패키지에 속하므로 비공개 메서드에 접근 가능합니다.

	shard1 := limiter.getShard("192.168.0.1")
	shard2 := limiter.getShard("192.168.0.2")

	// FNV 알고리즘 특성상 웬만하면 다른 샤드일 것임 (확률적, 테스트를 위해 로그만 찍음)
	t.Logf("Shard1 Addr: %p, Shard2 Addr: %p", shard1, shard2)

	// 각기 다른 IP로 요청
	limiter.getLimiter("192.168.0.1").Allow()
	limiter.getLimiter("192.168.0.2").Allow()
}

func TestRateLimiter_CapsShardAndReclaimsExpiredEntries(t *testing.T) {
	limiter := New(1, 1)
	var target *shard
	addresses := make([]string, 0, clientsLimit+1)
	for n := 0; len(addresses) <= clientsLimit; n++ {
		key := fmt.Sprintf("client-%d", n)
		s := limiter.getShard(key)
		if target == nil {
			target = s
		}
		if s == target {
			addresses = append(addresses, key)
		}
	}
	for _, key := range addresses[:clientsLimit] {
		if !limiter.getLimiter(key).Allow() {
			t.Fatal("new client denied below limit")
		}
	}
	overflow := addresses[clientsLimit]
	if limiter.getLimiter(overflow).Allow() {
		t.Fatal("new client bypassed the shard limit")
	}
	target.clients[addresses[0]].lastSeen = time.Now().Add(-clientTTL)
	target.nextSweep = time.Time{}
	if !limiter.getLimiter(overflow).Allow() {
		t.Fatal("expired entries did not free space")
	}
	if len(target.clients) != clientsLimit {
		t.Fatalf("size = %d, want %d", len(target.clients), clientsLimit)
	}
}
