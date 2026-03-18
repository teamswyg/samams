package task

import (
	"testing"

	"server/internal/domain/shared"
)

func TestNewTask_Valid(t *testing.T) {
	clock := shared.SystemClock{}
	id := shared.NewTaskID("t1")
	tenant := shared.NewTenantID("tenant1")
	project := shared.NewProjectID("proj1")

	task, err := NewTask(clock, id, tenant, project, "title", "desc", nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if task.Title != "title" {
		t.Errorf("unexpected title: %s", task.Title)
	}
	if task.Status != StatusCreated {
		t.Errorf("unexpected status: %v", task.Status)
	}
}

