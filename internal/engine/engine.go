package engine

import (
	"log"
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

	for pathPrefix, lb := range e.loadBalancers {
		if strings.HasPrefix(r.URL.Path, pathPrefix) {
			lb.ServeHTTP(w, r)
			return
		}
	}

	http.Error(w, "Service Not Found", http.StatusNotFound)
}

func (e *Engine) UpdateServiceEndpoints(pathPrefix string, endpoints []string, strategy BalancerStrategy) error {
	e.mu.RLock()
	lb, exists := e.loadBalancers[pathPrefix]
	e.mu.RUnlock()

	if exists {
		lb.UpdateTargets(endpoints)
		log.Printf("[Engine] Updated existing service %s with %d endpoints", pathPrefix, len(endpoints))
		return nil
	}

	e.mu.Lock()

	lb, exists = e.loadBalancers[pathPrefix]
	if !exists {
		useStrategy := strategy
		if useStrategy == nil {
			useStrategy = &RoundRobinStrategy{}
		}
		lb = NewLoadBalancer(useStrategy)
		e.loadBalancers[pathPrefix] = lb
	}
	e.mu.Unlock()

	lb.UpdateTargets(endpoints)
	log.Printf("[Engine] Created & Updated service %s with %d endpoints", pathPrefix, len(endpoints))

	return nil
}