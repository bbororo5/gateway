package engine

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouteRequestContract(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Observed-Request", r.Method+" "+r.URL.RequestURI()+" "+r.Header.Get("X-Forwarded-Prefix"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer backend.Close()
	eng := New()
	defer eng.CloseIdleConnections()
	routes := []Route{
		{Prefix: "/", Upstreams: []string{backend.URL + "/fallback"}},
		{Prefix: "/api", Upstreams: []string{backend.URL + "/plain"}, AllowedMethods: []string{"GET"}},
		{Prefix: "/api/v2", Upstreams: []string{backend.URL + "/service"}, StripPrefix: true},
		{Prefix: "/café", Upstreams: []string{backend.URL}, StripPrefix: true},
		{Prefix: "/upload", Upstreams: []string{backend.URL}, AllowedMethods: []string{"PUT", "OPTIONS"}},
	}
	if err := eng.ReplaceRoutes(routes); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, method, path, forwarded, observed, allow string
		status                                         int
	}{
		{"deepest", "GET", "/api/v2/items", "", "GET /service/items /api/v2", "", 204},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.forwarded != "" {
				r.Header.Set("X-Forwarded-Prefix", tc.forwarded)
			}
			w := httptest.NewRecorder()
			eng.ServeHTTP(w, r)
			if w.Code != tc.status || w.Header().Get("Allow") != tc.allow ||
				w.Header().Get("X-Observed-Request") != tc.observed {
				t.Fatalf("status=%d headers=%v, want status=%d Allow=%q upstream=%q",
					w.Code, w.Header(), tc.status, tc.allow, tc.observed)
			}
		})
	}
}
