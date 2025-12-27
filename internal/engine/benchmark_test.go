package engine

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Benchmark_Engine_Routing 은 엔진의 라우팅 처리 성능을 측정합니다.
// 주로 RLock(읽기 락)의 효율성을 봅니다.
func Benchmark_Engine_Routing(b *testing.B) {
	eng := New()
	eng.UpdateServiceEndpoints("/api/bench", []string{"http://localhost:8081", "http://localhost:8082"}, nil)

	// Mock Request & Recorder (매번 생성하면 할당 비용이 드므로 재사용 고려,
	// 하지만 실제 서버 부하와 유사하게 loop 내 생성이 더 리얼할 수 있음.
	// 여기선 순수 로직 테스트를 위해 미리 생성된 객체 재사용 X -> 병렬 테스트라 복제 필요)

	// 기본 요청 객체 템플릿
	baseReq, _ := http.NewRequest("GET", "/api/bench/resource", nil)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// 각 고루틴별로 Request 복제 (Thread Safe를 위해)
			// 단순화를 위해 같은 포인터를 써도 읽기 전용이면 괜찮지만 safe하게
			w := httptest.NewRecorder()
			// ServeHTTP 내부에서 RLock을 걸고 라우팅 매칭 수행
			eng.ServeHTTP(w, baseReq)
		}
	})
}
