package notification

import (
	"context"
	"fmt"
	"log"
	"time"
)

// Service handles notification lifecycle.
type Service struct {
	repo Repository
	bus  EventBus
}

// NewService creates a new notification service.
func NewService(repo Repository, bus EventBus) *Service {
	return &Service{repo: repo, bus: bus}
}

// Created handles the notification.created event — persists and optionally publishes.
func (s *Service) Created(ctx context.Context, cmd Command) error {
	n := &Notification{
		ID:        fmt.Sprintf("notif-%d", time.Now().UnixNano()),
		UserID:    cmd.UserID,
		Title:     cmd.Title,
		Body:      cmd.Body,
		Severity:  cmd.Severity,
		Read:      false,
		CreatedAt: time.Now().UnixMilli(),
	}

	if s.repo != nil {
		if err := s.repo.Save(ctx, n); err != nil {
			return fmt.Errorf("save notification: %w", err)
		}
	}

	log.Printf("[notification] Created: %s — %s", n.Title, n.Body)
	return nil
}
