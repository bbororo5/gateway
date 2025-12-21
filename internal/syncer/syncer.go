package syncer

import (
	"context"
	"log/slog"

	"github.com/jeonseon-ung/gateway/internal/discovery"
	"github.com/jeonseon-ung/gateway/internal/engine"
)

type Syncer struct {
	engine   *engine.Engine
	provider discovery.Provider
}

func New(eng *engine.Engine, prov discovery.Provider) *Syncer {
	return &Syncer{
		engine:   eng,
		provider: prov,
	}
}

func (s *Syncer) Start(ctx context.Context) error {
	events, err := s.provider.Watch(ctx)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-events:
			if !ok {
				return nil
			}
			s.handleEvent(event)
		}
	}
}

func (s *Syncer) handleEvent(event discovery.Event) {
	slog.Info("Received event", "event_type", event.Type, "service", event.Service)

	switch event.Type {
	case discovery.EventAdd, discovery.EventUpdate:
		err := s.engine.UpdateServiceEndpoints(event.Service, event.Endpoints, nil)
		if err != nil {
			slog.Error("Failed to update endpoints", "error", err)
		}
	case discovery.EventDelete:
		s.engine.DeleteService(event.Service)
	}
}
