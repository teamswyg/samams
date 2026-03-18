//go:build !deploy

// Local mode: WebSocket connection to server.
// Build without "deploy" tag (default).

package main

import (
	"log"

	"proxy/internal/adapter/inbound/cmdrouter"
	"proxy/internal/adapter/outbound/wsconn"
	"proxy/internal/app"
	"proxy/internal/port"
)

func createConnection(cfg config, token string, svcOpts *[]app.Option) port.ServerConnection {
	wsClient := wsconn.New(cfg.ServerURL, token, nil)
	wsPub := wsconn.NewPublisher(wsClient)
	*svcOpts = append(*svcOpts, app.WithPublisher(wsPub))
	return wsClient
}

func setupConnectionHandler(conn port.ServerConnection, svc port.TaskService, cfg config, tokenData *wsconn.TokenData) {
	wsClient, ok := conn.(*wsconn.Client)
	if !ok {
		log.Println("[conn] Warning: expected wsconn.Client in local mode")
		return
	}

	handler := cmdrouter.New(svc)
	wsClient.SetHandler(handler)
	wsClient.SetHeartbeatInterval(cfg.HeartbeatInterval)
	wsClient.SetHeartbeatProvider(func() wsconn.HeartbeatPayload {
		agents := svc.ListAgents()
		statuses := make(map[string]string, len(agents))
		for _, a := range agents {
			statuses[a.ID] = string(a.Status)
		}
		return wsconn.HeartbeatPayload{
			AgentStatuses: statuses,
			TaskCount:     len(svc.ListTasks()),
		}
	})
}

func updateConnectionToken(conn port.ServerConnection, newToken string) {
	if wsClient, ok := conn.(*wsconn.Client); ok {
		wsClient.UpdateToken(newToken)
		log.Println("[auth] WS client token updated")
	}
}
