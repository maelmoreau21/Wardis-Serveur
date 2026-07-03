package accesscontrol

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
)

type EventPublisher interface {
	PublishAccessGranted(ctx context.Context, payload interface{}) error
	PublishAccessDenied(ctx context.Context, payload interface{}) error
	Close()
}

type natsPublisher struct {
	nc *nats.Conn
}

func NewNatsPublisher(natsURL string) (EventPublisher, error) {
	nc, err := nats.Connect(natsURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS at %s: %w", natsURL, err)
	}
	return &natsPublisher{nc: nc}, nil
}

func (p *natsPublisher) PublishAccessGranted(ctx context.Context, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal access.granted event: %w", err)
	}
	return p.nc.Publish("access.granted", data)
}

func (p *natsPublisher) PublishAccessDenied(ctx context.Context, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal access.denied event: %w", err)
	}
	return p.nc.Publish("access.denied", data)
}

func (p *natsPublisher) Close() {
	if p.nc != nil {
		p.nc.Close()
	}
}
