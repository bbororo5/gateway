package engine

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestReplaceRoutesRejectsWholeInvalidSnapshot(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, "stable")
	}))
	defer backend.Close()

	eng := New()
	route := Route{Prefix: "/api", Upstreams: []string{backend.URL}}
	if err := eng.ReplaceRoutes([]Route{route}); err != nil {
		t.Fatal(err)
	}

	for _, bad := range []Route{
		{Prefix: "/api", Upstreams: []string{backend.URL}},
		{Prefix: "/next", Upstreams: []string{"file:///etc/passwd"}},
		{Prefix: "/next", Upstreams: []string{backend.URL}, Strategy: "unknown"},
		{Prefix: "/next", Upstreams: []string{backend.URL}, Circuit: CircuitPolicy{Threshold: 2}},
		{Prefix: "/next/", Upstreams: []string{backend.URL}},
	} {
		if err := eng.ReplaceRoutes([]Route{route, bad}); err == nil {
			t.Fatalf("invalid route accepted: %+v", bad)
		}
		recorder := httptest.NewRecorder()
		eng.ServeHTTP(recorder, httptest.NewRequest("GET", "/api/ping", nil))
		if recorder.Code != http.StatusOK || recorder.Body.String() != "stable" {
			t.Fatalf("last valid table lost: %d %q", recorder.Code, recorder.Body.String())
		}
	}
}

func TestRouteMatchesSegmentsAndRejectsAmbiguousPaths(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer backend.Close()

	eng := New()
	if err := eng.ReplaceRoutes([]Route{{Prefix: "/api", Upstreams: []string{backend.URL}}}); err != nil {
		t.Fatal(err)
	}
	for target, want := range map[string]int{
		"/api": http.StatusNoContent, "/api/": http.StatusNoContent, "/api/items": http.StatusNoContent,
		"/api-other": http.StatusNotFound, "/api/../private": http.StatusBadRequest,
		"/api/%2e%2e/private": http.StatusBadRequest, "/api%2fitems": http.StatusBadRequest,
		"/api//items": http.StatusBadRequest, "/api/%5citems": http.StatusBadRequest,
	} {
		t.Run(target, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			eng.ServeHTTP(recorder, httptest.NewRequest("GET", target, nil))
			if recorder.Code != want {
				t.Fatalf("status = %d, want %d", recorder.Code, want)
			}
		})
	}
}

func TestCircuitAllowsOneRecoveryProbeAndIgnoresStaleResults(t *testing.T) {
	now := time.Unix(100, 0)
	c := circuit{policy: CircuitPolicy{Threshold: 1, Cooldown: time.Second}}
	first, ok := c.admit(now)
	if !ok {
		t.Fatal("new circuit is closed to traffic")
	}
	c.record(first, failure, now)
	if _, allowed := c.admit(now); allowed {
		t.Fatal("failed upstream was admitted during cooldown")
	}

	probe, ok := c.admit(now.Add(time.Second))
	if !ok {
		t.Fatal("recovery probe was rejected")
	}
	c.record(first, success, now.Add(time.Second))
	if _, allowed := c.admit(now.Add(time.Second)); allowed {
		t.Fatal("stale response or concurrent recovery probe reopened the circuit")
	}
	c.record(probe, neutral, now.Add(time.Second))

	next, ok := c.admit(now.Add(time.Second))
	if !ok {
		t.Fatal("a canceled probe should release its slot")
	}
	c.record(next, success, now.Add(time.Second))
	if _, allowed := c.admit(now.Add(time.Second)); !allowed {
		t.Fatal("successful recovery did not reopen traffic")
	}
}

func TestCircuitSkipsFailedUpstreamWithoutReplayingRequests(t *testing.T) {
	var badCalls atomic.Int64
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		badCalls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer good.Close()

	eng := New()
	err := eng.ReplaceRoutes([]Route{{
		Prefix: "/", Upstreams: []string{good.URL, bad.URL},
		Circuit: CircuitPolicy{Threshold: 1, Cooldown: time.Minute},
	}})
	if err != nil {
		t.Fatal(err)
	}

	for _, status := range []int{http.StatusInternalServerError, http.StatusNoContent, http.StatusNoContent} {
		w := httptest.NewRecorder()
		eng.ServeHTTP(w, httptest.NewRequest("POST", "/", nil))
		if w.Code != status {
			t.Fatalf("status = %d, want %d", w.Code, status)
		}
	}
	if calls := badCalls.Load(); calls != 1 {
		t.Fatalf("bad upstream received %d requests, want 1", calls)
	}
}
