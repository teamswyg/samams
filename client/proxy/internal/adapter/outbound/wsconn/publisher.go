package wsconn

import (
	"context"
	"log"

	"proxy/internal/domain"
)

// Publisher bridges the port.Publisher interface to a WebSocket client.
type Publisher struct {
	client *Client
}

func NewPublisher(c *Client) *Publisher {
	return &Publisher{client: c}
}

var eventTypeToAction = map[domain.EventType]string{
	domain.EventAgentStateChanged: "agent.stateChanged",
	domain.EventAgentCreated:      "agent.stateChanged",
	domain.EventAgentPaused:       "agent.stateChanged",
	domain.EventAgentResumed:      "agent.stateChanged",
	domain.EventAgentStopped:      "agent.stateChanged",
	domain.EventAgentError:        "agent.stateChanged",
	domain.EventAgentLogAppended:  "agent.logAppended",
	domain.EventTaskCompleted:     "task.completed",
	domain.EventTaskCancelled:     "task.failed",
	domain.EventMaalRecordCreated: "maal.record",

	domain.EventMilestoneReviewCompleted: "milestone.review.completed",
	domain.EventMilestoneReviewFailed:    "milestone.review.failed",
	domain.EventMilestoneMerged:          "milestone.merged",

	domain.EventStrategyAllPaused:        "strategy.allPaused",
	domain.EventStrategyDecisionApplied: "strategy.decisionApplied",
}

func (p *Publisher) Publish(_ context.Context, e domain.Envelope) error {
	action, ok := eventTypeToAction[e.Type]
	if !ok {
		return nil
	}

	payload := map[string]any{
		"eventType": string(e.Type),
		"taskID":    e.TaskID,
	}
	for k, v := range e.Payload {
		payload[k] = v
	}
	for k, v := range e.Metadata {
		payload[k] = v
	}

	if err := p.client.SendEvent(action, payload); err != nil {
		log.Printf("[ws-pub] Failed to send event %s: %v", e.Type, err)
		return nil
	}
	return nil
}
