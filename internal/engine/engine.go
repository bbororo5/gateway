package engine

import (
	"fmt"
	"net/http"
	"path"
	"strings"
	"sync"
)

type Engine struct {
	mu            sync.RWMutex
	loadBalancers map[string]*LoadBalancer
}

func New() *Engine {
	return &Engine{loadBalancers: make(map[string]*LoadBalancer)}
}

func (e *Engine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !unambiguousPath(r) {
		http.Error(w, "Invalid request path", http.StatusBadRequest)
		return
	}
	e.mu.RLock()
	var matched *LoadBalancer
	longest := 0
	for prefix, lb := range e.loadBalancers {
		if len(prefix) > longest && (prefix == "/" || r.URL.Path == prefix || strings.HasPrefix(r.URL.Path, prefix+"/")) {
			matched = lb
			longest = len(prefix)
		}
	}
	e.mu.RUnlock()

	if matched == nil {
		http.Error(w, "Service Not Found", http.StatusNotFound)
		return
	}
	matched.ServeHTTP(w, r)
}

func (e *Engine) Ready() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, lb := range e.loadBalancers {
		lb.mu.RLock()
		available := len(lb.targets) > 0
		lb.mu.RUnlock()
		if available {
			return true
		}
	}
	return false
}

func (e *Engine) UpdateServiceEndpoints(prefix string, endpoints []string, strategy BalancerStrategy) error {
	if !validPrefix(prefix) {
		return fmt.Errorf("invalid route prefix %q", prefix)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	lb := e.loadBalancers[prefix]
	if lb == nil {
		if strategy == nil {
			strategy = &RoundRobinStrategy{}
		}
		lb = NewLoadBalancer(strategy)
	}
	if err := lb.UpdateTargets(endpoints); err != nil {
		return fmt.Errorf("update route %q: %w", prefix, err)
	}
	e.loadBalancers[prefix] = lb
	return nil
}

func (e *Engine) DeleteService(prefix string) {
	e.mu.Lock()
	delete(e.loadBalancers, prefix)
	e.mu.Unlock()
}

func validPrefix(prefix string) bool {
	return strings.HasPrefix(prefix, "/") && path.Clean(prefix) == prefix && !strings.ContainsAny(prefix, "?#%")
}

func unambiguousPath(r *http.Request) bool {
	value := r.URL.Path
	escaped := strings.ToLower(r.URL.EscapedPath())
	if !strings.HasPrefix(value, "/") || strings.Contains(escaped, "%2f") || strings.ContainsAny(value, "\\%") {
		return false
	}
	return value == "/" || strings.TrimSuffix(value, "/") == path.Clean(value)
}
