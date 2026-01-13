package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jeonseon-ung/gateway/internal/config"
	"github.com/jeonseon-ung/gateway/internal/engine"
	"github.com/jeonseon-ung/gateway/internal/middleware/auth"
)

func TestOperationalPipeline(t *testing.T) {
	const key = "integration-client-key-not-real"
	signing := []byte("integration-signing-key-not-real-long-enough")
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "" || r.Header.Get("X-Forwarded-For") != "192.0.2.1" || r.Header.Get("X-Forwarded-Proto") != "http" {
			t.Error("credential leaked or spoofed forwarding header reached upstream")
		}
		claims := &auth.InternalClaims{}
		_, err := jwt.ParseWithClaims(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), claims,
			func(*jwt.Token) (any, error) { return signing, nil },
			jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer("gateway"), jwt.WithExpirationRequired())
		if err != nil || claims.UserID != "app" || claims.Role != "reader" {
			t.Errorf("wrong upstream identity: %v, %+v", err, claims)
		}
		http.NewResponseController(w).Flush()
		io.WriteString(w, "streamed")
	}))
	defer backend.Close()

	eng := engine.New()
	srv := New(&config.Config{
		Server: config.ServerConfig{AdminAddress: "127.0.0.1:9090"},
		Middleware: config.MiddlewareConfig{
			GlobalEnabled: []string{"logging", "auth", "ratelimit"},
			Auth: config.AuthConfig{
				Enabled: true, ApiKeyHeader: "X-Api-Key", InternalSigningKey: string(signing),
				Credentials: []config.Credential{{Key: key, UserID: "app", Role: "reader"}},
			},
			RateLimit: config.RateLimitConfig{Enabled: true, RequestsPerSecond: 1, Burst: 1},
		},
	}, eng)
	admin := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		srv.admin.Handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w
	}
	if admin("/livez").Code != http.StatusOK || admin("/readyz").Code != http.StatusServiceUnavailable {
		t.Fatal("probe state incorrect before routes are loaded")
	}
	if err := eng.ReplaceRoutes([]engine.Route{{Prefix: "/api", Upstreams: []string{backend.URL}}}); err != nil {
		t.Fatal(err)
	}
	if admin("/readyz").Code != http.StatusOK {
		t.Fatal("configured gateway not ready")
	}

	request := func(apiKey, authorization, connection string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/api/items", nil)
		r.Header.Set("X-Forwarded-For", "203.0.113.99")
		r.Header.Set("X-Forwarded-Proto", "https")
		if apiKey != "" {
			r.Header.Set("X-Api-Key", apiKey)
		}
		if authorization != "" {
			r.Header.Set("Authorization", authorization)
		}
		if connection != "" {
			r.Header.Set("Connection", connection)
		}
		srv.httpServer.Handler.ServeHTTP(w, r)
		return w
	}
	if got := request(key, "Bearer "+key, "").Code; got != http.StatusUnauthorized {
		t.Fatalf("ambiguous credentials returned %d", got)
	}
	if got := request(key, "", "Authorization").Code; got != http.StatusBadRequest {
		t.Fatalf("hop-by-hop authorization returned %d", got)
	}
	w := request(key, "", "")
	if w.Code != http.StatusOK || w.Body.String() != "streamed" || w.Header().Get("X-Request-ID") == "" {
		t.Fatalf("authorized request failed: %d %q", w.Code, w.Body.String())
	}
	if got := request("", "Bearer "+key, "").Code; got != http.StatusTooManyRequests {
		t.Fatalf("expected rate limit, got %d", got)
	}

	metrics := admin("/metrics").Body.String()
	for _, sample := range []string{
		`gateway_http_requests_total{status_class="2xx"} 1`,
		`gateway_http_requests_total{status_class="4xx"} 3`,
		`gateway_http_request_duration_seconds_count 4`,
		`gateway_http_in_flight 0`,
	} {
		if !strings.Contains(metrics, sample) {
			t.Errorf("metric missing: %s", sample)
		}
	}
	srv.draining.Store(true)
	if admin("/readyz").Code != http.StatusServiceUnavailable {
		t.Fatal("draining gateway is still marked ready")
	}
}
