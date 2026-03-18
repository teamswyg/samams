//go:build deploy

// Deploy mode: HTTPS polling to API Gateway.
// Build with: go build -tags deploy

package main

import (
	"log"

	"proxy/internal/adapter/inbound/cmdrouter"
	"proxy/internal/adapter/outbound/httppoll"
	"proxy/internal/adapter/outbound/wsconn"
	"proxy/internal/app"
	"proxy/internal/domain"
	"proxy/internal/port"
)

func createConnection(cfg config, token string, svcOpts *[]app.Option) port.ServerConnection {
	httpClient := httppoll.New(httppoll.Config{
		ServerURL:          cfg.ServerURL,
		Token:              token,
		PollIntervalActive: cfg.PollIntervalActive,
		PollIntervalIdle:   cfg.PollIntervalIdle,
		HeartbeatInterval:  cfg.HeartbeatInterval,
	}, nil)
	httpPub := httppoll.NewPublisher(httpClient)
	*svcOpts = append(*svcOpts, app.WithPublisher(httpPub))
	return httpClient
}

func setupConnectionHandler(conn port.ServerConnection, svc port.TaskService, cfg config, tokenData *wsconn.TokenData) {
	httpClient, ok := conn.(*httppoll.Client)
	if !ok {
		log.Println("[conn] Warning: expected httppoll.Client in deploy mode")
		return
	}

	handler := cmdrouter.New(svc)
	httpClient.SetHandler(handler)
	httpClient.SetHeartbeatProvider(func() httppoll.HeartbeatPayload {
		agents := svc.ListAgents()
		statuses := make(map[string]string, len(agents))
		for _, a := range agents {
			statuses[a.ID] = string(a.Status)
		}
		return httppoll.HeartbeatPayload{
			AgentStatuses: statuses,
			TaskCount:     len(svc.ListTasks()),
		}
	})
	httpClient.SetActiveChecker(func() bool {
		for _, a := range svc.ListAgents() {
			if a.Status == domain.AgentStatusRunning || a.Status == domain.AgentStatusStarting {
				return true
			}
		}
		return false
	})
}

func updateConnectionToken(conn port.ServerConnection, newToken string) {
	if httpClient, ok := conn.(*httppoll.Client); ok {
		httpClient.UpdateToken(newToken)
		log.Println("[auth] HTTPS client token updated")
	}
}
