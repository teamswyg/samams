package port

import (
	"context"
	"encoding/json"
)

// ServerConnection is a secondary (driven) port for server communication.
// Local mode: WebSocket (persistent connection)
// Deploy mode: HTTPS polling (stateless)
type ServerConnection interface {
	Run(ctx context.Context) error
	SendEvent(action string, payload any) error
	Close()
}

// CommandHandler processes a command from the server and returns a response payload.
// Shared type used by both WS and HTTPS adapters.
type CommandHandler func(action string, payload json.RawMessage) (json.RawMessage, error)
