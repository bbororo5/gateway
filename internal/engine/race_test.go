package engine

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestEngine_Concurrency_HotSwap(t *testing.T) {
	backend := func() *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
	}
	first, second := backend(), backend()
	defer first.Close()
	defer second.Close()

	eng := New()
	defer eng.CloseIdleConnections()
	const prefix = "/api/v1"
	install := func(endpoint string) error {
		return eng.ReplaceRoutes([]Route{{Prefix: prefix, Upstreams: []string{endpoint}}})
	}
	if err := install(first.URL); err != nil {
		t.Fatal(err)
	}

	var workers sync.WaitGroup
	start := make(chan struct{})
	for range 8 {
		workers.Go(func() {
			<-start
			for range 64 {
				w := httptest.NewRecorder()
				eng.ServeHTTP(w, httptest.NewRequest(http.MethodGet, prefix+"/resource", nil))
				if w.Code != http.StatusNoContent {
					t.Errorf("route unavailable during reload: %d", w.Code)
					return
				}
			}
		})
	}
	for range 2 {
		workers.Go(func() {
			<-start
			for n := range 64 {
				endpoints := [...]string{first.URL, second.URL}
				if err := install(endpoints[n%len(endpoints)]); err != nil {
					t.Error(err)
					return
				}
			}
		})
	}
	close(start)
	workers.Wait()
}
