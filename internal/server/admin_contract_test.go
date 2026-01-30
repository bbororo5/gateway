package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/jeonseon-ung/gateway/internal/config"
	"github.com/jeonseon-ung/gateway/internal/engine"
)

func TestRouteDiagnosticsContract(t *testing.T) {
	const endpoint = "http://private-upstream.example"
	const diagnostics = "/routes"
	_ = endpoint
	cases := []struct {
		name, method string
		public       bool
		routes       []engine.Route
		code         int
		want         []string
	}{
		{"empty snapshot", "GET", false, nil, 200, []string{}},
		{"ordered and redacted", "GET", false, []engine.Route{{Prefix: "/z", Upstreams: []string{endpoint}}, {Prefix: "/a", Upstreams: []string{endpoint}}}, 200, []string{"/a", "/z"}},
		{"HEAD probe", "HEAD", false, nil, 200, nil},
		{"read only", "POST", false, nil, 405, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eng := engine.New()
			defer eng.CloseIdleConnections()
			if err := eng.ReplaceRoutes(tc.routes); err != nil {
				t.Fatal(err)
			}
			srv := New(&config.Config{Server: config.ServerConfig{AdminAddress: "127.0.0.1:9090"}}, eng)
			handler := srv.admin.Handler
			if tc.public {
				handler = srv.httpServer.Handler
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(tc.method, diagnostics, nil))
			if w.Code != tc.code {
				t.Fatalf("got %d, want %d", w.Code, tc.code)
			}
			if tc.code != http.StatusOK || tc.method == http.MethodHead {
				return
			}
			if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("diagnostic headers: %v", w.Header())
			}
			if strings.Contains(w.Body.String(), "private-upstream") {
				t.Fatal("upstream address leaked")
			}
			var status []engine.RouteStatus
			if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil || status == nil {
				t.Fatalf("invalid status: %v", err)
			}
			prefixes := make([]string, 0, len(status))
			for _, route := range status {
				prefixes = append(prefixes, route.Prefix)
				if route.Ready != 1 || route.Open != 0 || route.Probe != 0 {
					t.Fatalf("unexpected circuit summary %+v", route)
				}
			}
			if !reflect.DeepEqual(prefixes, tc.want) {
				t.Fatalf("prefixes %v, want %v", prefixes, tc.want)
			}
		})
	}
}
