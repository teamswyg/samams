package event

import (
	"context"
	"time"
)

type Publisher interface {
	Publish(ctx context.Context, e Envelope) error
}

func NewEnvelope(t Type, source string) Envelope {
	return Envelope{
		ID:        "",
		Type:      t,
		Source:    source,
		Timestamp: time.Now().UnixMilli(),
		Payload:   map[string]any{},
		Metadata:  map[string]string{},
	}
}

