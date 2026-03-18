package project

import (
	"time"

	"server/internal/domain/shared"
)

type Status int32

const (
	StatusUnknown Status = iota
	StatusActive
	StatusArchived
)

type ID string

type Project struct {
	ID          ID
	TenantID    shared.TenantID
	Name        string
	Description string
	OwnerID     shared.UserID
	Status      Status
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func NewProject(clock shared.Clock, id ID, tenantID shared.TenantID, name string, ownerID shared.UserID, description string) (*Project, error) {
	if tenantID == "" {
		return nil, shared.ValidationError{Msg: "tenant_id is required"}
	}
	if name == "" {
		return nil, shared.ValidationError{Msg: "name is required"}
	}
	if ownerID == "" {
		return nil, shared.ValidationError{Msg: "owner_id is required"}
	}

	now := clock.Now()

	return &Project{
		ID:          id,
		TenantID:    tenantID,
		Name:        name,
		Description: description,
		OwnerID:     ownerID,
		Status:      StatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (p *Project) Archive(clock shared.Clock) error {
	if p == nil {
		return shared.ValidationError{Msg: "project is nil"}
	}
	if p.Status == StatusArchived {
		return nil
	}
	p.Status = StatusArchived
	p.UpdatedAt = clock.Now()
	return nil
}

