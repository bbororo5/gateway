package engine

import (
	"log/slog"
	"net/http"
	"strings"
	"sync"
)

type Engine struct {
	mu            sync.RWMutex
	loadBalancers map[string]*LoadBalancer
}

func New() *Engine {
	return &Engine{
		loadBalancers: make(map[string]*LoadBalancer),
	}
}

func (e *Engine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var matchedLB *LoadBalancer
	matchedLen := 0

	for prefix, lb := range e.loadBalancers {
		if strings.HasPrefix(r.URL.Path, prefix) {
			if len(prefix) > matchedLen {
				matchedLB = lb
				matchedLen = len(prefix)
			}
		}
	}

	if matchedLB != nil {
		matchedLB.ServeHTTP(w, r)
		return
	}

	http.Error(w, "Service Not Found", http.StatusNotFound)
}

func (e *Engine) UpdateServiceEndpoints(pathPrefix string, endpoints []string, strategy BalancerStrategy) error {
	e.mu.RLock()
	lb, exists := e.loadBalancers[pathPrefix]
	e.mu.RUnlock()

	if exists {
		lb.UpdateTargets(endpoints)
		slog.Info("Updated existing service", "service", pathPrefix, "endpoints_count", len(endpoints))
		return nil
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	lb, exists = e.loadBalancers[pathPrefix]
	if !exists {
		useStrategy := strategy
		if useStrategy == nil {
			useStrategy = &RoundRobinStrategy{}
		}

		lb = NewLoadBalancer(useStrategy)
		lb.UpdateTargets(endpoints)

		e.loadBalancers[pathPrefix] = lb
		slog.Info("Created new service", "service", pathPrefix, "endpoints_count", len(endpoints))
	} else {
		lb.UpdateTargets(endpoints)
	}

	return nil
}

func (e *Engine) DeleteService(pathPrefix string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.loadBalancers[pathPrefix]; exists {
		delete(e.loadBalancers, pathPrefix)
		slog.Info("Deleted service", "service", pathPrefix)
	}
}
