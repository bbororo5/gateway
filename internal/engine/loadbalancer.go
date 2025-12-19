package engine

import (
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
)

type LoadBalancer struct {
	mu       sync.RWMutex
	targets  []*httputil.ReverseProxy
	strategy BalancerStrategy
}

func NewLoadBalancer(strategy BalancerStrategy) *LoadBalancer {
	return &LoadBalancer{
		strategy: strategy,
		targets:  make([]*httputil.ReverseProxy, 0),
	}
}

func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	lb.mu.RLock()
	targets := lb.targets
	lb.mu.RUnlock()

	if len(targets) == 0 {
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}

	remoteIP, _, _ := net.SplitHostPort(r.RemoteAddr)

	idx := lb.strategy.Next(len(targets), remoteIP)

	if idx < 0 || idx >= len(targets) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		log.Printf("[LoadBalancer] Strategy returned invalid index: %d, targets: %d", idx, len(targets))
		return
	}

	target := targets[idx]
	target.ServeHTTP(w, r)
}

func (lb *LoadBalancer) UpdateTargets(targetURLs []string) {
	newTargets := make([]*httputil.ReverseProxy, 0, len(targetURLs))

	for _, targetURL := range targetURLs {
		proxy, err := lb.createProxy(targetURL)
		if err != nil {
			log.Printf("[LoadBalancer] Skipping invalid URL %s: %v", targetURL, err)
			continue
		}
		newTargets = append(newTargets, proxy)
	}

	lb.mu.Lock()
	lb.targets = newTargets
	lb.mu.Unlock()
}

func (lb *LoadBalancer) createProxy(targetURL string) (*httputil.ReverseProxy, error) {
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(parsedURL)

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("[Proxy Error] Target: %s, Error: %v", targetURL, err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	return proxy, nil
}