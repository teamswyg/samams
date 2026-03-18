package app

import "proxy/internal/port"

// Option configures the TaskService.
type Option func(*TaskService)

func WithLogLines(n int) Option {
	return func(s *TaskService) {
		if n > 0 {
			s.logLines = n
		}
	}
}

func WithMaxTasks(n int) Option {
	return func(s *TaskService) {
		if n > 0 {
			s.maxTasks = n
		}
	}
}

func WithMaxAgents(n int) Option {
	return func(s *TaskService) {
		if n > 0 {
			s.maxAgents = n
		}
	}
}

func WithPublisher(p port.Publisher) Option {
	return func(s *TaskService) {
		if p != nil {
			s.publisher = p
		}
	}
}

func WithMaalStore(m *MaalStore) Option {
	return func(s *TaskService) {
		if m != nil {
			s.maal = m
		}
	}
}

func WithNotificationStore(n *NotificationStore) Option {
	return func(s *TaskService) {
		if n != nil {
			s.notifications = n
		}
	}
}

func WithBranchManager(b port.BranchManager) Option {
	return func(s *TaskService) {
		if b != nil {
			s.branches = b
		}
	}
}
