package engine

import (
	"fmt"
	"net/http"
	"slices"
	"time"
)

type Route struct {
	Prefix                string        `yaml:"prefix"`
	Upstreams             []string      `yaml:"upstreams"`
	Strategy              string        `yaml:"strategy"`
	ResponseHeaderTimeout time.Duration `yaml:"response_header_timeout"`
	Circuit               CircuitPolicy `yaml:"circuit"`
}

// ReplaceRoutes validates the whole snapshot and reuses unchanged routes.
func (e *Engine) ReplaceRoutes(routes []Route) error {
	e.updates.Lock()
	defer e.updates.Unlock()

	// Other writers are excluded, so the current table is read-only during preparation.
	next := make(map[string]routeEntry, len(routes))
	transports := make(map[time.Duration]*http.Transport)
	for _, route := range routes {
		if _, ok := next[route.Prefix]; ok {
			return fmt.Errorf("duplicate route prefix %q", route.Prefix)
		}
		if route.Strategy == "" {
			route.Strategy = "round_robin"
		}
		if route.ResponseHeaderTimeout == 0 {
			route.ResponseHeaderTimeout = 5 * time.Second
		}
		if route.ResponseHeaderTimeout < 0 {
			return fmt.Errorf("route %q: response_header_timeout cannot be negative", route.Prefix)
		}
		transport := transports[route.ResponseHeaderTimeout]
		if transport == nil {
			transport = e.transports[route.ResponseHeaderTimeout]
			if transport == nil {
				transport = newUpstreamTransport(route.ResponseHeaderTimeout)
			}
			transports[route.ResponseHeaderTimeout] = transport
		}
		existing := e.routes[route.Prefix]
		if existing.configured != nil && existing.configured.sameSettings(route) {
			next[route.Prefix] = existing
			continue
		}
		lb, err := buildRoute(route, transport)
		if err != nil {
			return fmt.Errorf("route %q: %w", route.Prefix, err)
		}
		route.Upstreams = slices.Clone(route.Upstreams)
		next[route.Prefix] = routeEntry{balancer: lb, configured: &route}
	}

	e.mu.Lock()
	e.routes = next
	e.mu.Unlock()
	for timeout, transport := range e.transports {
		if transports[timeout] == nil {
			transport.CloseIdleConnections()
		}
	}
	e.transports = transports
	return nil
}

func (route Route) sameSettings(other Route) bool {
	return route.Strategy == other.Strategy &&
		route.ResponseHeaderTimeout == other.ResponseHeaderTimeout &&
		route.Circuit == other.Circuit &&
		slices.Equal(route.Upstreams, other.Upstreams)
}

func buildRoute(route Route, transport http.RoundTripper) (*LoadBalancer, error) {
	if !validPrefix(route.Prefix) || len(route.Upstreams) == 0 {
		return nil, fmt.Errorf("a canonical path prefix and at least one upstream are required")
	}
	var strategy BalancerStrategy
	switch route.Strategy {
	case "round_robin":
		strategy = &RoundRobinStrategy{}
	case "ip_hash":
		strategy = &HashStrategy{}
	case "random":
		strategy = &RandomStrategy{}
	default:
		return nil, fmt.Errorf("unknown strategy %q", route.Strategy)
	}
	policy := route.Circuit
	if policy.Threshold < 0 || policy.Cooldown < 0 || (policy.Threshold == 0) != (policy.Cooldown == 0) {
		return nil, fmt.Errorf("failure_threshold and cooldown must both be positive when configured")
	}

	lb := NewLoadBalancer(strategy)
	lb.policy = policy
	lb.transport = transport
	if err := lb.UpdateTargets(route.Upstreams); err != nil {
		return nil, err
	}
	return lb, nil
}
