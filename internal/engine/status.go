package engine

import (
	"slices"
	"strings"
	"time"
)

type RouteStatus struct {
	Prefix string `json:"prefix"`
	Ready  int    `json:"closed_circuits"`
	Open   int    `json:"open_circuits"`
	Probe  int    `json:"half_open_circuits"`
}

// RouteStatuses omits upstream addresses and returns a stable order for diagnostics.
func (e *Engine) RouteStatuses() []RouteStatus {
	e.mu.RLock()
	routes := make([]struct {
		prefix string
		lb     *LoadBalancer
	}, 0, len(e.routes))
	for prefix, entry := range e.routes {
		routes = append(routes, struct {
			prefix string
			lb     *LoadBalancer
		}{prefix, entry.balancer})
	}
	e.mu.RUnlock()

	now := time.Now()
	result := make([]RouteStatus, 0, len(routes))
	for _, route := range routes {
		status := RouteStatus{Prefix: route.prefix}
		route.lb.mu.RLock()
		targets := route.lb.targets
		route.lb.mu.RUnlock()
		for _, target := range targets {
			switch target.circuit.state(now) {
			case circuitOpen:
				status.Open++
			case circuitHalfOpen:
				status.Probe++
			default:
				status.Ready++
			}
		}
		result = append(result, status)
	}
	slices.SortFunc(result, func(a, b RouteStatus) int { return strings.Compare(a.Prefix, b.Prefix) })
	return result
}
