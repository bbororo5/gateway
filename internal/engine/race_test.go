package engine

import (
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestEngine_Concurrency_HotSwap 은 요청 처리(Read)와 라우팅 업데이트(Write)가
// 동시에 격렬하게 발생할 때 데이터 레이스가 없는지 검증합니다.
func TestEngine_Concurrency_HotSwap(t *testing.T) {
	eng := New()

	// 초기 서비스 등록
	eng.UpdateServiceEndpoints("/api/v1", []string{"http://localhost:8081"}, nil)

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// 1. Reader Goroutines (요청 처리)
	// 50개의 고루틴이 계속 라우팅 테이블을 조회
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/api/v1/resource", nil)
			w := httptest.NewRecorder()

			for {
				select {
				case <-stop:
					return
				default:
					// 실제 ReverseProxy를 타지 않게 하기 위해
					// 여기서는 ServeHTTP 대신 내부 로직을 흉내내거나
					// 또는 "Service Not Found" 여부만 체크 (Mock backend 없으므로 502/503 날 수 있음)
					// 핵심은 Panic이나 Race가 안 나는 것임.

					// RLock을 사용하는 ServeHTTP 호출
					eng.ServeHTTP(w, req)
				}
			}
		}()
	}

	// 2. Writer Goroutines (설정 변경)
	// 5개의 고루틴이 계속 라우팅 테이블을 갱신 (Hot Swap)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					// 타겟을 계속 바꿈
					targets := []string{
						fmt.Sprintf("http://localhost:808%d", idx),
						fmt.Sprintf("http://localhost:909%d", idx),
					}
					// WLock을 사용하는 Update 호출
					eng.UpdateServiceEndpoints("/api/v1", targets, nil)
					time.Sleep(1 * time.Millisecond) // 너무 빠르면 Write Lock 독점하므로 양보
				}
			}
		}(i)
	}

	// 1초간 혼돈의 카오스 실행
	time.Sleep(1 * time.Second)
	close(stop)
	wg.Wait()
}
