package engine

import (
	"strings"
	"testing"
)

func TestRouteConfigurationRejectsInvalidSnapshots(t *testing.T) {
	tests := []struct {
		name    string
		route   Route
		message string
	}{
		{"empty upstreams", Route{Prefix: "/api"}, "at least one upstream"},
		{"duplicate upstream", Route{Prefix: "/api", Upstreams: []string{"http://localhost", "http://localhost"}}, "duplicate upstream"},
		{"upstream credentials", Route{Prefix: "/api", Upstreams: []string{"http://name:secret@localhost"}}, "without credentials"},
		{"upstream fragment", Route{Prefix: "/api", Upstreams: []string{"http://localhost#fragment"}}, "without credentials or fragment"},
		{"relative upstream", Route{Prefix: "/api", Upstreams: []string{"/local"}}, "absolute HTTP(S) URL"},
		{"negative timeout", Route{Prefix: "/api", Upstreams: []string{"http://localhost"}, ResponseHeaderTimeout: -1}, "cannot be negative"},
		{"cooldown only", Route{Prefix: "/api", Upstreams: []string{"http://localhost"}, Circuit: CircuitPolicy{Cooldown: 1}}, "must both be positive"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			eng := New()
			err := eng.ReplaceRoutes([]Route{tc.route})
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("got %v, want error containing %q", err, tc.message)
			}
			if eng.Ready() {
				t.Fatal("invalid route became active")
			}
		})
	}
}
