package inmemory

import (
	"context"
	"sync"

	"server/internal/domain/shared"
	domainTask "server/internal/domain/task"
	appTask "server/internal/app/task"
)

// Compile-time interface check.
var _ appTask.TaskRepository = (*TaskRepository)(nil)

// TaskRepository is an in-memory implementation of TaskRepository.
type TaskRepository struct {
	mu    sync.RWMutex
	tasks map[shared.TaskID]*domainTask.Task
	seq   int64
}

func NewTaskRepository() *TaskRepository {
	return &TaskRepository{
		tasks: make(map[shared.TaskID]*domainTask.Task),
	}
}

func (r *TaskRepository) GetByID(_ context.Context, id shared.TaskID) (*domainTask.Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tasks[id]
	if !ok {
		return nil, shared.NotFoundError{Resource: "task", ID: string(id)}
	}
	return t, nil
}

func (r *TaskRepository) Save(_ context.Context, t *domainTask.Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks[t.ID] = t
	return nil
}

func (r *TaskRepository) FindChildren(_ context.Context, parentID shared.TaskID) ([]*domainTask.Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var children []*domainTask.Task
	for _, t := range r.tasks {
		if t.ParentID != nil && *t.ParentID == parentID {
			children = append(children, t)
		}
	}
	return children, nil
}

func (r *TaskRepository) FindAll(_ context.Context) ([]*domainTask.Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var all []*domainTask.Task
	for _, t := range r.tasks {
		all = append(all, t)
	}
	return all, nil
}

func (r *TaskRepository) NextID(_ context.Context) (shared.TaskID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	return shared.TaskID(shared.GenerateID()), nil
}
