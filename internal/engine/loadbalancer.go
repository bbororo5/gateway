package engine

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/jeonseon-ung/gateway/pkg/utils"
)

type LoadBalancer struct {
	mu        sync.RWMutex
	targets   []*upstream
	strategy  BalancerStrategy
	transport http.RoundTripper
	policy    CircuitPolicy
}

type upstream struct {
	url     string
	proxy   *httputil.ReverseProxy
	circuit circuit
}

type leaseKey struct{}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func NewLoadBalancer(strategy BalancerStrategy) *LoadBalancer {
	return &LoadBalancer{strategy: strategy, transport: http.DefaultTransport}
}

func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	lb.mu.RLock()
	targets := lb.targets
	lb.mu.RUnlock()

	if len(targets) == 0 {
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}
	idx := lb.strategy.Next(len(targets), utils.GetClientIP(r))
	if idx < 0 || idx >= len(targets) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		slog.Error("Strategy returned invalid index", "index", idx, "targets_count", len(targets))
		return
	}

	for offset := range targets {
		target := targets[(idx+offset)%len(targets)]
		generation, allowed := target.circuit.admit(time.Now())
		if !allowed {
			continue
		}
		if lb.policy.Threshold > 0 {
			r = r.WithContext(context.WithValue(r.Context(), leaseKey{}, generation))
		}
		target.proxy.ServeHTTP(w, r)
		return
	}
	http.Error(w, "No healthy upstream available", http.StatusServiceUnavailable)
}

func (lb *LoadBalancer) UpdateTargets(targetURLs []string) error {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	existing := make(map[string]*upstream, len(lb.targets))
	for _, target := range lb.targets {
		existing[target.url] = target
	}

	targets := make([]*upstream, 0, len(targetURLs))
	seen := make(map[string]bool, len(targetURLs))
	for _, address := range targetURLs {
		if seen[address] {
			return fmt.Errorf("duplicate upstream %q", address)
		}
		seen[address] = true
		target := existing[address]
		if target == nil {
			var err error
			target, err = lb.createUpstream(address)
			if err != nil {
				return err
			}
		}
		targets = append(targets, target)
	}
	lb.targets = targets
	return nil
}

func (lb *LoadBalancer) createUpstream(address string) (*upstream, error) {
	parsed, err := url.Parse(address)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.Fragment != "" {
		return nil, fmt.Errorf("upstream must be an absolute HTTP(S) URL without credentials or fragment")
	}
	target := &upstream{url: address, circuit: circuit{policy: lb.policy}}
	target.proxy = &httputil.ReverseProxy{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			resp, err := lb.transport.RoundTrip(r)
			if generation, ok := r.Context().Value(leaseKey{}).(uint64); ok {
				result := success
				switch {
				case err != nil && r.Context().Err() != nil:
					result = neutral
				case err != nil || resp.StatusCode >= 500:
					result = failure
				}
				target.circuit.record(generation, result, time.Now())
			}
			return resp, err
		}),
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(parsed)
			pr.Out.Host = pr.In.Host
			pr.SetXForwarded()
			pr.Out.Header.Set("X-Forwarded-For", utils.GetClientIP(pr.In))
			pr.Out.Header.Set("X-Forwarded-Proto", utils.GetClientScheme(pr.In))
			pr.Out.Header.Del("X-Real-IP")
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Warn("Upstream request failed", "upstream", parsed.Host, "error", err)
			if timeout, ok := err.(interface{ Timeout() bool }); ok && timeout.Timeout() {
				http.Error(w, "Gateway Timeout", http.StatusGatewayTimeout)
				return
			}
			http.Error(w, "Bad Gateway", http.StatusBadGateway)
		},
	}
	return target, nil
}
