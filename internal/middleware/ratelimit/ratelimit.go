package ratelimit

import (
	"hash/fnv"
	"net/http"
	"sync"
	"time"

	"github.com/jeonseon-ung/gateway/pkg/utils"
	"golang.org/x/time/rate"
)

const (
	shardCount   = 256
	clientsLimit = 256
	clientTTL    = 3 * time.Minute
	sweepPeriod  = 30 * time.Second
)

type client struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type shard struct {
	mu        sync.Mutex
	clients   map[string]*client
	nextSweep time.Time
}

type IPRateLimiter struct {
	shards []*shard
	rps    rate.Limit
	burst  int
	deny   *rate.Limiter
}

func New(rps float64, burst int) *IPRateLimiter {
	i := &IPRateLimiter{
		shards: make([]*shard, shardCount),
		rps:    rate.Limit(rps),
		burst:  burst,
		deny:   rate.NewLimiter(0, 0),
	}
	for idx := range i.shards {
		i.shards[idx] = &shard{clients: make(map[string]*client)}
	}
	return i
}

func (i *IPRateLimiter) getShard(ip string) *shard {
	h := fnv.New32a()
	h.Write([]byte(ip))
	return i.shards[h.Sum32()%shardCount]
}

func (i *IPRateLimiter) getLimiter(ip string) *rate.Limiter {
	s := i.getShard(ip)
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	if !now.Before(s.nextSweep) {
		s.prune(now)
	}
	if c, ok := s.clients[ip]; ok {
		c.lastSeen = now
		return c.limiter
	}
	if len(s.clients) >= clientsLimit {
		return i.deny
	}
	limiter := rate.NewLimiter(i.rps, i.burst)
	s.clients[ip] = &client{limiter: limiter, lastSeen: now}
	return limiter
}

func (s *shard) prune(now time.Time) {
	for ip, c := range s.clients {
		if now.Sub(c.lastSeen) >= clientTTL {
			delete(s.clients, ip)
		}
	}
	s.nextSweep = now.Add(sweepPeriod)
}

func (i *IPRateLimiter) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !i.getLimiter(utils.GetClientIP(r)).Allow() {
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
