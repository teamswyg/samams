package task

import (
	"context"
	"fmt"
	"log"

	domainShared "server/internal/domain/shared"
	domainTask "server/internal/domain/task"
)

type Service struct {
	taskRepo   TaskRepository
	planner    ContextPlanner
	summarizer SummaryGenerator
	clock      domainShared.Clock
}

func NewService(taskRepo TaskRepository, planner ContextPlanner, summarizer SummaryGenerator, clock domainShared.Clock) *Service {
	if clock == nil {
		clock = domainShared.SystemClock{}
	}
	return &Service{
		taskRepo:   taskRepo,
		planner:    planner,
		summarizer: summarizer,
		clock:      clock,
	}
}

// CreateTask creates a new task using the Task aggregate.
func (s *Service) CreateTask(ctx context.Context, cmd CreateTaskCommand) (*domainTask.Task, error) {
	var id domainShared.TaskID
	if s.taskRepo != nil {
		next, err := s.taskRepo.NextID(ctx)
		if err != nil {
			return nil, err
		}
		id = next
	}

	task, err := domainTask.NewTask(
		s.clock,
		id,
		cmd.TenantID,
		cmd.ProjectID,
		cmd.Title,
		cmd.Description,
		cmd.ParentID,
	)
	if err != nil {
		return nil, err
	}

	if cmd.ParentID != nil && s.planner != nil && s.taskRepo != nil {
		parent, err := s.taskRepo.GetByID(ctx, *cmd.ParentID)
		if err == nil && parent != nil {
			summary, err := s.planner.BuildChildSummary(ctx, parent, SummaryInput{
				Title:       cmd.Title,
				Description: cmd.Description,
			})
			if err == nil {
				_ = task.ApplySummary(s.clock, summary)
			}
		}
	}

	if s.taskRepo != nil {
		if err := s.taskRepo.Save(ctx, task); err != nil {
			return nil, err
		}
	}

	return task, nil
}

// UpdateTaskStatus transitions a task to a new status.
func (s *Service) UpdateTaskStatus(ctx context.Context, cmd UpdateTaskStatusCommand) error {
	if s.taskRepo == nil {
		return nil
	}
	task, err := s.taskRepo.GetByID(ctx, cmd.TaskID)
	if err != nil {
		return err
	}
	if err := task.ChangeStatus(s.clock, cmd.Status); err != nil {
		return err
	}
	return s.taskRepo.Save(ctx, task)
}

// Cancel transitions a task to cancelled status.
func (s *Service) Cancel(ctx context.Context, cmd UpdateTaskStatusCommand) error {
	cmd.Status = domainTask.StatusCancelled
	return s.UpdateTaskStatus(ctx, cmd)
}

// HardStop transitions a task to hard-stopped status.
func (s *Service) HardStop(ctx context.Context, cmd UpdateTaskStatusCommand) error {
	cmd.Status = domainTask.StatusHardStopped
	return s.UpdateTaskStatus(ctx, cmd)
}

// Redistribute transitions a task to redistributed status.
func (s *Service) Redistribute(ctx context.Context, cmd UpdateTaskStatusCommand) error {
	cmd.Status = domainTask.StatusRedistributed
	return s.UpdateTaskStatus(ctx, cmd)
}

// SummaryCommand triggers (re-)summarization of a task.
type SummaryCommand struct {
	TaskID        string
	ManualTitle   string
	ManualSummary string
	Trigger       string
}

func (s *Service) UpdateSummary(ctx context.Context, cmd SummaryCommand) error {
	if s.taskRepo == nil {
		return nil
	}
	task, err := s.taskRepo.GetByID(ctx, domainShared.TaskID(cmd.TaskID))
	if err != nil {
		return err
	}
	return task.ApplySummary(s.clock, domainTask.TaskSummary{
		Title:       cmd.ManualTitle,
		Description: cmd.ManualSummary,
		Source:      cmd.Trigger,
	})
}

type RetryCommand struct {
	TaskID string
}

// IncrementRetry increases the retry count for a task.
func (s *Service) IncrementRetry(ctx context.Context, cmd RetryCommand) error {
	if s.taskRepo == nil {
		return nil
	}
	task, err := s.taskRepo.GetByID(ctx, domainShared.TaskID(cmd.TaskID))
	if err != nil {
		return err
	}
	if err := task.IncrementRetry(s.clock); err != nil {
		return err
	}
	return s.taskRepo.Save(ctx, task)
}

// CascadeExecute traverses the task tree generating summaries and frontier commands.
func (s *Service) CascadeExecute(ctx context.Context, taskID domainShared.TaskID) error {
	if s.taskRepo == nil || s.summarizer == nil {
		return nil
	}

	task, err := s.taskRepo.GetByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("cascade: find task %s: %w", taskID, err)
	}

	// If this is a root node with no summary, generate initial summary.
	if task.ParentID == nil && task.BuildChildContext() == "" {
		summary, err := s.summarizer.Summarize(ctx, task.Description)
		if err != nil {
			task.FailExecution(s.clock, "summary generation failed: "+err.Error())
			_ = s.taskRepo.Save(ctx, task)
			return fmt.Errorf("cascade: summarize root: %w", err)
		}
		task.CompleteExecution(s.clock, "root initialized", summary)
		if err := s.taskRepo.Save(ctx, task); err != nil {
			return err
		}
	}

	children, err := s.taskRepo.FindChildren(ctx, taskID)
	if err != nil {
		return fmt.Errorf("cascade: find children of %s: %w", taskID, err)
	}

	childContext := task.BuildChildContext()

	for _, child := range children {
		frontier, err := s.summarizer.GenerateFrontierCommand(ctx, childContext, child.Description)
		if err != nil {
			log.Printf("[Cascade] Failed to generate frontier for %s: %v", child.ID, err)
			child.FailExecution(s.clock, "frontier generation failed: "+err.Error())
			_ = s.taskRepo.Save(ctx, child)
			continue
		}

		child.ReceiveFrontier(s.clock, childContext, frontier)
		if err := s.taskRepo.Save(ctx, child); err != nil {
			return err
		}

		log.Printf("[Cascade] Task %s received frontier, status: %d", child.ID, child.Status)
	}

	return nil
}

// CompleteAndCascade is called when a task finishes execution.
func (s *Service) CompleteAndCascade(ctx context.Context, taskID domainShared.TaskID, executionResult string) error {
	if s.taskRepo == nil || s.summarizer == nil {
		return nil
	}

	task, err := s.taskRepo.GetByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("cascade: find task %s: %w", taskID, err)
	}

	ownSummary, err := s.summarizer.Summarize(ctx, executionResult)
	if err != nil {
		log.Printf("[Cascade] Summary generation failed for %s, using raw result", task.ID)
		ownSummary = executionResult
	}

	task.CompleteExecution(s.clock, executionResult, ownSummary)
	if err := s.taskRepo.Save(ctx, task); err != nil {
		return err
	}

	return s.CascadeExecute(ctx, taskID)
}
