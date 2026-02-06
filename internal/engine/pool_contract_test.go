package engine

import (
	"net/http"
	"testing"
	"time"
)

func TestUpstreamPoolContract(t *testing.T) {
	const endpoint = "http://localhost"
	const baseline = 5 * time.Second
	cases := []struct {
		name   string
		verify func(*testing.T, *Engine, func(...Route), *http.Transport)
	}{
		{"same timeout", func(t *testing.T, e *Engine, install func(...Route), original *http.Transport) {
			install(Route{Prefix: "/a", Upstreams: []string{endpoint}}, Route{Prefix: "/b", Upstreams: []string{endpoint}})
			if e.routes["/a"].balancer.transport != original || e.routes["/b"].balancer.transport != original {
				t.Fatal("matching routes did not share a pool")
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eng := New()
			defer eng.CloseIdleConnections()
			install := func(routes ...Route) {
				t.Helper()
				if err := eng.ReplaceRoutes(routes); err != nil {
					t.Fatal(err)
				}
			}
			install(Route{Prefix: "/a", Upstreams: []string{endpoint}})
			original := eng.transports[baseline]
			if original == nil {
				t.Fatal("missing default response timeout pool")
			}
			tc.verify(t, eng, install, original)
		})
	}
}
