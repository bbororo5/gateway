package ratelimit

import (
	"fmt"
	"sync"
	"testing"

	"golang.org/x/time/rate"
)

// GlobalLockLimiter 는 비교군(Control Group)을 위한 단일 락 구현체입니다.
// "왜 샤딩이 필요한가?"를 증명하기 위해 의도적으로 성능이 낮은 방식을 구현합니다.
type GlobalLockLimiter struct {
	mu       sync.Mutex
	visitors map[string]*rate.Limiter
	r        rate.Limit
	b        int
}

func NewGlobal(r rate.Limit, b int) *GlobalLockLimiter {
	return &GlobalLockLimiter{
		visitors: make(map[string]*rate.Limiter),
		r:        r,
		b:        b,
	}
}

func (l *GlobalLockLimiter) getVisitor(ip string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()

	limiter, exists := l.visitors[ip]
	if !exists {
		limiter = rate.NewLimiter(l.r, l.b)
		l.visitors[ip] = limiter
	}

	return limiter
}

// Benchmark_Lock_Contention 에서는 글로벌 락 vs 샤딩 락의 성능을 측정합니다.
// RunParallel을 사용하여 높은 동시성(High Contention) 상황을 연출합니다.
func Benchmark_Lock_Contention(b *testing.B) {
	// 설정: 100 RPS
	limit := rate.Limit(100)
	burst := 10

	// 1. Global Lock (단일 Mutex) 성능 측정
	b.Run("GlobalLock", func(b *testing.B) {
		l := NewGlobal(limit, burst)
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				// 1000개의 IP 풀 안에서 랜덤 접근하여 맵(Map) 락 경합 유발
				ip := fmt.Sprintf("192.168.0.%d", i%1000)
				l.getVisitor(ip).Allow()
				i++
			}
		})
	})

	// 2. Sharded Lock (분산 Mutex) 성능 측정
	b.Run("ShardedLock", func(b *testing.B) {
		l := New(float64(limit), burst)
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				ip := fmt.Sprintf("192.168.0.%d", i%1000)
				// 우리가 구현한 내부 메서드 getLimiter 사용
				l.getLimiter(ip).Allow()
				i++
			}
		})
	})
}
