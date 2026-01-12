package mock

import (
	"context"
	"time"

	"github.com/jeonseon-ung/gateway/internal/discovery"
)

type MockProvider struct{}

func New() *MockProvider {
	return &MockProvider{}
}

func (m *MockProvider) Watch(ctx context.Context) (<-chan discovery.Event, error) {
	events := make(chan discovery.Event, 1)
	go func() {
		defer close(events)
		endpoints := []string{"http://localhost:8081", "http://localhost:8082"}
		send := func(kind discovery.EventType, urls []string) bool {
			select {
			case events <- discovery.Event{Type: kind, Service: "/api/v1", Endpoints: urls}:
				return true
			case <-ctx.Done():
				return false
			}
		}
		if !send(discovery.EventAdd, endpoints) {
			return
		}
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		expanded := append(append([]string(nil), endpoints...), "http://localhost:8083")
		next := expanded
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !send(discovery.EventUpdate, next) {
					return
				}
				if len(next) == len(expanded) {
					next = endpoints
				} else {
					next = expanded
				}
			}
		}
	}()
	return events, nil
}

func (m *MockProvider) Close() error {
	return nil
}
