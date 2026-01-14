package server

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jeonseon-ung/gateway/internal/config"
	"github.com/jeonseon-ung/gateway/internal/engine"
)

// BenchmarkGatewayLoad exercises client, gateway, and upstream over loopback.
// All cases request the same route to isolate the cost of table size.
func BenchmarkGatewayLoad(b *testing.B) {
	// Keep sockets reusable across all workers so TCP port exhaustion does
	// not dominate the route-count and metrics comparison.
	defaultTransport := http.DefaultTransport
	upstreamTransport := defaultTransport.(*http.Transport).Clone()
	upstreamTransport.MaxIdleConnsPerHost = 128
	http.DefaultTransport = upstreamTransport
	b.Cleanup(func() { http.DefaultTransport = defaultTransport })

	for _, count := range []int{10, 100, 1000, 5000} {
		for _, metrics := range []bool{false, true} {
			b.Run(fmt.Sprintf("routes=%d/metrics=%t", count, metrics), func(b *testing.B) {
				backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					io.WriteString(w, "ok")
				}))
				defer backend.Close()

				routes := make([]engine.Route, count)
				for i := range routes {
					routes[i] = engine.Route{
						Prefix:    fmt.Sprintf("/api/service-%04d", i),
						Upstreams: []string{backend.URL},
					}
				}
				eng := engine.New()
				if err := eng.ReplaceRoutes(routes); err != nil {
					b.Fatal(err)
				}

				cfg := &config.Config{}
				if metrics {
					cfg.Server.AdminAddress = "127.0.0.1:9090"
				}
				srv := httptest.NewServer(New(cfg, eng).httpServer.Handler)
				defer srv.Close()

				transport := &http.Transport{MaxIdleConns: 128, MaxIdleConnsPerHost: 128}
				defer transport.CloseIdleConnections()
				client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
				target := srv.URL + routes[count/2].Prefix + "/items"
				b.ReportAllocs()
				b.SetParallelism(4)
				var sampleMu sync.Mutex
				var samples []time.Duration

				b.ResetTimer()
				b.RunParallel(func(pb *testing.PB) {
					var workerSamples []time.Duration
					for i := 0; pb.Next(); i++ {
						var start time.Time
						if i%16 == 0 {
							start = time.Now()
						}
						resp, err := client.Get(target)
						if err != nil {
							b.Error(err)
							return
						}
						n, err := io.Copy(io.Discard, resp.Body)
						resp.Body.Close()
						if err != nil || resp.StatusCode != http.StatusOK || n != 2 {
							b.Errorf("invalid upstream response: status=%d bytes=%d error=%v", resp.StatusCode, n, err)
							return
						}
						if !start.IsZero() {
							workerSamples = append(workerSamples, time.Since(start))
						}
					}
					sampleMu.Lock()
					samples = append(samples, workerSamples...)
					sampleMu.Unlock()
				})
				b.StopTimer()

				b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "req/s")
				if len(samples) > 0 {
					slices.Sort(samples)
					p95 := samples[(95*len(samples)-1)/100]
					b.ReportMetric(float64(p95)/float64(time.Millisecond), "p95-ms")
				}
			})
		}
	}
}
