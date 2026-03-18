package project

import (
	"time"

	"server/internal/domain/shared"
)

// TaskMapNode represents a single task in the task map.
type TaskMapNode struct {
	ID             string
	Title          string
	BoundedContext string
	Order          int
}

// TaskMapAggregate is the root aggregate for task planning.
type TaskMapAggregate struct {
	ID                   string
	ProjectID            ID
	PromptRef            string
	BoundedContextPrompt string
	Nodes                []TaskMapNode
	Status               string
	CreatedAt            time.Time

	events []shared.DomainEvent
}

// InitializeTaskMap creates a new task map from user prompt.
func InitializeTaskMap(clock shared.Clock, projectID ID, promptRef, boundedContextPrompt string, nodeTitles []string) *TaskMapAggregate {
	nodes := make([]TaskMapNode, len(nodeTitles))
	for i, title := range nodeTitles {
		nodes[i] = TaskMapNode{
			ID:    shared.GenerateID(),
			Title: title,
			Order: i,
		}
	}

	m := &TaskMapAggregate{
		ID:                   shared.GenerateID(),
		ProjectID:            projectID,
		PromptRef:            promptRef,
		BoundedContextPrompt: boundedContextPrompt,
		Nodes:                nodes,
		Status:               "initialized",
		CreatedAt:            clock.Now(),
	}

	m.addEvent(clock, "TaskMapInitialized", map[string]any{
		"map_id":     m.ID,
		"node_count": len(nodes),
		"prompt_ref": promptRef,
	})

	return m
}

// PullDomainEvents returns and clears pending events.
func (m *TaskMapAggregate) PullDomainEvents() []shared.DomainEvent {
	out := make([]shared.DomainEvent, len(m.events))
	copy(out, m.events)
	m.events = m.events[:0]
	return out
}

func (m *TaskMapAggregate) addEvent(clock shared.Clock, name string, payload any) {
	m.events = append(m.events, shared.NewDomainEvent(
		clock, name, "project", m.ID, "task_map", m.ID, "info", payload,
	))
}
