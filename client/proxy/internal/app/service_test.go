package app

import (
	"context"
	"errors"
	"sync"
	"testing"

	"proxy/internal/domain"
	"proxy/internal/port"
)

type mockRunner struct {
	mu      sync.Mutex
	started map[string]port.StartOptions
	stopped map[string]struct{}
	inputs  map[string][]string
}

func newMockRunner() *mockRunner {
	return &mockRunner{
		started: make(map[string]port.StartOptions),
		stopped: make(map[string]struct{}),
		inputs:  make(map[string][]string),
	}
}

func (m *mockRunner) StartAgent(_ context.Context, agentID string, opts port.StartOptions, _ port.LogFunc) (*port.Handle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started[agentID] = opts
	return port.NewHandleForTest(), nil
}

func (m *mockRunner) StopAgent(_ context.Context, agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.started[agentID]; !ok {
		return errors.New("not found")
	}
	m.stopped[agentID] = struct{}{}
	return nil
}

func (m *mockRunner) SendInput(agentID, input string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inputs[agentID] = append(m.inputs[agentID], input)
	return nil
}

func TestCreateTaskAndScale(t *testing.T) {
	r := newMockRunner()
	svc := NewTaskService(r)

	ctx := context.Background()
	summary, err := svc.CreateTask(ctx, domain.CreateTaskParams{
		Name:      "test",
		Prompt:    "do something",
		NumAgents: 2,
	})
	if err != nil {
		t.Fatalf("CreateTask error: %v", err)
	}
	if summary.Task == nil {
		t.Fatalf("expected task in summary")
	}
	if len(summary.Agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(summary.Agents))
	}

	summary, err = svc.ScaleTask(ctx, summary.Task.ID, domain.ScaleTaskParams{NumAgents: 3})
	if err != nil {
		t.Fatalf("ScaleTask error: %v", err)
	}
	if len(summary.Agents) != 3 {
		t.Fatalf("expected 3 agents after scale up, got %d", len(summary.Agents))
	}

	summary, err = svc.ScaleTask(ctx, summary.Task.ID, domain.ScaleTaskParams{NumAgents: 1})
	if err != nil {
		t.Fatalf("ScaleTask error: %v", err)
	}
	if len(summary.Agents) != 1 {
		t.Fatalf("expected 1 agent after scale down, got %d", len(summary.Agents))
	}
}

func TestResetTask(t *testing.T) {
	r := newMockRunner()
	svc := NewTaskService(r)

	ctx := context.Background()
	summary, err := svc.CreateTask(ctx, domain.CreateTaskParams{
		Name:      "test",
		Prompt:    "do something",
		NumAgents: 2,
	})
	if err != nil {
		t.Fatalf("CreateTask error: %v", err)
	}

	origAgentIDs := make(map[string]struct{})
	for _, a := range summary.Agents {
		origAgentIDs[a.ID] = struct{}{}
	}

	summary, err = svc.ResetTask(ctx, summary.Task.ID)
	if err != nil {
		t.Fatalf("ResetTask error: %v", err)
	}
	if len(summary.Agents) != 2 {
		t.Fatalf("expected 2 agents after reset, got %d", len(summary.Agents))
	}

	for _, a := range summary.Agents {
		if _, existed := origAgentIDs[a.ID]; existed {
			t.Fatalf("expected new agent IDs after reset, but found %s", a.ID)
		}
	}
}

func TestStopTask(t *testing.T) {
	r := newMockRunner()
	svc := NewTaskService(r)

	ctx := context.Background()
	summary, err := svc.CreateTask(ctx, domain.CreateTaskParams{
		Name:      "test",
		Prompt:    "do something",
		NumAgents: 1,
	})
	if err != nil {
		t.Fatalf("CreateTask error: %v", err)
	}

	if err := svc.StopTask(ctx, summary.Task.ID, nil); err != nil {
		t.Fatalf("StopTask error: %v", err)
	}
	if summary.Task.Status == domain.TaskStatusRunning {
		t.Fatalf("expected task not running after StopTask")
	}
}
