package video

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
)

type EventPublisher interface {
	PublishCameraDiscovered(ctx context.Context, payload interface{}) error
	PublishEvent(ctx context.Context, subject string, payload interface{}) error
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

func (p *natsPublisher) PublishCameraDiscovered(ctx context.Context, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal cameras.discovered event: %w", err)
	}
	return p.nc.Publish("cameras.discovered", data)
}

func (p *natsPublisher) PublishEvent(ctx context.Context, subject string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal video event payload: %w", err)
	}
	return p.nc.Publish(subject, data)
}

func (p *natsPublisher) Close() {
	if p.nc != nil {
		p.nc.Close()
	}
}
