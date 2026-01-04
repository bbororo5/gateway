package engine

import (
	"fmt"
	"net/http"
	"time"
)

type Route struct {
	Prefix                string        `yaml:"prefix"`
	Upstreams             []string      `yaml:"upstreams"`
	Strategy              string        `yaml:"strategy"`
	ResponseHeaderTimeout time.Duration `yaml:"response_header_timeout"`
	Circuit               CircuitPolicy `yaml:"circuit"`
}

// ReplaceRoutes validates and builds the whole table before publishing it.
func (e *Engine) ReplaceRoutes(routes []Route) error {
	next := make(map[string]*LoadBalancer, len(routes))
	for _, route := range routes {
		if _, ok := next[route.Prefix]; ok {
			return fmt.Errorf("duplicate route prefix %q", route.Prefix)
		}
		lb, err := buildRoute(route)
		if err != nil {
			return fmt.Errorf("route %q: %w", route.Prefix, err)
		}
		next[route.Prefix] = lb
	}
	e.mu.Lock()
	e.loadBalancers = next
	e.mu.Unlock()
	return nil
}

func buildRoute(route Route) (*LoadBalancer, error) {
	if !validPrefix(route.Prefix) || len(route.Upstreams) == 0 {
		return nil, fmt.Errorf("a canonical path prefix and at least one upstream are required")
	}
	var strategy BalancerStrategy
	switch route.Strategy {
	case "", "round_robin":
		strategy = &RoundRobinStrategy{}
	case "ip_hash":
		strategy = &HashStrategy{}
	case "random":
		strategy = &RandomStrategy{}
	default:
		return nil, fmt.Errorf("unknown strategy %q", route.Strategy)
	}
	if route.ResponseHeaderTimeout < 0 {
		return nil, fmt.Errorf("response_header_timeout cannot be negative")
	}
	policy := route.Circuit
	if policy.Threshold < 0 || policy.Cooldown < 0 || (policy.Threshold == 0) != (policy.Cooldown == 0) {
		return nil, fmt.Errorf("failure_threshold and cooldown must both be positive when configured")
	}

	lb := NewLoadBalancer(strategy)
	lb.policy = policy
	// Configured routes have bounded waits for the upstream response headers.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.ResponseHeaderTimeout = route.ResponseHeaderTimeout
	if transport.ResponseHeaderTimeout == 0 {
		transport.ResponseHeaderTimeout = 5 * time.Second
	}
	lb.transport = transport
	if err := lb.UpdateTargets(route.Upstreams); err != nil {
		return nil, err
	}
	return lb, nil
}
