package httpstats

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

var bounds = [...]float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}
var classes = [...]string{"other", "1xx", "2xx", "3xx", "4xx", "5xx"}

type counters struct {
	inFlight int
	requests [len(classes)]uint64
	buckets  [len(bounds)]uint64
	count    uint64
	seconds  float64
}

type Metrics struct {
	mu   sync.Mutex
	data counters
}

func (m *Metrics) Instrument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		m.mu.Lock()
		m.data.inFlight++
		m.mu.Unlock()

		recorder := &Response{ResponseWriter: w}
		completed := false
		defer func() {
			status := recorder.StatusCode()
			if !completed && recorder.Status == 0 {
				status = http.StatusInternalServerError
			}
			m.observe(status, time.Since(start).Seconds())
		}()
		next.ServeHTTP(recorder, r)
		completed = true
	})
}

func (m *Metrics) observe(status int, elapsed float64) {
	class := status / 100
	if class < 0 || class >= len(classes) {
		class = 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data.inFlight--
	m.data.requests[class]++
	m.data.seconds += elapsed
	m.data.count++
	for i, bound := range bounds {
		if elapsed <= bound {
			m.data.buckets[i]++
		}
	}
}

func (m *Metrics) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	m.mu.Lock()
	data := m.data
	m.mu.Unlock()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprintln(w, "# TYPE gateway_http_in_flight gauge")
	fmt.Fprintf(w, "gateway_http_in_flight %d\n", data.inFlight)
	fmt.Fprintln(w, "# TYPE gateway_http_requests_total counter")
	for i, name := range classes {
		fmt.Fprintf(w, "gateway_http_requests_total{status_class=%q} %d\n", name, data.requests[i])
	}
	fmt.Fprintln(w, "# TYPE gateway_http_request_duration_seconds histogram")
	for i, bound := range bounds {
		fmt.Fprintf(w, "gateway_http_request_duration_seconds_bucket{le=\"%g\"} %d\n", bound, data.buckets[i])
	}
	fmt.Fprintf(w, "gateway_http_request_duration_seconds_bucket{le=\"+Inf\"} %d\n", data.count)
	fmt.Fprintf(w, "gateway_http_request_duration_seconds_sum %g\n", data.seconds)
	fmt.Fprintf(w, "gateway_http_request_duration_seconds_count %d\n", data.count)
}
