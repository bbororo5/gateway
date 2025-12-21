package mock

import (
	"context"
	"time"

	"github.com/jeonseon-ung/gateway/internal/discovery"
)

type MockProvider struct {
	events chan discovery.Event
}

func New() *MockProvider {
	return &MockProvider{
		events: make(chan discovery.Event, 10),
	}
}

func (m *MockProvider) Watch(ctx context.Context) (<-chan discovery.Event, error) {
	go func() {
		m.events <- discovery.Event{
			Type:      discovery.EventAdd,
			Service:   "/api/v1",
			Endpoints: []string{"http://localhost:8081", "http://localhost:8082"},
		}

		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		toggle := false

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				var endpoints []string

				if toggle {
					// 짝수 틱
					endpoints = []string{"http://localhost:8081", "http://localhost:8082"}
				} else {
					// 홀수 틱
					endpoints = []string{"http://localhost:8081", "http://localhost:8082", "http://localhost:8083"}
				}

				m.events <- discovery.Event{
					Type:      discovery.EventUpdate,
					Service:   "/api/v1",
					Endpoints: endpoints,
				}

				toggle = !toggle
			}
		}
	}()
	return m.events, nil
}

func (m *MockProvider) Close() error {
	close(m.events)
	return nil
}
