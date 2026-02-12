package engine

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRouteStatusContract(t *testing.T) {
	cases := []struct {
		name    string
		route   Route
		arrange func(*testing.T, *Engine)
		want    RouteStatus
	}{
		{"open", Route{Circuit: CircuitPolicy{Threshold: 1, Cooldown: time.Minute}}, func(t *testing.T, e *Engine) {
			w := httptest.NewRecorder()
			e.ServeHTTP(w, httptest.NewRequest("GET", "/api", nil))
			if w.Code != http.StatusBadGateway {
				t.Fatal(w.Code)
			}
		}, RouteStatus{Prefix: "/api", Open: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			failed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
			}))
			defer failed.Close()
			eng := New()
			defer eng.CloseIdleConnections()
			route := tc.route
			route.Prefix = "/api"
			route.Upstreams = []string{failed.URL}
			if err := eng.ReplaceRoutes([]Route{route}); err != nil {
				t.Fatal(err)
			}
			tc.arrange(t, eng)
			got := eng.RouteStatuses()
			if len(got) != 1 || got[0] != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}
