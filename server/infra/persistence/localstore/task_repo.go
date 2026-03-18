package localstore

import (
	"context"
	"fmt"

	"server/internal/domain/shared"
	domainTask "server/internal/domain/task"
	appTask "server/internal/app/task"
)

var _ appTask.TaskRepository = (*TaskRepository)(nil)

// TaskRepository stores tasks as JSON files.
// Key: tasks/{id}.json
type TaskRepository struct {
	store *Store
}

func NewTaskRepository(store *Store) *TaskRepository {
	return &TaskRepository{store: store}
}

func (r *TaskRepository) key(id shared.TaskID) string {
	return fmt.Sprintf("tasks/%s.json", id)
}

func (r *TaskRepository) GetByID(_ context.Context, id shared.TaskID) (*domainTask.Task, error) {
	var t domainTask.Task
	if err := r.store.Get(r.key(id), &t); err != nil {
		if err == ErrNotFound {
			return nil, shared.NotFoundError{Resource: "task", ID: string(id)}
		}
		return nil, err
	}
	return &t, nil
}

func (r *TaskRepository) Save(_ context.Context, t *domainTask.Task) error {
	return r.store.Put(r.key(t.ID), t)
}

func (r *TaskRepository) FindChildren(_ context.Context, parentID shared.TaskID) ([]*domainTask.Task, error) {
	keys, err := r.store.List("tasks")
	if err != nil {
		return nil, err
	}
	var children []*domainTask.Task
	for _, k := range keys {
		var t domainTask.Task
		if err := r.store.Get(k, &t); err != nil {
			continue
		}
		if t.ParentID != nil && *t.ParentID == parentID {
			children = append(children, &t)
		}
	}
	return children, nil
}

func (r *TaskRepository) FindAll(_ context.Context) ([]*domainTask.Task, error) {
	keys, err := r.store.List("tasks")
	if err != nil {
		return nil, err
	}
	var all []*domainTask.Task
	for _, k := range keys {
		var t domainTask.Task
		if err := r.store.Get(k, &t); err != nil {
			continue
		}
		all = append(all, &t)
	}
	return all, nil
}

func (r *TaskRepository) NextID(_ context.Context) (shared.TaskID, error) {
	return shared.TaskID(shared.GenerateID()), nil
}
