package ratelimit

import (
	"hash/fnv"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jeonseon-ung/gateway/pkg/utils"
	"golang.org/x/time/rate"
)

const shardCount = 256 

type client struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type shard struct {
	mu      sync.RWMutex
	clients map[string]*client
}

type IPRateLimiter struct {
	shards []*shard
	rps      rate.Limit
	burst      int
}

func New(rps float64, burst int) *IPRateLimiter {
	i := &IPRateLimiter{
		shards: 	make([]*shard, shardCount),
		rps:     	rate.Limit(rps),
		burst:      burst,
	}

	for idx := range i.shards {
		i.shards[idx] = &shard{
			clients: make(map[string]*client),
		}
	}

	go i.cleanupLoop()

	return i
}

func (i *IPRateLimiter) getShard(ip string) *shard {
	h := fnv.New32a()
	h.Write([]byte(ip))
	idx := h.Sum32() % uint32(shardCount)
	return i.shards[idx]
}

func (i *IPRateLimiter) getLimiter(ip string) *rate.Limiter {
	shard := i.getShard(ip)

	shard.mu.Lock()
	defer shard.mu.Unlock()

	v, exists := shard.clients[ip]
	if !exists {
		limiter := rate.NewLimiter(i.rps, i.burst)
		shard.clients[ip] = &client{
			limiter:  limiter,
			lastSeen: time.Now(),
		}
		return limiter
	}

	v.lastSeen = time.Now()
	return v.limiter
}

func (i *IPRateLimiter) cleanupLoop() {
	for {
		time.Sleep(1 * time.Minute)

		for _, shard := range i.shards {
			shard.mu.Lock()
			for ip, v := range shard.clients {
				if time.Since(v.lastSeen) > 3*time.Minute {
					delete(shard.clients, ip)
				}
			}
			shard.mu.Unlock()
			
			// manner time
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func (i *IPRateLimiter) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := utils.GetClientIP(r)
		limiter := i.getLimiter(ip)

		if !limiter.Allow() {
			slog.Warn("Rate limit exceeded", "ip", ip, "path", r.URL.Path)
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}
