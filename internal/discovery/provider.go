package discovery

import "context"

type EventType int

const (
	EventAdd EventType = iota
	EventDelete
	EventUpdate
)

type Event struct {
	Type      EventType
	Service   string
	Endpoints []string
}

type Provider interface {
	Watch(ctx context.Context) (<-chan Event, error)
	Close() error
}