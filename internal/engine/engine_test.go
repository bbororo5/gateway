package engine

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEngine_Routing(t *testing.T) {
	eng := New()

	// 서비스 등록 (/api/v1 -> dummy targets)
	// 실제 타겟은 호출하지 않을 것이므로(경로 매칭만 테스트),
	// LoadBalancer가 내부적으로 ReverseProxy를 생성하더라도
	// ServeHTTP 호출 시 ReverseProxy가 실제 요청을 시도하다 에러가 날 수 있음.
	// 따라서 여기서는 "Service Not Found"가 안 뜨는지만 확인하거나,
	// Mock 서버를 띄워야 함.

	// Mock Backend Server
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("backend ok"))
	}))
	defer mockBackend.Close()

	err := eng.UpdateServiceEndpoints("/api/v1", []string{mockBackend.URL}, &RoundRobinStrategy{})
	if err != nil {
		t.Fatalf("failed to update service: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/users", nil)
	w := httptest.NewRecorder()

	eng.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", w.Code)
	}

	// 없는 경로 테스트
	req2 := httptest.NewRequest("GET", "/api/v2/users", nil)
	w2 := httptest.NewRecorder()

	eng.ServeHTTP(w2, req2)

	if w2.Code != http.StatusNotFound {
		t.Errorf("expected 404 Not Found, got %d", w2.Code)
	}
}
